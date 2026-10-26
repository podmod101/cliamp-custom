package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/internal/appdir"
	"github.com/bjarneo/cliamp/internal/fileutil"
	"github.com/bjarneo/cliamp/playlist"
)

type vibeSnapshot struct {
	Version  int
	SavedAt  int64
	Playlist playlist.PlaylistInfo
	Result   *vibeResult
	Cursor   int
}

func defaultVibeResultsPath() string {
	dir, err := appdir.Dir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "stream-matching-results.json")
}

func readVibeJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 2*1024*1024+1))
	if err != nil {
		return err
	}
	if len(data) > 2*1024*1024 {
		return fmt.Errorf("saved stream results exceed size limit")
	}
	return json.Unmarshal(data, target)
}

func loadVibeSnapshot(path, legacy string) (*vibeSnapshot, error) {
	var saved vibeSnapshot
	err := readVibeJSON(path, &saved)
	if errors.Is(err, os.ErrNotExist) && legacy != "" {
		var result vibeResult
		if err = readVibeJSON(legacy, &result); err == nil {
			saved = vibeSnapshot{Version: 1, SavedAt: result.MeasuredAt, Result: &result,
				Playlist: playlist.PlaylistInfo{ID: result.PlaylistID, Name: result.Name}}
		}
	}
	if err != nil {
		return nil, err
	}
	if saved.Version != 1 || saved.Result == nil {
		return nil, fmt.Errorf("invalid saved stream results")
	}
	if err = validateVibeResult(saved.Result); err != nil {
		return nil, err
	}
	saved.Cursor = max(0, min(saved.Cursor, len(saved.Result.Matches)-1))
	return &saved, nil
}

func (m *Model) rememberVibeResults() {
	if m.vibe.results == nil || validateVibeResult(m.vibe.results) != nil {
		return
	}
	result := *m.vibe.results
	m.vibe.last = &vibeSnapshot{Version: 1, SavedAt: time.Now().Unix(), Playlist: m.vibe.selected, Result: &result, Cursor: m.vibe.cursor}
	if m.vibeResultsPath == "" {
		return
	} // Pure in-memory models/tests do not touch user state.
	data, err := json.Marshal(m.vibe.last)
	if err == nil {
		err = fileutil.WriteFileAtomic(m.vibeResultsPath, data, 0o600)
	}
	if err != nil {
		m.status.Warning("Could not save stream results; retained in this session", statusTTLDefault)
	}
}

func (m *Model) openLastVibeResults() tea.Cmd {
	if m.vibe.searching || m.vibe.results != nil || m.vibe.selected.ID != "" {
		m.vibe.visible = true
		m.vibe.replacePrompt = false
		m.vibe.filtering = false
		m.vibe.choosingGenre = false
		return nil // The active job already owns one event waiter. Do not restart it.
	}
	if !m.vibe.loading {
		m.rememberVibeResults()
	}
	if m.vibe.last == nil && m.vibeResultsPath != "" {
		legacy := ""
		// Legacy worker cache is only used for the standard config, never to mix
		// a separate explicit profile with another profile's results.
		home, homeErr := os.UserHomeDir()
		if homeErr == nil && filepath.Clean(filepath.Dir(m.vibeResultsPath)) == filepath.Join(home, ".config", "cliamp") {
			if cache, err := os.UserCacheDir(); err == nil {
				legacy = filepath.Join(cache, "cliamp", "vibe", "latest.json")
			}
		}
		saved, err := loadVibeSnapshot(m.vibeResultsPath, legacy)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) && m.vibe.loading {
				m.vibe.visible = true
				return nil // No previous results: return to the pending playlist picker.
			}
			message := "Could not restore saved stream results"
			if errors.Is(err, os.ErrNotExist) {
				message = "No saved stream results yet; Alt+R starts a new search"
			}
			m.status.Warning(message, statusTTLDefault)
			return nil
		}
		m.vibe.last = saved
	}
	if m.vibe.last == nil {
		if m.vibe.loading {
			m.vibe.visible = true
			return nil
		}
		m.status.Warning("No saved stream results yet; Alt+R starts a new search", statusTTLDefault)
		return nil
	}
	if m.vibe.cancel != nil {
		m.vibe.cancel()
	}
	saved := m.vibe.last
	m.vibe.gen++
	m.vibe.visible, m.vibe.loading, m.vibe.auth, m.vibe.filtering = true, false, false, false
	m.vibe.choosingGenre = false
	m.vibe.results = saved.Result
	m.vibe.cursor = saved.Cursor
	m.vibe.selected = saved.Playlist
	m.vibe.title = saved.Playlist.Name
	if saved.Result.MeasuredAt > 0 {
		m.vibe.title += " · measured " + time.Unix(saved.Result.MeasuredAt, 0).Local().Format("Jan 2 15:04")
	} else if saved.SavedAt > 0 {
		m.vibe.title += " · saved " + time.Unix(saved.SavedAt, 0).Local().Format("Jan 2 15:04")
	}
	m.vibe.genre = saved.Result.Genre
	m.vibe.err = ""
	m.vibe.restored = true
	return nil // Deliberately no playlist/provider/model/capture command.
}
