package model

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bjarneo/cliamp/playlist"
)

type vibeMetadataProvider struct{ commandsTestProvider }

func (vibeMetadataProvider) Tracks(string) ([]playlist.Track, error) {
	return nil, errors.New("full playlist must not be loaded")
}
func (vibeMetadataProvider) MatchingTracks(context.Context, string) ([]playlist.Track, error) {
	return []playlist.Track{{Title: "Fusion example", Artist: "Artist", DurationSecs: 180}}, nil
}
func (vibeMetadataProvider) MatchingPlaylistDescription(string) string {
	return "Electronic blues with glitch hop"
}

func TestVibeWorkerPassesBoundedMetadataDescriptionAndSelectedGenre(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix helper fixture")
	}
	folder := t.TempDir()
	helper, input := filepath.Join(folder, "helper"), filepath.Join(folder, "input.json")
	fixture := "#!/bin/sh\ncat > \"$CLIAMP_VIBE_TEST_INPUT\"\necho '{\"matches\":[{\"name\":\"Verified\",\"url\":\"https://station.example/live\"}]}'\n"
	if err := os.WriteFile(helper, []byte(fixture), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLIAMP_VIBE_HELPER", helper)
	t.Setenv("CLIAMP_VIBE_TEST_INPUT", input)
	_, err := runVibeWorker(context.Background(), vibeMetadataProvider{}, playlist.PlaylistInfo{ID: "selected", TrackCount: 11000}, func(string) {}, vibeWorkerOptions{genre: "edm"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Description, Genre string
		Tracks             []any
		Total              int `json:"playlist_total"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Description != "Electronic blues with glitch hop" || got.Genre != "edm" || got.Total != 11000 || len(got.Tracks) != 1 {
		t.Fatalf("Lost playlist intent or bounded metadata: %+v", got)
	}
}
