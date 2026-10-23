package resolve

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseSongRecognition(t *testing.T) {
	for _, tc := range []struct {
		name, response  string
		matched, failed bool
	}{
		{"match", `{"matches":[{"id":"123"}],"track":{"key":"123","title":"Song","subtitle":"Artist"}}`, true, false},
		{"no match", `{"matches":[]}`, false, false},
		{"incomplete metadata", `{"matches":[{}],"track":{"key":"123","title":"Song"}}`, false, false},
		{"service error", `{"error":"rate limited"}`, false, true},
		{"invalid JSON", `bad response`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			song, err := parseSongRecognition([]byte(tc.response))
			if (song.Key != "") != tc.matched || (err != nil) != tc.failed {
				t.Fatalf("got %+v, %v", song, err)
			}
		})
	}
}

func TestIdentifySongRequiresAgreementAndRejectsMixedTracks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		keys   []string
		status string
		calls  int
	}{
		{"agreement", []string{"A", "A"}, "identified", 2},
		{"one missing", []string{"A", "", "A"}, "identified", 3},
		{"one vote", []string{"A", "", ""}, "unmatched", 3},
		{"no matches", []string{"", ""}, "unmatched", 2},
		{"mixed songs", []string{"A", "B"}, "conflicting matches", 2},
		{"third conflicts", []string{"A", "", "B"}, "conflicting matches", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			var offsets []float64
			song, status, err := identifySong("track.mp3", 120, func(_ string, offset float64) (songIdentity, error) {
				offsets = append(offsets, offset)
				key := tc.keys[calls]
				calls++
				return songIdentity{Key: key, Artist: "Artist", Title: "Song"}, nil
			})
			if err != nil || status != tc.status || calls != tc.calls || ((song.Key != "") != (status == "identified")) {
				t.Fatalf("got %+v, %s, %v; calls=%d", song, status, err, calls)
			}
			if len(offsets) >= 2 && offsets[1]-offsets[0] < 12 {
				t.Fatalf("samples overlapped: %v", offsets)
			}
		})
	}
	calls := 0
	_, status, err := identifySong("track", 120, func(string, float64) (songIdentity, error) {
		calls++
		return songIdentity{}, errors.New("offline")
	})
	if status != "unavailable" || err == nil || calls != 1 {
		t.Fatal("lookup failure did not fail fast")
	}
}

func recognitionFixture(t *testing.T, seconds int) string {
	t.Helper()
	for _, binary := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skip(binary + " not installed")
		}
	}
	dir := filepath.Join(t.TempDir(), "Stream title [test123]")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "Full stream.mp3")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration="+string(mustJSON(t, seconds)), source)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generating recognition fixture: %v: %s", err, out)
	}
	return source
}

func fakeSongrec(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell fixture")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "songrec")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLIAMP_SONGREC", path)
	t.Setenv("CLIAMP_IDENTIFY", "1")
	return path
}

