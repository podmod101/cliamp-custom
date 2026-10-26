package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/cliamp/playlist"
)

func waitVibeEvent(events <-chan vibeEvent) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return nil
		}
		return event
	}
}

func startVibeWorker(ctx context.Context, source playlist.Provider, item playlist.PlaylistInfo, gen uint64, requested ...vibeWorkerOptions) <-chan vibeEvent {
	events := make(chan vibeEvent, 16)
	go func() {
		defer close(events)
		send := func(event vibeEvent) {
			event.gen = gen
			// Progress is expendable; reserve a slot for completion even if the
			// terminal is briefly too busy to consume updates.
			if len(events) >= cap(events)-1 {
				return
			}
			select {
			case events <- event:
			case <-ctx.Done():
			default:
			}
		}
		options := vibeWorkerOptions{}
		if len(requested) > 0 {
			options = requested[0]
		}
		options.preview = func(result *vibeResult) { send(vibeEvent{result: result}) }
		result, err := runVibeWorker(ctx, source, item, func(stage string) { send(vibeEvent{stage: stage}) }, options)
		// Completion is consumed even while minimized. Cancellation may have
		// retired this generation; the bounded buffer avoids blocking that worker.
		select {
		case events <- vibeEvent{gen: gen, result: result, err: err, finished: true}:
		default:
		}
	}()
	return events
}

type vibeWorkerOptions struct {
	deeper  bool
	preview func(*vibeResult)
	genre   string
}

func runVibeWorker(ctx context.Context, source playlist.Provider, item playlist.PlaylistInfo, progress func(string), options ...vibeWorkerOptions) (*vibeResult, error) {
	if source == nil {
		return nil, fmt.Errorf("Spotify playlist source is unavailable")
	}
	settings := vibeWorkerOptions{}
	if len(options) > 0 {
		settings = options[0]
	}
	helper := os.Getenv("CLIAMP_VIBE_HELPER")
	if helper == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		helper = filepath.Join(home, ".local", "libexec", "cliamp-vibe", "run")
	}
	if _, err := os.Stat(helper); err != nil {
		return nil, fmt.Errorf("audio matching helper is not installed; see docs/playlist-streams.md")
	}
	var tracks []playlist.Track
	var err error
	if sampler, ok := source.(interface {
		MatchingTracks(context.Context, string) ([]playlist.Track, error)
	}); ok {
		tracks, err = sampler.MatchingTracks(ctx, item.ID)
	} else {
		tracks, err = source.Tracks(item.ID)
	}
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	type song struct {
		Title    string `json:"title"`
		Artist   string `json:"artist"`
		Duration int    `json:"duration"`
	}
	songs := make([]song, 0, len(tracks))
	for _, track := range tracks {
		if track.Title != "" && track.Artist != "" && !strings.HasPrefix(track.Path, "spotify:episode:") {
			songs = append(songs, song{track.Title, track.Artist, track.DurationSecs})
		}
	}
	description := ""
	if metadata, ok := source.(interface{ MatchingPlaylistDescription(string) string }); ok {
		description = metadata.MatchingPlaylistDescription(item.ID)
	}
	input, err := json.Marshal(struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Tracks        []song `json:"tracks"`
		Deeper        bool   `json:"deeper"`
		Description   string `json:"description"`
		Genre         string `json:"genre"`
		PlaylistTotal int    `json:"playlist_total"`
	}{item.ID, item.Name, songs, settings.deeper, description, settings.genre, item.TrackCount})
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, helper)
	cmd.Stdin = bytes.NewReader(input)
	// Python handles SIGTERM by terminating its capture/search children.
	configureVibeCancellation(cmd)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	cmd.Stdout = &output
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start audio matcher: %w", err)
	}
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 4096), 512*1024)
	for scanner.Scan() {
		var event struct {
			Progress string
			Preview  *vibeResult
		}
		if json.Unmarshal(scanner.Bytes(), &event) == nil {
			if event.Progress != "" {
				progress(event.Progress)
			}
			if event.Preview != nil && settings.preview != nil && validateVibeResult(event.Preview) == nil {
				settings.preview(event.Preview)
			}
		}
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var result vibeResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("audio matcher failed; check isolated audio environment (%v)", waitErr)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("%s", result.Error)
	}
	if waitErr != nil {
		return nil, waitErr
	}
	if err := validateVibeResult(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func validateVibeResult(result *vibeResult) error {
	if len(result.Matches) == 0 || len(result.Matches) > 10 {
		return fmt.Errorf("audio matcher returned an invalid result count")
	}
	seen := make(map[string]bool)
	for _, match := range result.Matches {
		parsed, err := url.Parse(match.URL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") ||
			match.Name == "" || seen[match.URL] || math.IsNaN(match.Score) || math.IsInf(match.Score, 0) || match.Score < -1 || match.Score > 1 {
			return fmt.Errorf("audio matcher returned an invalid stream")
		}
		seen[match.URL] = true
	}
	return nil
}
