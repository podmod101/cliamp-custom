package resolve

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func cleanupFixture(t *testing.T) (string, []string, []audioChapter, string) {
	t.Helper()
	source := recognitionFixture(t, 6)
	duration, err := probeAudioDuration(source)
	if err != nil {
		t.Fatal(err)
	}
	segments := []audioChapter{{Title: "First", End: 3}, {Title: "Second", Start: 3, End: duration}}
	var paths []string
	var contents strings.Builder
	contents.WriteString("#EXTM3U\n")
	for i, segment := range segments {
		path, err := splitAudioTrack(source, i+1, segment)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
		relative, _ := filepath.Rel(filepath.Dir(source), path)
		contents.WriteString(filepath.ToSlash(relative) + "\n")
	}
	playlist := filepath.Join(filepath.Dir(source), "Tracks.m3u8")
	if err := os.WriteFile(playlist, []byte(contents.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return source, paths, segments, playlist
}

func TestFullRecordingCleanupRequiresCompletePlayableOutput(t *testing.T) {
	for _, scenario := range []string{"complete", "missing track", "empty track", "truncated track", "gap", "missing end", "no playlist", "partial playlist", "relative reference", "URI reference", "PLS reference", "record write failure", "symlink track"} {
		t.Run(scenario, func(t *testing.T) {
			source, paths, segments, playlist := cleanupFixture(t)
			folder := filepath.Dir(source)
			switch scenario {
			case "missing track":
				if err := os.Remove(paths[1]); err != nil {
					t.Fatal(err)
				}
			case "empty track":
				if err := os.WriteFile(paths[1], nil, 0o600); err != nil {
					t.Fatal(err)
				}
			case "truncated track":
				if err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", source, "-t", "0.5", "-c:a", "copy", paths[1]).Run(); err != nil {
					t.Fatal(err)
				}
			case "gap":
				segments[1].Start += 0.2
			case "missing end":
				segments[1].End -= 1
			case "no playlist":
				if err := os.Remove(playlist); err != nil {
					t.Fatal(err)
				}
			case "partial playlist":
				if err := os.WriteFile(playlist, []byte("Tracks/001 - First.mp3\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "relative reference", "URI reference", "PLS reference":
				reference := "./Full stream.mp3\n"
				ext := ".m3u8"
				if scenario == "URI reference" {
					reference = "file://" + strings.ReplaceAll(source, " ", "%20") + "\n"
				} else if scenario == "PLS reference" {
					reference = "[playlist]\nFile1=" + source + "\n"
					ext = ".pls"
				}
				if err := os.WriteFile(filepath.Join(folder, "Old playlist"+ext), []byte(reference), 0o600); err != nil {
					t.Fatal(err)
				}
			case "record write failure":
				if err := os.Mkdir(filepath.Join(folder, recognitionRecordName), 0o700); err != nil {
					t.Fatal(err)
				}
			case "symlink track":
				if err := os.Remove(paths[1]); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(source, paths[1]); err != nil {
					t.Fatal(err)
				}
			}
			removed, err := removeCompletedRecording(source, paths, segments)
			if removed != (scenario == "complete") || (err != nil) != (scenario == "record write failure") {
				t.Fatalf("removed=%v, error=%v", removed, err)
			}
			_, sourceErr := os.Stat(source)
			if removed != os.IsNotExist(sourceErr) {
				t.Fatalf("unexpected source state: %v", sourceErr)
			}
		})
	}
}

func TestCleanupPreservesIdentityCacheAcrossSourceRemoval(t *testing.T) {
	source, paths, segments, _ := cleanupFixture(t)
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(source)
	trackInfo, _ := os.Stat(paths[0])
	cache := recognitionRecord{Version: 1, SourceSize: info.Size(), SourceMtime: info.ModTime().UnixNano(), Tracks: []identifiedTrack{{Number: 1, Start: segments[0].Start, End: segments[0].End, Path: filepath.Join("Tracks", filepath.Base(paths[0])), Status: "identified", Song: songIdentity{Key: "123", Artist: "Artist", Title: "Song"}, Size: trackInfo.Size(), Mtime: trackInfo.ModTime().UnixNano()}}}
	encoded, _ := json.Marshal(cache)
	if err := saveRecognitionRecord(filepath.Join(filepath.Dir(source), recognitionRecordName), encoded); err != nil {
		t.Fatal(err)
	}
	removed, err := removeCompletedRecording(source, paths, segments)
	if err != nil || !removed {
		t.Fatalf("cleanup: %v, %v", removed, err)
	}
	cache = loadRecognitionRecord(source)
	if !cache.SourceRemoved || cache.SourceDigest == "" {
		t.Fatal("missing persistent cleanup record")
	}
	if _, ok := cache.track(source, 1, segments[0]); !ok {
		t.Fatal("lost confirmed identity after source removal")
	}
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadRecognitionRecord(source).track(source, 1, segments[0]); !ok {
		t.Fatal("lost confirmed identity after identical re-download")
	}
	// Equal size/timestamp must not make changed audio reuse a removed source's
	// identities. The persisted digest is the authoritative content check.
	data[len(data)-1] ^= 1
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(source, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if cache := loadRecognitionRecord(source); cache.Version != 0 {
		t.Fatal("reused identity cache for changed source audio")
	}
}

func TestExistingDownloadCleanupRequiresMatchingRecord(t *testing.T) {
	source, paths, segments, _ := cleanupFixture(t)
	if removed, err := CleanupCompletedYouTubeRecording(source); removed || err != nil {
		t.Fatal("removed source without a completed record")
	}
	info, _ := os.Stat(source)
	record := recognitionRecord{Version: 1, SourceSize: info.Size(), SourceMtime: info.ModTime().UnixNano()}
	for i, path := range paths {
		output, _ := os.Stat(path)
		relative, _ := filepath.Rel(filepath.Dir(source), path)
		record.Tracks = append(record.Tracks, identifiedTrack{Number: i + 1, Start: segments[i].Start, End: segments[i].End, Path: relative, Status: "unmatched", Size: output.Size(), Mtime: output.ModTime().UnixNano()})
	}
	data, _ := json.Marshal(record)
	if err := saveRecognitionRecord(filepath.Join(filepath.Dir(source), recognitionRecordName), data); err != nil {
		t.Fatal(err)
	}
	if removed, err := CleanupCompletedYouTubeRecording(source); !removed || err != nil {
		t.Fatalf("did not clean verified finished download: %v, %v", removed, err)
	}
}
