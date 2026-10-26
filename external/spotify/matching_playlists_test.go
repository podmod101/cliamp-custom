package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func matchingProvider(t *testing.T, transport roundTripFunc) *SpotifyProvider {
	t.Helper()
	previous := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = previous })
	return New(&Session{tokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fixture"})}, DefaultClientID, 320)
}

func matchingResponse(request *http.Request, status int, payload any, retry string) *http.Response {
	body, _ := json.Marshal(payload)
	header := make(http.Header)
	if retry != "" {
		header.Set("Retry-After", retry)
	}
	return &http.Response{StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header: header, Body: io.NopCloser(strings.NewReader(string(body))), Request: request}
}

func TestMatchingPlaylistsOnlyLoadsPlaylistsAndCachesAllPages(t *testing.T) {
	calls := 0
	p := matchingProvider(t, func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.Path != "/v1/me/playlists" {
			t.Fatalf("Unnecessary endpoint blocks playlist loading: %s", request.URL.Path)
		}
		offset, _ := strconv.Atoi(request.URL.Query().Get("offset"))
		items := []map[string]any{}
		for i := offset; i < min(offset+spotifyPlaylistPageSize, 51); i++ {
			items = append(items, map[string]any{"id": fmt.Sprintf("p%d", i), "name": fmt.Sprintf("Playlist %d", i),
				"snapshot_id": "snapshot", "items": map[string]int{"total": 3}})
		}
		return matchingResponse(request, 200, map[string]any{"items": items, "total": 51}, ""), nil
	})
	got, err := p.MatchingPlaylists(context.Background())
	if err != nil || len(got) != 51 || calls != 2 || got[50].ID != "p50" {
		t.Fatalf("Bad paginated playlist result: len=%d calls=%d error=%v", len(got), calls, err)
	}
	got[0].Name = "Caller mutation"
	cached, err := p.MatchingPlaylists(context.Background())
	if err != nil || calls != 2 || cached[0].Name == "Caller mutation" {
		t.Fatal("Cache was not reused or was mutated by caller")
	}
	p.Refresh()
	if _, err := p.MatchingPlaylists(context.Background()); err != nil || calls != 4 {
		t.Fatalf("Explicit refresh failed: calls=%d error=%v", calls, err)
	}
}

func TestMatchingPlaylistsHonorsCancellation(t *testing.T) {
	p := matchingProvider(t, func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := p.MatchingPlaylists(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Request did not respect context: %v", err)
	}
}

func TestSpotifyLongRateLimitFailsPromptlyAndRefreshRespectsCooldown(t *testing.T) {
	calls := 0
	p := matchingProvider(t, func(request *http.Request) (*http.Response, error) {
		calls++
		return matchingResponse(request, 429, map[string]string{"error": "rate limited"}, "86400"), nil
	})
	before := time.Now()
	_, err := p.MatchingPlaylists(context.Background())
	var limited *RateLimitError
	if !errors.As(err, &limited) || !limited.SharedClient || limited.Until.Before(before.Add(23*time.Hour)) {
		t.Fatalf("Lost Spotify cooldown information: %v", err)
	}
	if time.Since(before) > time.Second {
		t.Fatal("Long cooldown should be returned, not slept through")
	}
	p.Refresh()
	_, err = p.MatchingPlaylists(context.Background())
	if !errors.As(err, &limited) || calls != 1 {
		t.Fatalf("Retry bypassed cooldown: calls=%d error=%v", calls, err)
	}
	if strings.Contains(err.Error(), "re-auth") {
		t.Fatal("Rate limiting should not suggest resetting valid credentials")
	}
}

func TestSpotifyCooldownDoesNotBecomeContextDeadlineExceeded(t *testing.T) {
	p := matchingProvider(t, func(request *http.Request) (*http.Response, error) {
		return matchingResponse(request, 429, nil, "1"), nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := p.MatchingPlaylists(ctx)
	var limited *RateLimitError
	if !errors.As(err, &limited) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Rate limit was hidden behind timeout: %v", err)
	}
}

func TestRetryDelayParsesSecondsAndHTTPDates(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		value string
		want  time.Duration
	}{
		{"86400", 24 * time.Hour}, {now.Add(2 * time.Minute).Format(http.TimeFormat), 2 * time.Minute},
		{"", time.Second}, {"broken", time.Second}, {"-4", time.Second},
	} {
		if got := retryDelay(test.value, now, time.Second); got != test.want {
			t.Errorf("retryDelay(%q) = %v, want %v", test.value, got, test.want)
		}
	}
}
