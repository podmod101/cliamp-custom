package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/playlist"
)

func TestVibeGenrePickerPreservesPriorResultsAndStartsSelectedFocus(t *testing.T) {
	m := keybindingTestModel()
	m.width, m.height = 100, 24
	previous := &vibeResult{Genres: []vibeGenre{{ID: "edm", Name: "Electronic / EDM"}, {ID: "blues", Name: "Blues"}},
		Matches: []vibeMatch{{Name: "Verified", URL: "https://station.example/live"}}}
	m.vibe = vibeState{visible: true, results: previous, selected: playlist.PlaylistInfo{ID: "chosen", Name: "Playlist"}}
	m.handleVibeKey(tea.KeyPressMsg{Code: 'g'})
	if !m.vibe.choosingGenre || !strings.Contains(m.renderVibeStreams(), "Electronic / EDM") {
		t.Fatal("Genre picker not shown")
	}
	m.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyDown})
	cmd := m.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	defer m.vibe.cancel()
	if cmd == nil || m.vibe.genre != "edm" || !m.vibe.loading || m.vibe.results != previous || m.vibe.choosingGenre {
		t.Fatal("Focused search lost prior results or selected wrong genre")
	}
	if previous.Genre != "" {
		t.Fatal("Prior unfiltered results must retain their own genre label")
	}
}

func TestVibeGenrePickerCancelDoesNotStartWork(t *testing.T) {
	m := keybindingTestModel()
	m.vibe = vibeState{visible: true, results: &vibeResult{Genres: []vibeGenre{{ID: "edm", Name: "EDM"}}}}
	m.handleVibeKey(tea.KeyPressMsg{Code: 'g'})
	cmd := m.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil || m.vibe.choosingGenre || !m.vibe.visible || m.vibe.loading {
		t.Fatal("Cancelling genre menu started work or closed results")
	}
}
