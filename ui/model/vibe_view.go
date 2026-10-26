package model

import (
	"fmt"
	"strings"

	"github.com/bjarneo/cliamp/ui"
)

func (m Model) renderVibeStreams() string {
	if m.vibe.replacePrompt {
		return m.renderVibeReplacement()
	}
	width, height := max(20, m.width-6), max(6, m.height-4)
	var errorLines []string
	if m.vibe.err != "" {
		errorLines = wrapText("Error: "+m.vibe.err, width)
	}
	heading := "New Spotify playlist stream search (Alt+R)"
	if m.vibe.selected.ID != "" {
		heading = "Playlist stream search (Alt+L)"
	}
	if m.vibe.searching {
		heading = "Stream search running (Alt+L)"
	}
	if m.vibe.results != nil {
		heading = "Analyzed stream results (Alt+L)"
	}
	controls := "↑↓ browse · Enter select · / filter"
	if m.vibe.loading {
		controls = "s cancel search · Enter plays available results"
	} else if m.vibe.results != nil {
		controls = "↑↓ browse · Enter play · g genre · d deeper · b playlists"
	}
	lines := []string{heading,
		"Esc/m music · Alt+L reopen · Alt+R new search", controls, ""}
	if m.vibe.choosingGenre {
		return m.renderVibeGenrePicker(width, height)
	}
	switch {
	case m.vibe.loading && m.vibe.results == nil:
		lines = append(lines, m.vibe.title, spinnerFrame()+" "+m.vibe.progress,
			"Esc or m: minimize and keep searching · s: cancel search")
	case m.vibe.auth:
		lines = append(lines, "Press Enter to sign in to Spotify for CLIamp.",
			"Your ChatGPT Spotify connection is separate.")
	case m.vibe.results == nil && m.vibe.selected.ID != "":
		lines = append(lines, m.vibe.title, "Search ended without verified results.",
			m.vibe.progress, "Alt+R: new search · Esc/m: music")
	case m.vibe.results != nil:
		result := m.vibe.results
		lines = append(lines, m.vibe.title, result.Summary,
			fmt.Sprintf("%d playlist samples · %d found · %d shortlisted · %d audio verified", result.Sampled, result.Searched, result.Shortlisted, result.Compared),
			"Found worldwide · audio-compatible variety · raw cosine, not confidence")
		if m.vibe.restored {
			lines = append(lines, "Saved results: original measurements; no new processing.")
		}
		genreLabel := "Playlist blend"
		for _, genre := range result.Genres {
			if genre.ID == result.Genre {
				genreLabel = genre.Name
			}
		}
		lines = append(lines, fmt.Sprintf("Genre: %s · %d excerpts excluded by audio genre check", genreLabel, result.Coverage.GenreRejected))
		if m.vibe.loading {
			lines = append(lines, spinnerFrame()+" "+m.vibe.progress, "Enter plays and keeps searching · s stops search and keeps results")
		} else {
			lines = append(lines, fmt.Sprintf("Catalog coverage: %d countries / %d languages · d deeper · Enter play", result.Coverage.Countries, result.Coverage.Languages))
		}
		for _, warning := range result.Warnings[:min(2, len(result.Warnings))] {
			lines = append(lines, "Note: "+warning)
		}
		rows := max(1, height-len(lines)-len(errorLines)-4)
		start := max(0, m.vibe.cursor-rows+1)
		for i := start; i < len(result.Matches) && i < start+rows; i++ {
			item := result.Matches[i]
			label := fmt.Sprintf("%d. raw %.3f [%s] %s", i+1, item.Score, item.Source, item.Name)
			lines = append(lines, cursorLine(truncate(label, width-2), i == m.vibe.cursor))
		}
		if m.vibe.cursor < len(result.Matches) {
			item := result.Matches[m.vibe.cursor]
			lines = append(lines, truncate("Audio: "+item.Reason+" ["+item.AudioGenreLabel+"]", width),
				fmt.Sprintf("%ds · %s · %s", item.SampleSeconds, item.Country, item.Uncertainty),
				truncate("Metadata prediction: "+item.Prediction, width))
		}
	default:
		query := m.vibe.query
		if m.vibe.filtering {
			query += "_"
		}
		lines = append(lines, "Playlist filter: "+query)
		lists := m.vibeVisibleLists()
		rows := max(1, height-len(lines)-len(errorLines)-2)
		start := max(0, m.vibe.cursor-rows+1)
		for i := start; i < len(lists) && i < start+rows; i++ {
			lines = append(lines, cursorLine(truncate(lists[i].Name, width-2), i == m.vibe.cursor))
		}
		if len(lists) == 0 {
			if m.vibe.err != "" {
				lines = append(lines, "Could not load Spotify playlists. Press r to retry.")
			} else {
				lines = append(lines, "No playlists found. Press r to retry.")
			}
		}
	}
	lines = append(lines, errorLines...)
	for i := range lines {
		lines[i] = truncate(lines[i], width)
	}
	return ui.FitRect(strings.Join(lines, "\n"), max(1, m.width), max(1, m.height))
}
