package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/cliamp/external/spotify"
	"github.com/bjarneo/cliamp/playlist"
)

type vibeMatch struct {
	Name, URL, Source, Reason                  string
	Score                                      float64
	Prediction, Uncertainty, Country, Language string
	SampleSeconds                              int    `json:"sample_seconds"`
	AudioGenreLabel                            string `json:"audio_genre_label"`
}

type vibeResult struct {
	Name                  string
	PlaylistID            string `json:"playlist_id"`
	MeasuredAt            int64  `json:"at"`
	Matches               []vibeMatch
	Summary, Error        string
	Sampled, Compared     int
	Warnings              []string
	Searched, Shortlisted int
	Partial               bool
	Genre                 string
	Genres                []vibeGenre
	Coverage              struct {
		Attempted     int
		Countries     int    `json:"countries_measured"`
		Languages     int    `json:"languages_measured"`
		StopReason    string `json:"stop_reason"`
		GenreRejected int    `json:"genre_rejected"`
	}
}

type vibeEvent struct {
	gen      uint64
	stage    string
	lists    []playlist.PlaylistInfo
	result   *vibeResult
	err      error
	finished bool
}

type vibeState struct {
	visible, loading, auth, filtering bool
	gen                               uint64
	provider                          playlist.Provider
	ownedProvider                     bool
	lists                             []playlist.PlaylistInfo
	results                           *vibeResult
	cursor                            int
	query, title, progress, err       string
	cancel                            context.CancelFunc
	events                            <-chan vibeEvent
	selected                          playlist.PlaylistInfo
	genre                             string
	choosingGenre                     bool
	genreCursor                       int
	last                              *vibeSnapshot
	restored                          bool
	searching                         bool
	replacePrompt                     bool
}

func (m *Model) openVibeStreams() tea.Cmd {
	if m.vibe.searching {
		m.vibe.replacePrompt = true
		m.vibe.visible = true
		return nil
	}
	m.rememberVibeResults()
	gen := m.vibe.gen + 1
	if m.vibe.cancel != nil {
		m.vibe.cancel()
	}
	m.ensureVibeProvider()
	m.vibe = vibeState{visible: true, loading: true, gen: gen, provider: m.vibe.provider,
		ownedProvider: m.vibe.ownedProvider, last: m.vibe.last, progress: "Loading your Spotify playlists..."}
	return m.loadVibeLists(false)
}

func (m *Model) ensureVibeProvider() {
	var source playlist.Provider
	owned := false
	for _, entry := range m.providers {
		if entry.Provider != nil && strings.EqualFold(entry.Provider.Name(), "Spotify") {
			source = entry.Provider
			break
		}
	}
	if source == nil {
		source, owned = m.vibe.provider, m.vibe.ownedProvider
	}
	if source == nil {
		// Recommendation browsing works even when Spotify playback wasn't enabled.
		// Uses CLIamp's own OAuth flow, never ChatGPT's account credentials.
		source = spotify.New(nil, spotify.DefaultClientID, 320)
		owned = true
	}
	if m.vibe.ownedProvider && m.vibe.provider != source {
		m.releaseVibeProvider()
	}
	m.vibe.provider, m.vibe.ownedProvider = source, owned
}

