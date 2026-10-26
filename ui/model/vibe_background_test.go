package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/playlist"
)

func backgroundFixture(t *testing.T) (Model, context.Context) {
	t.Helper()
	m := keybindingTestModel()
	m.width, m.height = 120, 32
	m.recomputeLayout()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	m.vibe = vibeState{visible: true, loading: true, searching: true, gen: 5, cancel: cancel,
		selected: playlist.PlaylistInfo{ID: "chosen", Name: "Playlist"}, title: "Playlist", progress: "Finding audio"}
	return m, ctx
}

func TestVibeMinimizeKeepsJobAndReopensWithoutAnotherWaiter(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyEscape}, {Code: 'm'}} {
		m, ctx := backgroundFixture(t)
		generation := m.vibe.gen
		events := make(chan vibeEvent, 2)
		m.vibe.events = events
		if cmd := m.handleVibeKey(key); cmd != nil || m.vibe.visible || !m.vibe.searching || ctx.Err() != nil || m.vibe.gen != generation {
			t.Fatal("Minimize changed the active worker lifetime")
		}
		m.queue.visible = true
		if cmd := m.handleKey(tea.KeyPressMsg{Code: 'l', Mod: tea.ModAlt}); cmd != nil || !m.vibe.visible || ctx.Err() != nil || m.vibe.events != events || m.vibe.gen != generation {
			t.Fatal("Alt+L restarted/cancelled work or scheduled another waiter")
		}
		m.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyEscape})
		if !m.queue.visible {
			t.Fatal("Underlying queue view was lost")
		}
	}
}

func TestVibeBackgroundEventsFinishWithoutOpeningScreenOrPlaying(t *testing.T) {
	m, ctx := backgroundFixture(t)
	m.playlist.Add(playlist.Track{Path: "one.mp3"}, playlist.Track{Path: "two.mp3"})
	m.player.(*playbackFakeEngine).playing = true
	m.minimizeVibeSearch()
	events := make(chan vibeEvent, 2)
	m.vibe.events = events
	updated, wait := m.Update(vibeEvent{gen: 5, stage: "Comparing stations"})
	m = updated.(Model)
	if wait == nil || m.vibe.progress != "Comparing stations" || m.vibe.visible || ctx.Err() != nil {
		t.Fatal("Hidden progress was dropped")
	}
	// Normal playback controls remain routed to the player while the job runs.
	m.handleKey(tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.player.IsPaused() || ctx.Err() != nil || !m.vibe.searching {
		t.Fatal("Background search blocked pause")
	}
	m.handleKey(tea.KeyPressMsg{Code: tea.KeySpace})
	m.handleKey(tea.KeyPressMsg{Code: '.'})
	track, _ := m.playlist.Current()
	if track.Path != "two.mp3" || ctx.Err() != nil {
		t.Fatal("Background search blocked track change")
	}
	result := &vibeResult{Matches: []vibeMatch{{Name: "Verified", URL: "https://station.example/live", Source: "Radio"}}}
	updated, _ = m.Update(vibeEvent{gen: 5, result: result, finished: true})
	m = updated.(Model)
	track, _ = m.playlist.Current()
	if m.vibe.visible || m.vibe.searching || m.vibe.results != result || ctx.Err() == nil || track.Path != "two.mp3" {
		t.Fatal("Hidden completion was lost, reopened the screen or changed music")
	}
	if !strings.Contains(m.vibeIndicator(), "results ready") {
		t.Fatal("Completion indicator missing")
	}
	m.handleKey(tea.KeyPressMsg{Code: 'l', Mod: tea.ModAlt})
	if !m.vibe.visible || m.vibe.loading {
		t.Fatal("Completed results did not reopen directly")
	}
	m.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	track, _ = m.playlist.Current()
	if track.Path != result.Matches[0].URL || m.playlist.Len() != 3 {
		t.Fatal("Completed result did not play normally")
	}
	m.handleKey(tea.KeyPressMsg{Code: 'l', Mod: tea.ModAlt})
	if !m.vibe.visible || len(m.vibe.results.Matches) != 1 {
		t.Fatal("Another result visit restarted the search")
	}
}

func TestVibeReplacementRequiresExplicitYes(t *testing.T) {
	m, ctx := backgroundFixture(t)
	generation := m.vibe.gen
	m.minimizeVibeSearch()
	m.handleKey(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	if !m.vibe.replacePrompt || ctx.Err() != nil || m.vibe.gen != generation {
		t.Fatal("Alt+R cancelled without confirmation")
	}
	if !strings.Contains(m.renderVibeStreams(), "will cancel") {
		t.Fatal("Replacement consequence unclear")
	}
	m.handleVibeKey(tea.KeyPressMsg{Code: 'n'})
	if m.vibe.replacePrompt || ctx.Err() != nil {
		t.Fatal("Declining replacement cancelled the job")
	}
	m.openVibeStreams()
	cmd := m.handleVibeKey(tea.KeyPressMsg{Code: 'y'})
	if cmd == nil || ctx.Err() == nil || m.vibe.searching || !m.vibe.loading || m.vibe.replacePrompt {
		t.Fatal("Confirmed replacement did not enter new playlist workflow")
	}
	m.closeVibeStreams()
	m.releaseVibeProvider()
}

func TestVibeCancelBeforeFirstResultAndBackgroundFailure(t *testing.T) {
	m, ctx := backgroundFixture(t)
	m.handleVibeKey(tea.KeyPressMsg{Code: 's'})
	if ctx.Err() == nil || m.vibe.searching || m.vibe.loading || !m.vibe.visible {
		t.Fatal("Explicit cancel before first result failed")
	}
	m.handleVibeEvent(vibeEvent{gen: 5, result: &vibeResult{Summary: "late"}, finished: true})
	if m.vibe.results != nil {
		t.Fatal("Cancelled worker result was accepted")
	}
	m, _ = backgroundFixture(t)
	m.minimizeVibeSearch()
	m.handleVibeEvent(vibeEvent{gen: 5, err: errors.New("fixture failure"), finished: true})
	if m.vibe.visible || !strings.Contains(m.vibeIndicator(), "attention") {
		t.Fatal("Hidden failure stole focus or lost its indicator")
	}
	m.openLastVibeResults()
	if !m.vibe.visible || m.vibe.err != "fixture failure" {
		t.Fatal("Alt+L lost failure detail or restarted it")
	}
}

func TestVibeIndicatorAppearsWithoutConsumingPlaybackStatus(t *testing.T) {
	m, _ := backgroundFixture(t)
	m.minimizeVibeSearch()
	if !strings.Contains(m.renderBottomStatus(), "AI searching") {
		t.Fatal("Running indicator missing from main status")
	}
	m.simplified = true
	sections := m.mainSections("", true, false)
	if !strings.Contains(strings.Join(sections, "\n"), "AI searching") {
		t.Fatal("Simplified player lost background indicator")
	}
}
