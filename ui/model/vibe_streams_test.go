package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/cliamp/playlist"
)

func TestVibeHotkeyOpensFromMainAndProviderWithoutReplacingQueue(t *testing.T) {
	for _, focus := range []focusArea{focusPlaylist, focusProvider} {
		m := keybindingTestModel()
		m.focus = focus
		m.playlist.Add(playlist.Track{Path: "original.mp3"})
		cmd := m.handleKey(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
		if cmd == nil || !m.vibe.visible || !m.vibe.loading || m.playlist.Len() != 1 {
			t.Fatal("Alt+R did not open playlist matcher while preserving queue")
		}
	}
}

func TestVibeMinimizeThenExplicitCancellationRejectsStaleResult(t *testing.T) {
	m := keybindingTestModel()
	ctx, cancel := context.WithCancel(context.Background())
	m.vibe = vibeState{visible: true, loading: true, searching: true, gen: 3, cancel: cancel}
	m.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if ctx.Err() != nil || m.vibe.visible {
		t.Fatal("Escape must minimize without cancelling")
	}
	m.stopVibeSearch()
	if ctx.Err() == nil {
		t.Fatal("Explicit cancel did not stop work")
	}
	m.handleVibeEvent(vibeEvent{gen: 3, finished: true, result: &vibeResult{Summary: "stale"}})
	if m.vibe.results != nil {
		t.Fatal("Cancelled result reopened picker")
	}
}

func TestVibeSignInAndAlbumFiltering(t *testing.T) {
	m := keybindingTestModel()
	m.width, m.height = 100, 24
	m.vibe = vibeState{visible: true, gen: 1}
	m.handleVibeEvent(vibeEvent{gen: 1, finished: true, err: playlist.ErrNeedsAuth})
	if !m.vibe.auth || !strings.Contains(m.renderVibeStreams(), "sign in") {
		t.Fatal("Sign-in requirement not explained")
	}
	m.handleVibeEvent(vibeEvent{gen: 1, finished: true, lists: []playlist.PlaylistInfo{
		{ID: "one", Name: "Sample playlist"}, {ID: "spotify:album:two", Name: "Album"},
	}})
	if len(m.vibe.lists) != 1 || m.vibe.auth {
		t.Fatal("Saved albums should not appear as playlists")
	}
}

func TestVibeSelectingResultPlaysStreamAndPreservesQueue(t *testing.T) {
	m := keybindingTestModel()
	m.playlist.Add(playlist.Track{Path: "original.mp3"})
	m.vibe = vibeState{visible: true, results: &vibeResult{Matches: []vibeMatch{
		{Name: "Matched station", URL: "https://stream.example/live", Source: "Radio"},
	}}}
	m.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.vibe.visible || m.playlist.Len() != 2 {
		t.Fatal("Result selection should append and play without losing queue")
	}
	track, _ := m.playlist.Current()
	if track.Path != "https://stream.example/live" || !track.Realtime {
		t.Fatalf("Wrong playback target: %+v", track)
	}
}

func TestVibeFailureDoesNotModifyQueue(t *testing.T) {
	m := keybindingTestModel()
	m.playlist.Add(playlist.Track{Path: "original.mp3"})
	m.vibe = vibeState{visible: true, loading: true, gen: 2}
	m.handleVibeEvent(vibeEvent{gen: 2, finished: true, err: errors.New("no usable samples")})
	if m.vibe.loading || m.vibe.err == "" || m.playlist.Len() != 1 {
		t.Fatal("Matching failure should remain reviewable and preserve playback queue")
	}
}
