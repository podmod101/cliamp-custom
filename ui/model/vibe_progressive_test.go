package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/playlist"
)

func TestVibeProgressiveUpdatePreservesSelectionAndAllowsPlayback(t *testing.T) {
	m := keybindingTestModel()
	m.playlist.Add(playlist.Track{Path: "original.mp3"})
	first := vibeMatch{Name: "One", URL: "https://one.example/live", Source: "Radio"}
	second := vibeMatch{Name: "Two", URL: "https://two.example/live", Source: "Radio"}
	m.vibe = vibeState{visible: true, loading: true, gen: 1, cursor: 1,
		results: &vibeResult{Matches: []vibeMatch{first, second}, Partial: true}}
	m.handleVibeEvent(vibeEvent{gen: 1, result: &vibeResult{Matches: []vibeMatch{second, first}, Partial: true}})
	if m.vibe.cursor != 0 || !m.vibe.loading {
		t.Fatal("Live reranking should preserve selected station and keep search active")
	}
	m.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	track, _ := m.playlist.Current()
	if m.vibe.visible || m.playlist.Len() != 2 || track.Path != second.URL {
		t.Fatal("A progressive match should play normally while preserving the queue")
	}
}

func TestVibeStopKeepsVerifiedResultsAndRejectsLateEvents(t *testing.T) {
	m := keybindingTestModel()
	ctx, cancel := context.WithCancel(context.Background())
	m.vibe = vibeState{visible: true, loading: true, gen: 2, cancel: cancel,
		results: &vibeResult{Matches: []vibeMatch{{Name: "Verified"}}, Partial: true}}
	m.handleVibeKey(tea.KeyPressMsg{Code: 's'})
	if ctx.Err() == nil || !m.vibe.visible || m.vibe.loading || m.vibe.results == nil {
		t.Fatal("Stop should cancel search and keep verified results browsable")
	}
	m.handleVibeEvent(vibeEvent{gen: 2, finished: true, err: errors.New("late cancellation")})
	if m.vibe.err != "" {
		t.Fatal("Stopped worker must not replace retained results with late error")
	}
}

func TestVibeDeeperSearchRetainsResultsAndPlaylist(t *testing.T) {
	m := keybindingTestModel()
	m.vibe = vibeState{visible: true, gen: 2, selected: playlist.PlaylistInfo{ID: "selected", Name: "Playlist"},
		results: &vibeResult{Matches: []vibeMatch{{Name: "Verified"}}}}
	cmd := m.handleVibeKey(tea.KeyPressMsg{Code: 'd'})
	defer m.vibe.cancel()
	if cmd == nil || !m.vibe.loading || m.vibe.results == nil || m.vibe.selected.ID != "selected" {
		t.Fatal("Deeper search should reuse playlist and leave verified results available")
	}
}

func TestVibeBackCancelsSearchAndFailurePreservesPreview(t *testing.T) {
	m := keybindingTestModel()
	ctx, cancel := context.WithCancel(context.Background())
	m.vibe = vibeState{visible: true, loading: true, gen: 2, cancel: cancel,
		results: &vibeResult{Matches: []vibeMatch{{Name: "Verified"}}}}
	m.handleVibeEvent(vibeEvent{gen: 2, finished: true, err: context.DeadlineExceeded})
	if m.vibe.results == nil || m.vibe.loading || ctx.Err() == nil {
		t.Fatal("Timeout should preserve verified results and release context")
	}
	cmd := m.handleVibeKey(tea.KeyPressMsg{Code: 'b'})
	if m.vibe.results != nil || cmd == nil || !m.vibe.loading {
		t.Fatal("Back should return to playlists")
	}
	defer m.vibe.cancel()
}

func TestVibeViewSeparatesPredictionFromAudioAndShowsControls(t *testing.T) {
	m := keybindingTestModel()
	m.width, m.height = 120, 32
	m.vibe = vibeState{visible: true, loading: true, results: &vibeResult{Matches: []vibeMatch{
		{Name: "Station", Source: "Radio", Reason: "measured sound", Prediction: "predicted fit", Uncertainty: "short excerpt", SampleSeconds: 20},
	}}}
	view := m.renderVibeStreams()
	for _, expected := range []string{"Audio:", "Metadata prediction:", "short excerpt", "s stops search", "worldwide"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("Missing %q in progressive results", expected)
		}
	}
}