func (m *Model) loadVibeLists(auth bool) tea.Cmd {
	if m.vibe.cancel != nil {
		m.vibe.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.vibe.cancel = cancel
	return vibeListsCmd(ctx, m.vibe.provider, m.vibe.gen, auth)
}

func vibeListsCmd(ctx context.Context, source playlist.Provider, gen uint64, auth bool) tea.Cmd {
	return func() tea.Msg {
		if auth {
			if authenticator, ok := source.(interface{ AuthenticateContext(context.Context) error }); ok {
				if err := authenticator.AuthenticateContext(ctx); err != nil {
					return vibeEvent{gen: gen, err: err, finished: true}
				}
			} else {
				authenticator, ok := source.(playlist.Authenticator)
				if !ok {
					return vibeEvent{gen: gen, err: errors.New("Spotify sign-in is unavailable"), finished: true}
				}
				if err := authenticator.Authenticate(); err != nil {
					return vibeEvent{gen: gen, err: err, finished: true}
				}
			}
		}
		loadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		var lists []playlist.PlaylistInfo
		var err error
		if loader, ok := source.(interface {
			MatchingPlaylists(context.Context) ([]playlist.PlaylistInfo, error)
		}); ok {
			lists, err = loader.MatchingPlaylists(loadCtx)
		} else {
			lists, err = source.Playlists()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("Spotify playlist request timed out. Check your connection, then press r to retry")
		}
		return vibeEvent{gen: gen, lists: lists, err: err, finished: true}
	}
}

func (m *Model) releaseVibeProvider() {
	if m.vibe.ownedProvider {
		if closer, ok := m.vibe.provider.(interface{ Close() }); ok {
			closer.Close()
		}
		m.vibe.ownedProvider = false
	}
}

func (m *Model) closeVibeStreams() {
	m.stopVibeSearch()
	m.vibe.visible = false
}

func (m *Model) handleVibeEvent(event vibeEvent) tea.Cmd {
	if event.gen != m.vibe.gen {
		return nil
	}
	if event.stage != "" {
		m.vibe.progress = event.stage
	}
	if event.result != nil && !event.finished {
		m.applyVibeResults(event.result)
	}
	if !event.finished {
		return waitVibeEvent(m.vibe.events)
	}
	m.vibe.loading = false
	m.vibe.searching = false
	m.vibe.events = nil
	if m.vibe.cancel != nil {
		m.vibe.cancel()
	}
	if event.err != nil {
		m.vibe.auth = errors.Is(event.err, playlist.ErrNeedsAuth)
		m.vibe.err = event.err.Error()
		return nil
	}
	m.vibe.auth = false
	m.vibe.err = ""
	if event.result != nil {
		m.applyVibeResults(event.result)
		m.rememberVibeResults()
	} else {
		m.vibe.lists = nil
		for _, item := range event.lists {
			if !strings.HasPrefix(item.ID, "spotify:album:") {
				m.vibe.lists = append(m.vibe.lists, item)
			}
		}
	}
	if event.result == nil {
		m.vibe.cursor = 0
	}
	return nil
}

func (m Model) vibeVisibleLists() []playlist.PlaylistInfo {
	var out []playlist.PlaylistInfo
	for _, item := range m.vibe.lists {
		if strings.Contains(strings.ToLower(item.Name), strings.ToLower(m.vibe.query)) {
			out = append(out, item)
		}
	}
	return out
}

func (m *Model) handleVibeKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	if key == "alt+r" {
		return m.openVibeStreams()
	}
	if key == "alt+l" {
		return m.openLastVibeResults()
	}
	if m.vibe.replacePrompt {
		return m.handleVibeReplacement(key)
	}
	if key == "m" && !m.vibe.filtering {
		m.minimizeVibeSearch()
		return nil
	}
	if key == "s" && !m.vibe.filtering && m.vibe.loading {
		m.stopVibeSearch()
		return nil
	}
	if m.vibe.choosingGenre {
		return m.handleVibeGenreKey(key)
	}
	if key == "esc" {
		if m.vibe.filtering {
			m.vibe.filtering = false
			return nil
		}
		m.minimizeVibeSearch()
		return nil
	}
	if m.vibe.loading && m.vibe.results == nil {
		return nil
	}
	if m.vibe.filtering {
		switch key {
		case "enter":
			m.vibe.filtering = false
		case "backspace":
			runes := []rune(m.vibe.query)
			if len(runes) > 0 {
				m.vibe.query = string(runes[:len(runes)-1])
			}
		default:
			if len([]rune(key)) == 1 && len(m.vibe.query) < 200 {
				m.vibe.query += key
			}
		}
		m.vibe.cursor = 0
		return nil
	}
	count := len(m.vibeVisibleLists())
	if m.vibe.results != nil {
		count = len(m.vibe.results.Matches)
	}
	switch key {
	case "up", "k":
		m.vibe.cursor = max(0, m.vibe.cursor-1)
	case "down", "j":
		m.vibe.cursor = min(max(0, count-1), m.vibe.cursor+1)
	case "home":
		m.vibe.cursor = 0
	case "end", "G":
		m.vibe.cursor = max(0, count-1)
	case "/":
		if m.vibe.results == nil {
			m.vibe.filtering = true
		}
	case "backspace", "b":
		return m.openVibeStreams()
	case "r":
		if m.vibe.results == nil {
			if refresher, ok := m.vibe.provider.(playlist.Refresher); ok {
				refresher.Refresh()
			}
			m.vibe.loading = true
			m.vibe.gen++
			m.vibe.err = ""
			m.vibe.progress = "Loading your Spotify playlists..."
			return m.loadVibeLists(false)
		}
	case "d":
		if m.vibe.results != nil && !m.vibe.loading && m.vibe.selected.ID != "" {
			return m.beginVibeSearch(m.vibe.selected, true)
		}
	case "g":
		if m.vibe.results != nil && len(m.vibe.results.Genres) > 0 && !m.vibe.searching {
			m.vibe.choosingGenre = true
			m.vibe.genreCursor = 0
		}
	case "enter":
		if m.vibe.auth {
			m.vibe.loading = true
			m.vibe.progress = "Complete Spotify sign-in in your browser..."
			m.vibe.gen++
			return m.loadVibeLists(true)
		}
		if count == 0 || m.vibe.cursor >= count {
			return nil
		}
		if m.vibe.results != nil {
			match := m.vibe.results.Matches[m.vibe.cursor]
			m.minimizeVibeSearch()
			return m.playTrackImmediate(playlist.Track{Path: match.URL, Title: match.Name,
				Artist: match.Source, Stream: true, Realtime: true})
		}
		item := m.vibeVisibleLists()[m.vibe.cursor]
		return m.beginVibeSearch(item, false)
	}
	return nil
}
