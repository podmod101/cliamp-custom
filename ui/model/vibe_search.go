package model

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/playlist"
)

func (m *Model) applyVibeResults(result *vibeResult) {
	selected := ""
	if m.vibe.results != nil && m.vibe.cursor < len(m.vibe.results.Matches) {
		selected = m.vibe.results.Matches[m.vibe.cursor].URL
	}
	m.vibe.results = result
	m.vibe.cursor = min(m.vibe.cursor, max(0, len(result.Matches)-1))
	for i, item := range result.Matches {
		if item.URL == selected {
			m.vibe.cursor = i
			break
		}
	}
}

func (m *Model) stopVibeSearch() {
	if m.vibe.cancel != nil {
		m.vibe.cancel()
	}
	m.vibe.loading = false
	m.vibe.searching = false
	m.vibe.events = nil
	m.vibe.replacePrompt = false
	m.vibe.gen++
	if m.vibe.results != nil && m.vibe.results.Partial {
		m.vibe.results.Partial = false
		m.vibe.results.Warnings = append(m.vibe.results.Warnings, "Search stopped early; showing verified matches so far")
	}
	m.vibe.progress = "Search cancelled; verified results retained"
	m.rememberVibeResults()
}

func (m *Model) beginVibeSearch(item playlist.PlaylistInfo, deeper bool) tea.Cmd {
	if m.vibe.searching {
		m.status.Warning("Search already running; use s to cancel before changing genre", statusTTLDefault)
		return nil
	}
	m.rememberVibeResults()
	m.ensureVibeProvider()
	m.vibe.restored = false
	if m.vibe.cancel != nil {
		m.vibe.cancel()
	}
	m.vibe.gen++
	m.vibe.loading = true
	m.vibe.searching = true
	m.vibe.visible = true
	m.vibe.err = ""
	m.vibe.title = item.Name
	m.vibe.selected = item
	if !deeper {
		m.vibe.results = nil
		m.vibe.cursor = 0
	}
	m.vibe.progress = "Preparing playlist sound and local AI worldwide discovery..."
	deadline := 15 * time.Minute
	if deeper {
		deadline = 20 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	m.vibe.cancel = cancel
	m.vibe.events = startVibeWorker(ctx, m.vibe.provider, item, m.vibe.gen,
		vibeWorkerOptions{deeper: deeper, genre: m.vibe.genre})
	return waitVibeEvent(m.vibe.events)
}
