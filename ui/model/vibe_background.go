package model

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/ui"
)

func (m *Model) minimizeVibeSearch() {
	m.rememberVibeResults()
	m.vibe.visible = false
	m.vibe.filtering = false
	m.vibe.choosingGenre = false
	m.vibe.replacePrompt = false
	// Keep the context, generation and existing event waiter untouched.
}

func (m *Model) handleVibeReplacement(key string) tea.Cmd {
	switch key {
	case "y", "Y":
		m.stopVibeSearch()
		return m.openVibeStreams()
	case "n", "N", "esc":
		m.vibe.replacePrompt = false
	case "m":
		m.minimizeVibeSearch()
	}
	return nil
}

func (m Model) renderVibeReplacement() string {
	lines := []string{"Start a new playlist search?", ""}
	if m.vibe.searching {
		lines = append(lines, "A stream search is still running: "+m.vibe.title,
			"Starting another search will cancel this work.")
	} else {
		lines = append(lines, "The previous search has finished. Its results will be retained.")
	}
	lines = append(lines, "", "y: start new search · n/Esc: keep current search · m: minimize")
	return ui.FitRect(strings.Join(lines, "\n"), max(1, m.width), max(1, m.height))
}

func (m Model) vibeIndicator() string {
	count := 0
	if m.vibe.results != nil {
		count = len(m.vibe.results.Matches)
	}
	if m.vibe.searching {
		return fmt.Sprintf("AI searching (%d) · Alt+L", count)
	}
	if m.vibe.err != "" && m.vibe.selected.ID != "" {
		return "AI needs attention · Alt+L"
	}
	if count == 0 && m.vibe.last != nil && m.vibe.last.Result != nil {
		count = len(m.vibe.last.Result.Matches)
	}
	if count > 0 {
		return fmt.Sprintf("AI results ready (%d) · Alt+L", count)
	}
	return ""
}
