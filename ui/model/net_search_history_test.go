package model

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/cliamp/internal/searchhistory"
)

func historyKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func TestYouTubeHistoryNavigationPreservesDraftAndStopsAtBounds(t *testing.T) {
	m := Model{netSearch: netSearchState{
		query: "unfinished draft", history: []string{"newest", "older", "oldest"},
	}}
	for _, want := range []string{"newest", "older", "oldest", "oldest"} {
		m.handleNetSearchInputKey(historyKey(tea.KeyUp))
		if m.netSearch.query != want {
			t.Fatalf("Up = %q, want %q", m.netSearch.query, want)
		}
	}
	for _, want := range []string{"older", "newest", "unfinished draft", "unfinished draft"} {
		m.handleNetSearchInputKey(historyKey(tea.KeyDown))
		if m.netSearch.query != want {
			t.Fatalf("Down = %q, want %q", m.netSearch.query, want)
		}
	}
}

func TestYouTubeHistoryRecallIsEditableAndDoesNotMutateStoredQuery(t *testing.T) {
	m := Model{netSearch: netSearchState{history: []string{"日本の音楽"}}}
	m.handleNetSearchInputKey(historyKey(tea.KeyUp))
	m.handleNetSearchInputKey(tea.KeyPressMsg{Code: '!', Text: "!"})
	if m.netSearch.query != "日本の音楽!" || m.netSearch.history[0] != "日本の音楽" {
		t.Fatalf("edited recall = %+v", m.netSearch)
	}
	m.handleNetSearchInputKey(historyKey(tea.KeyDown))
	if m.netSearch.query != "" {
		t.Fatalf("empty draft was lost: %q", m.netSearch.query)
	}
}

func TestYouTubeHistoryEmptyLoadingAndSoundCloud(t *testing.T) {
	for _, state := range []netSearchState{
		{query: "draft"},
		{query: "draft", history: []string{"older"}, loading: true},
		{query: "draft", history: []string{"older"}, soundcloud: true},
	} {
		m := Model{netSearch: state}
		m.handleNetSearchInputKey(historyKey(tea.KeyUp))
		m.handleNetSearchInputKey(historyKey(tea.KeyDown))
		if m.netSearch.query != "draft" {
			t.Fatalf("unexpected recall: %+v", m.netSearch)
		}
	}
}

func TestYouTubeHistoryPersistsSubmittedSearchesAndReopens(t *testing.T) {
	t.Setenv("CLIAMP_CONFIG_DIR", t.TempDir())
	m := Model{}
	m.openProviderSearchWith(nil)
	// Don't execute the returned network commands; test the real submit handler.
	for i := range 22 {
		m.netSearch.loading = false
		m.netSearch.query = fmt.Sprintf("  search %d  ", i)
		if cmd := m.handleNetSearchInputKey(historyKey(tea.KeyEnter)); cmd == nil {
			t.Fatal("submission did not schedule a search")
		}
	}
	if m.netSearch.request != "ytsearch10:search 21" {
		t.Fatalf("search request = %q", m.netSearch.request)
	}
	// Repeated Enter while loading must not add another entry.
	m.handleNetSearchInputKey(historyKey(tea.KeyEnter))
	queries, err := searchhistory.Load()
	if err != nil || len(queries) != 20 || queries[19] != "search 2" {
		t.Fatalf("persisted history = %v, %v", queries, err)
	}
	fresh := Model{}
	fresh.openProviderSearchWith(nil)
	fresh.handleNetSearchInputKey(historyKey(tea.KeyUp))
	if fresh.netSearch.query != "search 21" {
		t.Fatalf("reopened history = %q", fresh.netSearch.query)
	}
	fresh.handleNetSearchInputKey(historyKey(tea.KeyEnter))
	queries, err = searchhistory.Load()
	if err != nil || queries[0] != "search 21" || len(queries) != 20 {
		t.Fatalf("recalled query was not resubmitted: %v, %v", queries, err)
	}
}

func TestYouTubeHistorySkipsBlankAndSoundCloudSubmissions(t *testing.T) {
	t.Setenv("CLIAMP_CONFIG_DIR", t.TempDir())
	m := Model{}
	m.handleNetSearchInputKey(historyKey(tea.KeyEnter))
	m.netSearch.query = "soundcloud query"
	m.netSearch.soundcloud = true
	m.handleNetSearchInputKey(historyKey(tea.KeyEnter))
	queries, err := searchhistory.Load()
	if err != nil || len(queries) != 0 {
		t.Fatalf("unexpected YouTube history = %v, %v", queries, err)
	}
}
