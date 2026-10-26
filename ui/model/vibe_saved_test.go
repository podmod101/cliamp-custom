package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/playlist"
)

func savedVibeFixture() *vibeSnapshot {
	return &vibeSnapshot{Version: 1, SavedAt: 12345, Cursor: 1, Playlist: playlist.PlaylistInfo{ID: "chosen", Name: "Playlist"},
		Result: &vibeResult{MeasuredAt: 12000, Matches: []vibeMatch{
			{Name: "First", URL: "https://one.example/live", Source: "Radio", Score: .6},
			{Name: "Second", URL: "https://two.example/live", Source: "Radio", Score: .5},
		}}}
}

func TestVibeSavedResultsSurviveRestartAndPlayWithoutAnySearch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.json")
	first := keybindingTestModel()
	first.vibeResultsPath = path
	fixture := savedVibeFixture()
	first.vibe = vibeState{visible: true, results: fixture.Result, selected: fixture.Playlist, cursor: 1}
	first.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("Saved results not private: %v", err)
	}
	for _, focus := range []focusArea{focusPlaylist, focusProvider} {
		original, _ := json.Marshal(first.vibe.last)
		if err := os.WriteFile(path, original, 0600); err != nil {
			t.Fatal(err)
		}
		restarted := keybindingTestModel()
		restarted.focus = focus
		restarted.vibeResultsPath = path
		restarted.playlist.Add(playlist.Track{Path: "existing.mp3"})
		cmd := restarted.handleKey(tea.KeyPressMsg{Code: 'l', Mod: tea.ModAlt})
		if cmd != nil || !restarted.vibe.visible || restarted.vibe.loading || restarted.vibe.cursor != 1 ||
			restarted.vibe.provider != nil || len(restarted.vibe.results.Matches) != 2 || !restarted.vibe.restored {
			t.Fatalf("Alt+L should restore exact results without a provider: visible=%v loading=%v cursor=%d provider=%v restored=%v", restarted.vibe.visible, restarted.vibe.loading, restarted.vibe.cursor, restarted.vibe.provider, restarted.vibe.restored)
		}
		restarted.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyUp})
		restarted.handleVibeKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		track, _ := restarted.playlist.Current()
		if track.Path != fixture.Result.Matches[0].URL || restarted.playlist.Len() != 2 {
			t.Fatal("Restored result did not play normally while preserving queue")
		}
	}
}

func TestVibeNewSearchDoesNotReplaceLastCompletedResults(t *testing.T) {
	m := keybindingTestModel()
	fixture := savedVibeFixture()
	m.vibe = vibeState{results: fixture.Result, selected: fixture.Playlist, cursor: 1}
	cmd := m.openVibeStreams()
	if cmd == nil || !m.vibe.loading || m.vibe.results != nil || m.vibe.last == nil {
		t.Fatal("Alt+R must open new playlist workflow and retain prior results separately")
	}
	if cmd = m.openLastVibeResults(); cmd != nil || m.vibe.loading || m.vibe.results.Matches[1].URL != fixture.Result.Matches[1].URL {
		t.Fatal("Alt+L should cancel playlist loading and restore prior list without processing")
	}
	m.releaseVibeProvider()
}

func TestVibeSavedRejectsCorruptUnsafeAndOversizedData(t *testing.T) {
	for _, bad := range []string{"{", `{"Version":2}`, `{"Version":1,"Result":{"Matches":[{"Name":"Bad","URL":"file:///etc/passwd"}]}}`} {
		path := filepath.Join(t.TempDir(), "saved.json")
		if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadVibeSnapshot(path, ""); err == nil {
			t.Fatal("Invalid saved data accepted")
		}
	}
	path := filepath.Join(t.TempDir(), "large.json")
	if err := os.WriteFile(path, make([]byte, 2*1024*1024+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadVibeSnapshot(path, ""); err == nil {
		t.Fatal("Oversized results accepted")
	}
}

func TestVibeSavedLegacyRecoveryDoesNotAnalyzeOrRefreshMeasurements(t *testing.T) {
	folder := t.TempDir()
	fixture := savedVibeFixture()
	fixture.Result.Name = "Previously analyzed"
	fixture.Result.PlaylistID = "chosen"
	data, _ := json.Marshal(fixture.Result)
	legacy := filepath.Join(folder, "latest.json")
	if err := os.WriteFile(legacy, data, 0600); err != nil {
		t.Fatal(err)
	}
	recovered, err := loadVibeSnapshot(filepath.Join(folder, "absent.json"), legacy)
	if err != nil || recovered.Result.MeasuredAt != 12000 || recovered.Playlist.ID != "chosen" || len(recovered.Result.Matches) != 2 {
		t.Fatalf("Failed to recover available prior analysis without changing measurements: %+v %v", recovered, err)
	}
}
