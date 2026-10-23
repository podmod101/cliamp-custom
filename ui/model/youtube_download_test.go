package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/cliamp/playlist"
)

func TestAltSSavesCurrentYouTubeRegardlessOfDuration(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, source := range []string{"https://youtube.com/watch?v=archive1234", "https://music.youtube.com/watch?v=archive1234"} {
		m := Model{playlist: playlist.New()}
		m.playlist.Add(playlist.Track{Path: source, Stream: true})
		cmd := m.handleKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModAlt})
		if cmd == nil || m.save.pendingDownloads != 1 {
			t.Fatalf("Alt+S did not start download for %s", source)
		}
	}
}

func TestAltSRejectsEmptyAndOtherSources(t *testing.T) {
	for _, source := range []string{"", "/tmp/track.mp3", "https://soundcloud.com/user/track"} {
		m := Model{playlist: playlist.New()}
		if source != "" {
			m.playlist.Add(playlist.Track{Path: source})
		}
		if cmd := m.handleKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModAlt}); cmd != nil || m.save.pendingDownloads != 0 {
			t.Fatalf("Alt+S started download for %q", source)
		}
		if m.status.text == "" {
			t.Fatal("missing explanation")
		}
	}
}

func TestYouTubeDownloadFailureClearsActivity(t *testing.T) {
	m := Model{playlist: playlist.New()}
	// Execute the async path with a missing downloader, then handle its result.
	t.Setenv("PATH", t.TempDir())
	m.playlist.Add(playlist.Track{Path: "https://youtube.com/watch?v=archive1234"})
	t.Setenv("HOME", t.TempDir())
	msg := m.saveYouTubeToMusic()().(ytdlSavedMsg)
	if msg.err == nil {
		t.Fatal("expected downloader error")
	}
	next, _ := m.Update(msg)
	result := next.(Model)
	if result.save.pendingDownloads != 0 || !strings.Contains(result.status.text, "Download failed") {
		t.Fatalf("failed download not reported: %+v", result.save)
	}
}

func TestAltSDoesNotStartOverlappingDownloads(t *testing.T) {
	m := Model{playlist: playlist.New(), save: saveState{pendingDownloads: 1}}
	m.playlist.Add(playlist.Track{Path: "https://youtube.com/watch?v=archive1234"})
	if cmd := m.handleKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModAlt}); cmd != nil || m.save.pendingDownloads != 1 {
		t.Fatal("Alt+S started a second download")
	}
}

func TestYouTubeSaveReportsEstimatedSplitsAndFallback(t *testing.T) {
	for _, msg := range []ytdlSavedMsg{
		{path: "/Music/Show/Show.m3u8", tracks: 2, method: "silence (estimated)"},
		{path: "/Music/Show/Show.m3u8", tracks: 1, warning: "Track splitting failed; full recording kept"},
	} {
		m := Model{save: saveState{pendingDownloads: 1}}
		next, _ := m.Update(msg)
		result := next.(Model)
		want := msg.method
		if msg.warning != "" {
			want = msg.warning
		}
		if result.save.pendingDownloads != 0 || !strings.Contains(result.status.text, want) || !strings.Contains(result.status.text, msg.path) {
			t.Fatalf("missing download result: %s", result.status.text)
		}
	}
}

func TestYouTubeSaveReportsIdentificationCount(t *testing.T) {
	m := Model{save: saveState{pendingDownloads: 1}}
	next, _ := m.Update(ytdlSavedMsg{path: "/Music/Show/Show.m3u8", tracks: 3, identified: 2, identification: true, method: "YouTube chapters"})
	result := next.(Model)
	if result.save.pendingDownloads != 0 || !strings.Contains(result.status.text, "2 identified") || !strings.Contains(result.status.text, "3 track(s)") {
		t.Fatalf("missing identification summary: %s", result.status.text)
	}
}