func TestDownloadOrganizationIdentifiesTagsAndCachesSplitTracks(t *testing.T) {
	source := recognitionFixture(t, 35)
	before, _ := os.ReadFile(source)
	requests := filepath.Join(t.TempDir(), "requests.txt")
	t.Setenv("CLIAMP_TEST_RECOGNITION_REQUESTS", requests)
	fakeSongrec(t, `test "$1" = audio-file-to-recognized-song || exit 2
test -s "$2" || exit 3
printf '%s\n' "$2" >> "$CLIAMP_TEST_RECOGNITION_REQUESTS"
printf '%s\n' '{"matches":[{"id":"123"}],"track":{"key":"123","title":"Song / title","subtitle":"Artist"}}'
`)
	audio := downloadedYouTubeAudio{Filepath: source, Duration: 35, Chapters: []audioChapter{{Title: "Unknown first track", Start: 0}, {Title: "Outro", Start: 30}}}
	result, err := organizeYouTubeAudio(audio)
	if err != nil || result.Tracks != 2 || result.Identified != 1 || result.Warning != "" {
		t.Fatalf("identification pipeline: %+v, %v", result, err)
	}
	named := filepath.Join(filepath.Dir(source), "Tracks", "001 - Artist - Song _ title.mp3")
	if err := checkAudioFile(named); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(source), "Tracks", "001 - Unknown first track.mp3")); !os.IsNotExist(err) {
		t.Fatal("left duplicate generic track after successful rename")
	}
	manifest, _ := os.ReadFile(result.PlaylistPath)
	if !strings.Contains(string(manifest), "Artist - Song _ title") || !strings.Contains(string(manifest), "Tracks/002 - Outro.mp3") {
		t.Fatalf("playlist did not follow organized tracks: %s", manifest)
	}
	tags, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format_tags=artist,title,track,album", "-of", "json", named).Output()
	if err != nil || !bytes.Contains(tags, []byte(`"artist": "Artist"`)) || !bytes.Contains(tags, []byte(`"title": "Song / title"`)) {
		t.Fatalf("missing embedded tags: %s, %v", tags, err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) || !result.FullRecordingRemoved {
		t.Fatal("redundant source was not cleaned up")
	}
	// Simulate the source being fetched again; its digest must preserve the
	// confirmed identity cache even though its modification time changed.
	if err := os.WriteFile(source, before, 0o600); err != nil {
		t.Fatal(err)
	}
	result2, err := organizeYouTubeAudio(audio)
	if err != nil || result2.Identified != 1 || result2.PlaylistPath != result.PlaylistPath {
		t.Fatalf("repeated download: %+v, %v", result2, err)
	}
	lookups, _ := os.ReadFile(requests)
	if len(strings.Fields(string(lookups))) != 2 {
		t.Fatalf("repeated cached lookups: %s", lookups)
	}
	for _, sample := range strings.Fields(string(lookups)) {
		if _, err := os.Stat(sample); !os.IsNotExist(err) {
			t.Fatalf("left temporary audio sample: %s", sample)
		}
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) || !result2.FullRecordingRemoved {
		t.Fatal("kept redundant source after repeat processing")
	}
}

func TestIdentificationUnavailableKeepsDownloadAndStopsRequests(t *testing.T) {
	source := recognitionFixture(t, 60)
	requests := filepath.Join(t.TempDir(), "requests.txt")
	t.Setenv("CLIAMP_TEST_RECOGNITION_REQUESTS", requests)
	fakeSongrec(t, "printf 'request\\n' >> \"$CLIAMP_TEST_RECOGNITION_REQUESTS\"\nexit 1\n")
	result, err := organizeYouTubeAudio(downloadedYouTubeAudio{Filepath: source, Duration: 60, Chapters: []audioChapter{{Start: 0}, {Start: 30}}})
	if err != nil || result.Tracks != 2 || result.Identified != 0 || result.Warning == "" {
		t.Fatalf("offline fallback: %+v, %v", result, err)
	}
	lookups, _ := os.ReadFile(requests)
	if string(lookups) != "request\n" {
		t.Fatalf("did not stop unavailable batch: %s", lookups)
	}
	if err := checkAudioFile(result.PlaylistPath); err != nil {
		t.Fatal(err)
	}
}

func TestIdentifiedNamesAndPlaylistsPreserveExistingFiles(t *testing.T) {
	source := recognitionFixture(t, 30)
	dir := filepath.Join(filepath.Dir(source), "Tracks")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(dir, "001 - Artist - Song.mp3")
	if err := os.WriteFile(existing, []byte("existing user's file"), 0o600); err != nil {
		t.Fatal(err)
	}
	named, err := nameIdentifiedTrack(source, source, 1, songIdentity{Key: "123", Artist: "Artist", Title: "Song"})
	if err != nil || !strings.HasSuffix(named, " (2).mp3") {
		t.Fatalf("name collision not handled: %s, %v", named, err)
	}
	data, _ := os.ReadFile(existing)
	if string(data) != "existing user's file" {
		t.Fatal("overwrote existing audio")
	}
	playlist := filepath.Join(filepath.Dir(source), "Stream.m3u8")
	if err := os.WriteFile(playlist, []byte("original playlist"), 0o600); err != nil {
		t.Fatal(err)
	}
	saved, err := writeNewPlaylist(playlist, "identified playlist")
	if err != nil || saved == playlist {
		t.Fatalf("existing playlist not preserved: %s, %v", saved, err)
	}
	data, _ = os.ReadFile(playlist)
	if string(data) != "original playlist" {
		t.Fatal("overwrote existing playlist")
	}
}

