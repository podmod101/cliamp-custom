package spotify

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"
)

func TestMatchingTracksBoundsLargePlaylistAndPreservesOpening(t *testing.T) {
	offsets := []int{}
	p := matchingProvider(t, func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/playlists/large/items" {
			t.Fatal("Wrong playlist endpoint")
		}
		offset, _ := strconv.Atoi(request.URL.Query().Get("offset"))
		offsets = append(offsets, offset)
		items := []any{}
		for i := offset; i < offset+50; i++ {
			items = append(items, map[string]any{"item": map[string]any{"id": fmt.Sprint(i), "name": fmt.Sprint(i),
				"type": "track", "uri": "spotify:track:" + fmt.Sprint(i), "artists": []any{map[string]string{"name": "Artist"}}}})
		}
		return matchingResponse(request, 200, map[string]any{"items": items, "total": 11000}, ""), nil
	})
	tracks, err := p.MatchingTracks(context.Background(), "large")
	if err != nil || len(tracks) != 300 || len(offsets) != 6 || offsets[0] != 0 || offsets[1] != 50 {
		t.Fatalf("Unbounded or unstratified metadata sample: tracks=%d offsets=%v err=%v", len(tracks), offsets, err)
	}
	for i := 0; i < 100; i++ {
		if tracks[i].Title != fmt.Sprint(i) {
			t.Fatal("Opening order lost")
		}
	}
	seen := map[int]bool{}
	for _, offset := range offsets {
		if seen[offset] {
			t.Fatal("Duplicate page")
		}
		seen[offset] = true
	}
	tracks[0].Title = "Mutation"
	cached, err := p.MatchingTracks(context.Background(), "large")
	if err != nil || len(offsets) != 6 || cached[0].Title == "Mutation" {
		t.Fatal("Matching sample cache not isolated/reused")
	}
}

func TestMatchingPlaylistCarriesDescriptionWithoutExtraCalls(t *testing.T) {
	p := matchingProvider(t, func(request *http.Request) (*http.Response, error) {
		return matchingResponse(request, 200, map[string]any{"items": []any{map[string]any{
			"id": "one", "name": "Playlist", "description": "Electronic blues and glitch hop", "snapshot_id": "v1"}}, "total": 1}, ""), nil
	})
	if _, err := p.MatchingPlaylists(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.MatchingPlaylistDescription("one") != "Electronic blues and glitch hop" {
		t.Fatal("Curator intent discarded")
	}
	p.Refresh()
	if p.MatchingPlaylistDescription("one") != "" {
		t.Fatal("Refresh retained stale intent")
	}
}
