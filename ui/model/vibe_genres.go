package model

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/ui"
)

type vibeGenre struct{ ID, Name string }

func (m *Model) handleVibeGenreKey(key string) tea.Cmd {
	count := len(m.vibe.results.Genres) + 1
	switch key {
	case "esc", "g":
		m.vibe.choosingGenre = false
	case "up", "k":
		m.vibe.genreCursor = max(0, m.vibe.genreCursor-1)
	case "down", "j":
		m.vibe.genreCursor = min(count-1, m.vibe.genreCursor+1)
	case "enter":
		genre := ""
		if m.vibe.genreCursor > 0 && m.vibe.genreCursor <= len(m.vibe.results.Genres) {
			genre = m.vibe.results.Genres[m.vibe.genreCursor-1].ID
		}
		m.vibe.choosingGenre = false
		if genre == m.vibe.genre {
			return nil
		}
		m.vibe.genre = genre
		// A new focused search uses retained reference embeddings, with a normal
		// candidate budget. Old unfiltered matches must not appear under its label.
		previous := m.vibe.results
		cmd := m.beginVibeSearch(m.vibe.selected, false)
		// Keep the prior verified list under its own unchanged genre label until
		// focused results arrive; failure must not destroy an expensive search.
		m.vibe.results = previous
		return cmd
	}
	return nil
}

func (m Model) renderVibeGenrePicker(width, height int) string {
	lines := []string{"Choose a genre from this playlist", "↑↓ browse · Enter search · Esc cancel", ""}
	lines = append(lines, cursorLine("Playlist blend", m.vibe.genreCursor == 0))
	for i, genre := range m.vibe.results.Genres {
		lines = append(lines, cursorLine(truncate(genre.Name, width-2), m.vibe.genreCursor == i+1))
	}
	lines = append(lines, "", "Reuses playlist audio; searches and verifies streams for this genre.")
	return ui.FitRect(strings.Join(lines, "\n"), max(1, m.width), max(1, m.height))
}