func TestOriginalCleanupPreservesReferencedTracks(t *testing.T) {
	for _, reference := range []string{"Tracks/001.mp3", "./Tracks/001.mp3", "absolute"} {
		dir := filepath.Join(t.TempDir(), "Stream title [video123]")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		tracks := filepath.Join(dir, "Tracks")
		if err := os.Mkdir(tracks, 0o755); err != nil {
			t.Fatal(err)
		}
		original := filepath.Join(tracks, "001.mp3")
		if err := os.WriteFile(original, []byte("audio"), 0o600); err != nil {
			t.Fatal(err)
		}
		if reference == "absolute" {
			reference = original
		}
		if err := os.WriteFile(filepath.Join(dir, "existing.m3u8"), []byte("#EXTM3U\n"+reference+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		removeUnusedOriginals(dir, []string{original})
		if err := checkAudioFile(original); err != nil {
			t.Fatalf("broke existing playlist %q: %v", reference, err)
		}
	}
}

func TestCachedUnsplitIdentityWorksWhenOnlineRecognitionDisabled(t *testing.T) {
	source := recognitionFixture(t, 30)
	dir := filepath.Join(filepath.Dir(source), "Tracks")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	named := filepath.Join(dir, "001 - Artist - Song.mp3")
	if err := os.WriteFile(named, []byte("already tagged audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(named)
	cache := recognitionRecord{Tracks: []identifiedTrack{{Number: 1, End: 30, Path: filepath.Join("Tracks", filepath.Base(named)), Status: "identified", Song: songIdentity{Key: "123", Artist: "Artist", Title: "Song"}, Size: info.Size(), Mtime: info.ModTime().UnixNano()}}}
	paths := []string{source}
	segments := []audioChapter{{Title: "Full stream", End: 30}}
	t.Setenv("CLIAMP_IDENTIFY", "0")
	identified, warning, _ := identifyDownloadedTracks(source, paths, segments, cache)
	if identified != 1 || warning != "" || paths[0] != named || segments[0].Title != "Artist - Song" {
		t.Fatalf("disabled recognition lost confirmed name: %d, %s, %v, %+v", identified, warning, paths, segments)
	}
}

// Opt-in smoke check for the real installed recognizer and an explicitly chosen
// public sample. Normal test runs never contact Shazam.
func TestSongIdentificationOnlineSmoke(t *testing.T) {
	preview := os.Getenv("CLIAMP_IDENTIFICATION_SMOKE_SOURCE")
	if preview == "" {
		t.Skip("set CLIAMP_IDENTIFICATION_SMOKE_SOURCE to test online recognition")
	}
	t.Setenv("CLIAMP_IDENTIFY", "1")
	dir := filepath.Join(t.TempDir(), "Public preview")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "Full stream.m4a")
	data, err := os.ReadFile(preview)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := organizeYouTubeAudio(downloadedYouTubeAudio{Filepath: source})
	if err != nil || result.Identified != 1 || result.Warning != "" {
		t.Fatalf("online identification: %+v, %v", result, err)
	}
	record := loadRecognitionRecord(source)
	encoded, _ := json.Marshal(record.Tracks[0].Song)
	t.Logf("Verified real recognition, artist/title tags and playlist: %s", encoded)
}
