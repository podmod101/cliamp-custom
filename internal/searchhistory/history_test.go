package searchhistory

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRecordKeepsLast20AcrossLoads(t *testing.T) {
	t.Setenv("CLIAMP_CONFIG_DIR", t.TempDir())
	queries, err := Load()
	if err != nil || len(queries) != 0 {
		t.Fatalf("missing history = %v, %v", queries, err)
	}
	for i := 0; i < 25; i++ {
		if _, err := Record(fmt.Sprintf("  search %d  ", i)); err != nil {
			t.Fatal(err)
		}
	}
	queries, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 20 || queries[0] != "search 24" || queries[19] != "search 5" {
		t.Fatalf("history = %v", queries)
	}
	queries, err = Record(" \t ")
	if err != nil || len(queries) != 20 || queries[0] != "search 24" {
		t.Fatalf("blank submission changed history: %v, %v", queries, err)
	}
	path, _ := historyPath()
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("history must be private: %v, %v", info, err)
	}
}

func TestRecordPreservesUnreadableHistory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLIAMP_CONFIG_DIR", dir)
	path := filepath.Join(dir, "youtube-search-history.json")
	const corrupt = "not valid json"
	if err := os.WriteFile(path, []byte(corrupt), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Record("new query"); err == nil {
		t.Fatal("expected load error")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != corrupt {
		t.Fatalf("existing history was changed: %q, %v", data, err)
	}
}

func TestConcurrentRecordDoesNotLoseQueries(t *testing.T) {
	t.Setenv("CLIAMP_CONFIG_DIR", t.TempDir())
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			if _, err := Record(fmt.Sprintf("query %d", i)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	queries, err := Load()
	if err != nil || len(queries) != 20 {
		t.Fatalf("concurrent history = %v, %v", queries, err)
	}
}
