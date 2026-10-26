package model

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/playlist"
)

type backgroundCountingProvider struct {
	commandsTestProvider
	calls atomic.Int64
}

func (p *backgroundCountingProvider) Tracks(string) ([]playlist.Track, error) {
	p.calls.Add(1)
	return []playlist.Track{{Title: "Fixture song", Artist: "Fixture artist"}}, nil
}

func boundedVibeMessage(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("Missing event waiter")
	}
	output := make(chan tea.Msg, 1)
	go func() { output <- cmd() }()
	select {
	case msg := <-output:
		return msg
	case <-time.After(4 * time.Second):
		t.Fatal("Worker event timed out")
		return nil
	}
}

func TestVibeBackgroundRealWorkerContinuesWhileMinimized(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell worker fixture")
	}
	folder := t.TempDir()
	gate, helper := filepath.Join(folder, "continue"), filepath.Join(folder, "helper")
	result := vibeResult{Matches: []vibeMatch{
		{Name: "Station one", URL: "https://one.example/live", Source: "Radio", Score: .6},
		{Name: "Station two", URL: "https://two.example/live", Source: "Radio", Score: .5},
	}}
	preview, _ := json.Marshal(map[string]any{"preview": result})
	final, _ := json.Marshal(result)
	fixture := "#!/bin/sh\ncat >/dev/null\necho '{\"progress\":\"Waiting for test gate\"}' >&2\n" +
		"while [ ! -f \"$CLIAMP_VIBE_TEST_GATE\" ]; do sleep 0.02; done\n" +
		"echo '" + string(preview) + "' >&2\necho '" + string(final) + "'\n"
	if err := os.WriteFile(helper, []byte(fixture), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLIAMP_VIBE_HELPER", helper)
	t.Setenv("CLIAMP_VIBE_TEST_GATE", gate)
	source := &backgroundCountingProvider{commandsTestProvider: commandsTestProvider{name: "Spotify"}}
	m := keybindingTestModel()
	m.width, m.height = 120, 32
	m.recomputeLayout()
	m.vibe.provider = source
	m.vibeResultsPath = filepath.Join(folder, "results.json")
	m.playlist.Add(playlist.Track{Path: "existing.mp3"})
	m.player.(*playbackFakeEngine).playing = true
	wait := m.beginVibeSearch(playlist.PlaylistInfo{ID: "fixture", Name: "Fixture playlist"}, false)
	defer func() { m.closeVibeStreams() }()
	msg := boundedVibeMessage(t, wait)
	updated, wait := m.Update(msg)
	m = updated.(Model)
	generation := m.vibe.gen
	if m.vibe.progress != "Waiting for test gate" {
		t.Fatal("Initial progress not delivered")
	}
	// A real helper remains blocked here until the test releases it.
	m.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.handleKey(tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.player.IsPaused() || !m.vibe.searching || m.vibe.visible {
		t.Fatal("Minimizing blocked music controls or cancelled the worker")
	}
	if cmd := m.handleKey(tea.KeyPressMsg{Code: 'l', Mod: tea.ModAlt}); cmd != nil || !m.vibe.visible || m.vibe.gen != generation {
		t.Fatal("Reopening duplicated the waiter, restarted or cancelled the job")
	}
	m.handleVibeKey(tea.KeyPressMsg{Code: 'm'})
	if err := os.WriteFile(gate, []byte("continue"), 0600); err != nil {
		t.Fatal(err)
	}
	msg = boundedVibeMessage(t, wait)
	updated, wait = m.Update(msg)
	m = updated.(Model)
	if m.vibe.visible || m.vibe.results == nil || len(m.vibe.results.Matches) != 2 || !m.vibe.searching {
		t.Fatal("Hidden preview was dropped or stole focus")
	}
	msg = boundedVibeMessage(t, wait)
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if m.vibe.visible || m.vibe.searching || m.vibe.loading || source.calls.Load() != 1 || !m.player.IsPaused() {
		t.Fatal("Completion changed playback/focus or caused another playlist fetch")
	}
	if _, err := os.Stat(m.vibeResultsPath); err != nil {
		t.Fatal("Hidden completion was not persisted")
	}
	m.handleKey(tea.KeyPressMsg{Code: 'l', Mod: tea.ModAlt})
	m.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyDown})
	m.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	current, _ := m.playlist.Current()
	if current.Path != result.Matches[1].URL || source.calls.Load() != 1 {
		t.Fatal("Selecting another result reprocessed the playlist")
	}
	m.handleKey(tea.KeyPressMsg{Code: 'l', Mod: tea.ModAlt})
	if !m.vibe.visible || m.vibe.cursor != 1 {
		t.Fatal("Latest result selection was lost")
	}
}

func TestVibeBackgroundReplacementWhileWorkerFinishesKeepsConfirmationExplicit(t *testing.T) {
	m, _ := backgroundFixture(t)
	m.openVibeStreams()
	m.handleVibeEvent(vibeEvent{gen: 5, finished: true, err: context.DeadlineExceeded})
	if !m.vibe.replacePrompt || m.vibe.searching {
		t.Fatal("Completion silently accepted a replacement")
	}
	m.handleVibeKey(tea.KeyPressMsg{Code: 'n'})
	if m.vibe.replacePrompt || m.vibe.err == "" {
		t.Fatal("Declining lost the completed search state")
	}
}
