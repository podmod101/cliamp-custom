package model

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/playlist"
)

func TestVibeWorkerCancellationDeliversFinalEventAndStopsChildren(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix helper fixture")
	}
	helper := filepath.Join(t.TempDir(), "worker")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\necho '{\"progress\":\"started\"}' >&2\nsleep 30\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLIAMP_VIBE_HELPER", helper)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := startVibeWorker(ctx, commandsTestProvider{name: "Spotify"}, playlist.PlaylistInfo{}, 7)
	select {
	case event := <-events:
		if event.stage != "started" {
			t.Fatalf("Missing progress: %+v", event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Helper did not start")
	}
	cancel()
	select {
	case event := <-events:
		if !event.finished || event.err == nil || event.gen != 7 {
			t.Fatalf("Missing cancellation completion: %+v", event)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Cancellation left a helper child holding the pipe open")
	}
}

func TestVibeWorkerRejectsUnsafePlaybackTargets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix helper fixture")
	}
	helper := filepath.Join(t.TempDir(), "worker")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\necho '{\"matches\":[{\"name\":\"bad\",\"url\":\"file:///etc/passwd\"}]}'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLIAMP_VIBE_HELPER", helper)
	_, err := runVibeWorker(context.Background(), commandsTestProvider{name: "Spotify"}, playlist.PlaylistInfo{}, func(string) {})
	if err == nil {
		t.Fatal("Non-stream URL accepted")
	}
}

func TestVibeWorkerDeliversValidatedPreviewBeforeCompletion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix helper fixture")
	}
	helper := filepath.Join(t.TempDir(), "worker")
	fixture := "#!/bin/sh\n" +
		"echo '{\"preview\":{\"partial\":true,\"matches\":[{\"name\":\"Verified\",\"url\":\"https://station.example/live\",\"score\":0.5}]}}' >&2\n" +
		"echo '{\"preview\":{\"matches\":[{\"name\":\"Unsafe\",\"url\":\"file:///etc/passwd\"}]}}' >&2\n" +
		"echo '{\"matches\":[{\"name\":\"Verified\",\"url\":\"https://station.example/live\",\"score\":0.5}]}'\n"
	if err := os.WriteFile(helper, []byte(fixture), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLIAMP_VIBE_HELPER", helper)
	previews := 0
	result, err := runVibeWorker(context.Background(), commandsTestProvider{name: "Spotify"}, playlist.PlaylistInfo{},
		func(string) {}, vibeWorkerOptions{preview: func(result *vibeResult) {
			previews++
			if result.Matches[0].Name != "Verified" || !result.Partial {
				t.Fatal("Wrong preview")
			}
		}})
	if err != nil || result == nil || previews != 1 {
		t.Fatalf("Progressive result protocol failed: previews=%d err=%v", previews, err)
	}
}
