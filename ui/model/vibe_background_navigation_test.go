package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/ui"
)

func TestVibeBackgroundNormalNavigationAndStopMusic(t *testing.T) {
	m, ctx := backgroundFixture(t)
	m.playlist.Add(playlist.Track{Path: "one.mp3"}, playlist.Track{Path: "two.mp3"})
	m.playlist.Queue(0)
	m.playlist.Queue(1)
	m.minimizeVibeSearch()
	m.queue.visible = true
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.queue.cursor != 1 || ctx.Err() != nil {
		t.Fatal("Hidden search intercepted queue navigation")
	}
	m.handleKey(tea.KeyPressMsg{Code: 'l', Mod: tea.ModAlt})
	m.handleVibeKey(tea.KeyPressMsg{Code: 'm'})
	if !m.queue.visible || m.queue.cursor != 1 {
		t.Fatal("Reopening lost underlying queue selection")
	}
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.handleKey(tea.KeyPressMsg{Code: 'y', Mod: tea.ModShift, ShiftedCode: 'Y'})
	if m.provider.Name() != "YouTube" || ctx.Err() != nil {
		t.Fatal("Hidden search blocked provider navigation")
	}
	m.player.(*playbackFakeEngine).playing = true
	m.focus = focusPlaylist
	m.handleKey(tea.KeyPressMsg{Code: 's'})
	if m.player.IsPlaying() || !m.vibe.searching || ctx.Err() != nil {
		t.Fatal("Main s should stop playback without cancelling hidden search")
	}
}

func TestVibeMinimizeDoesNotConsumePlaylistFilterLetters(t *testing.T) {
	m := keybindingTestModel()
	m.vibe.visible, m.vibe.filtering = true, true
	for _, key := range "ms" {
		m.handleVibeKey(tea.KeyPressMsg{Code: key})
	}
	if m.vibe.query != "ms" || !m.vibe.visible {
		t.Fatal("Screen shortcuts intercepted filter typing")
	}
	m.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !m.vibe.visible || m.vibe.filtering {
		t.Fatal("First Escape should leave filter editing")
	}
	m.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.vibe.visible {
		t.Fatal("Second Escape should minimize")
	}
}

func TestVibeMinimizeReopensLoadingPickerAndCancelledEmptySearch(t *testing.T) {
	m, ctx := backgroundFixture(t)
	m.vibe.searching = false
	m.vibe.selected = playlist.PlaylistInfo{}
	m.minimizeVibeSearch()
	if cmd := m.openLastVibeResults(); cmd != nil || !m.vibe.visible || ctx.Err() != nil {
		t.Fatal("Loading picker was not reopened intact")
	}
	m, _ = backgroundFixture(t)
	m.stopVibeSearch()
	m.minimizeVibeSearch()
	if cmd := m.openLastVibeResults(); cmd != nil || !m.vibe.visible || m.vibe.loading || m.vibe.searching {
		t.Fatal("Cancelled empty search restarted or disappeared")
	}
	if !strings.Contains(m.renderVibeStreams(), "Search cancelled") {
		t.Fatal("Cancelled status missing")
	}
}

func TestVibeIndicatorFitsPlayerAndQueueAtSupportedSizes(t *testing.T) {
	prevStyle, prevWidth := ui.FrameStyle, ui.PanelWidth
	t.Cleanup(func() { ui.FrameStyle, ui.PanelWidth = prevStyle, prevWidth })
	for _, size := range [][2]int{{40, 10}, {80, 24}, {120, 32}} {
		for _, mode := range []string{"player", "queue", "simplified", "visualizer"} {
			m, _ := backgroundFixture(t)
			m.minimizeVibeSearch()
			m.width, m.height = size[0], size[1]
			m.vis = ui.NewVisualizer(44100)
			m.queue.visible = mode == "queue"
			m.simplified = mode == "simplified"
			m.fullVis = mode == "visualizer"
			frame := m.View().Content
			if !strings.Contains(frame, "Alt+L") || !strings.Contains(frame, "AI searching") {
				t.Fatalf("Missing search indicator in %s at %v: %q", mode, size, frame)
			}
			if lipgloss.Width(frame) > m.width || lipgloss.Height(frame) > m.height {
				t.Fatalf("Overflow in %s at %v", mode, size)
			}
		}
	}
}
