package model

import (
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/resolve"
)

// saveYouTubeToMusic downloads the current item's entire audio independently
// of playback position. Unknown duration is allowed; yt-dlp checks live status.
func (m *Model) saveYouTubeToMusic() tea.Cmd {
	if m.save.pendingDownloads > 0 {
		m.status.Warning("A download is already in progress", statusTTLShort)
		return nil
	}
	track, idx := m.currentPlaybackTrack()
	if idx < 0 {
		m.status.Warning("Nothing to save", statusTTLShort)
		return nil
	}
	if !playlist.IsYouTubeURL(track.Path) && !playlist.IsYouTubeMusicURL(track.Path) {
		m.status.Warning("Alt+S saves YouTube audio to ~/Music", statusTTLShort)
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		m.status.Errorf(statusTTLShort, "Save failed: %s", err)
		return nil
	}
	saveDir := filepath.Join(home, "Music")
	m.status.Clear()
	m.save.startDownload()
	return func() tea.Msg {
		result, err := resolve.DownloadYouTubeAudio(track.Path, saveDir)
		return ytdlSavedMsg{path: result.PlaylistPath, tracks: result.Tracks, method: result.Method, warning: result.Warning, identified: result.Identified, identification: result.Identification, err: err}
	}
}
