// Package searchhistory stores the last 20 submitted YouTube search queries.
package searchhistory

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/bjarneo/cliamp/internal/appdir"
	"github.com/bjarneo/cliamp/internal/fileutil"
)

const Limit = 20

func historyPath() (string, error) {
	dir, err := appdir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "youtube-search-history.json"), nil
}

// Load returns queries newest first. A missing file is an empty history.
func Load() ([]string, error) {
	path, err := historyPath()
	if err != nil {
		return nil, err
	}
	return load(path)
}

func load(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var queries []string
	if err := json.Unmarshal(data, &queries); err != nil {
		return nil, err
	}
	if len(queries) > Limit {
		queries = queries[:Limit]
	}
	return queries, nil
}

// Record serializes updates across player instances and atomically saves them.
// It leaves the existing file intact if loading or writing fails.
func Record(query string) ([]string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return Load()
	}
	path, err := historyPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	unlock, err := fileutil.LockFile(path + ".lock")
	if err != nil {
		return nil, err
	}
	defer unlock()
	queries, err := load(path)
	if err != nil {
		return nil, err
	}
	queries = append([]string{query}, queries...)
	if len(queries) > Limit {
		queries = queries[:Limit]
	}
	data, err := json.MarshalIndent(queries, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := fileutil.WriteFileAtomic(path, append(data, '\n'), 0o600); err != nil {
		return nil, err
	}
	return queries, nil
}
