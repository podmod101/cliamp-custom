package model

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/playlist"
)

func TestVibeLastRetainsResultsAndSelectionWithoutStartingWork(t *testing.T) {
	for _, partial := range []bool{false, true} {
		m := keybindingTestModel()
		m.playlist.Add(playlist.Track{Path: "original.mp3"})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		results := &vibeResult{Partial: partial, Matches: []vibeMatch{
			{Name: "First", URL: "https://first.example/live", Source: "Radio"},
			{Name: "Second", URL: "https://second.example/live", Source: "Radio"},
		}}
		m.vibe = vibeState{visible: true, loading: partial, searching: partial, gen: 1, results: results,
			cursor: 1, cancel: cancel, selected: playlist.PlaylistInfo{ID: "chosen", Name: "Playlist"}, title: "Playlist"}
		m.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.vibe.visible || ctx.Err() != nil {
			t.Fatal("Selecting playback should minimize without cancelling search")
		}
		cmd := m.openLastVibeResults()
		if cmd != nil || !m.vibe.visible || m.vibe.loading != partial || len(m.vibe.results.Matches) != len(results.Matches) ||
			m.vibe.cursor != 1 || m.vibe.selected.ID != "chosen" || !strings.HasPrefix(m.vibe.title, "Playlist") {
			t.Fatal("Alt+L should restore analyzed results and selection without any new work")
		}
		if m.playlist.Len() != 2 {
			t.Fatal("Reopening should not modify the queue")
		}
		m.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyUp})
		m.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		track, _ := m.playlist.Current()
		if track.Path != results.Matches[0].URL || m.playlist.Len() != 3 {
			t.Fatal("Another analyzed result should play normally")
		}
	}
}

func TestVibeLastAfterEscapeAndExplicitPlaylistReturn(t *testing.T) {
	m := keybindingTestModel()
	m.vibe = vibeState{visible: true, results: &vibeResult{Matches: []vibeMatch{{Name: "Saved", URL: "https://saved.example/live"}}}}
	m.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd := m.openLastVibeResults(); cmd != nil || !m.vibe.visible || m.vibe.results == nil {
		t.Fatal("Escape should preserve the results for Alt+L")
	}
	m.handleVibeKey(tea.KeyPressMsg{Code: 'b'})
	if m.vibe.results != nil {
		t.Fatal("Explicit playlist return should leave the results view")
	}
	if cmd := m.openVibeStreams(); cmd == nil || !m.vibe.loading {
		t.Fatal("Without retained results, Alt+R should load playlists normally")
	}
	m.closeVibeStreams()
	m.releaseVibeProvider()
}
