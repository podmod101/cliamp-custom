package resolve

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestChapterTracksPreserveEntireRecording(t *testing.T) {
	chapters := []audioChapter{{Title: "Song one", Start: 10, End: 20}, {Title: "Song two", Start: 30, End: 40}}
	tracks := chapterTracks(chapters, 50)
	if len(tracks) != 3 || tracks[0].Start != 0 || tracks[0].End != 10 || tracks[1].End != 30 || tracks[2].End != 50 {
		t.Fatalf("chapters lose intro, gap or outro: %+v", tracks)
	}
	if chapters[0].End != 20 {
		t.Fatal("mutated source metadata")
	}
	if tracks := chapterTracks([]audioChapter{{Start: 20}, {Start: 10}}, 50); tracks != nil {
		t.Fatalf("accepted out-of-order chapters: %+v", tracks)
	}
}

func TestSilenceTracksAvoidShortTracksAndLeadingTrailingSilence(t *testing.T) {
	log := `silence_end: 5 | silence_duration: 5
silence_end: 45 | silence_duration: 3
silence_end: 123 | silence_duration: 4
silence_end: 145 | silence_duration: 4
silence_end: 243 | silence_duration: 4
silence_end: 300 | silence_duration: 4
silence_end: 360 | silence_duration: 5`
	tracks := silenceTracks(log, 360)
	if len(tracks) != 3 || tracks[0].End != 121 || tracks[1].Start != 121 || tracks[1].End != 241 || tracks[2].End != 360 {
		t.Fatalf("bad estimated boundaries: %+v", tracks)
	}
	for _, log := range []string{"", "silence_end: 360 | silence_duration: 360", "silence_end: 45 | silence_duration: 3"} {
		if tracks := silenceTracks(log, 360); tracks != nil {
			t.Fatalf("invented boundaries for %q: %+v", log, tracks)
		}
	}
}

func TestCleanTrackNameKeepsSafeShortUnicodeNames(t *testing.T) {
	name := cleanTrackName("../../Song\n" + strings.Repeat("音楽", 100))
	if len(name) > 150 || !utf8.ValidString(name) || strings.ContainsAny(name, "/\\\n") {
		t.Fatalf("unsafe or broken track name: %q", name)
	}
}

func TestSilenceFallbackWithRealAudio(t *testing.T) {
	t.Setenv("CLIAMP_IDENTIFY", "0")
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skip(name + " is not installed")
		}
	}
	dir := filepath.Join(t.TempDir(), "Stream title")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "Full stream.wav")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=8000:duration=61",
		"-f", "lavfi", "-i", "anullsrc=r=8000:cl=mono:d=3",
		"-f", "lavfi", "-i", "sine=frequency=880:sample_rate=8000:duration=61",
		"-filter_complex", "[0:a][1:a][2:a]concat=n=3:v=0:a=1[out]", "-map", "[out]", source)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generating test audio: %v: %s", err, out)
	}
	before, _ := os.ReadFile(source)
	result, err := organizeYouTubeAudio(downloadedYouTubeAudio{Filepath: source, Duration: 125})
	if err != nil {
		t.Fatal(err)
	}
	if result.Tracks != 2 || result.Method != "silence (estimated)" || result.Warning != "" {
		t.Fatalf("did not detect track gap: %+v", result)
	}
	for _, name := range []string{"001 - Track 001 (estimated).wav", "002 - Track 002 (estimated).wav"} {
		duration, err := probeAudioDuration(filepath.Join(dir, "Tracks", name))
		if err != nil || math.Abs(duration-62.5) > 0.1 {
			t.Fatalf("incorrect split: %f, %v", duration, err)
		}
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) || !result.FullRecordingRemoved {
		t.Fatal("kept redundant full recording after complete splits")
	}
	manifest, err := os.ReadFile(result.PlaylistPath)
	if err != nil || !strings.Contains(string(manifest), "silence (estimated)") || strings.Contains(string(manifest), "Full stream.wav") {
		t.Fatalf("incorrect detected playlist: %s, %v", manifest, err)
	}
	// Existing split tracks and playlist must survive another save unchanged.
	track := filepath.Join(dir, "Tracks", "001 - Track 001 (estimated).wav")
	if err := os.WriteFile(track, []byte("existing track"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A later download supplies the source again. A damaged existing output
	// must prevent cleanup while remaining protected from overwrite.
	if err := os.WriteFile(source, before, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := organizeYouTubeAudio(downloadedYouTubeAudio{Filepath: source, Duration: 125}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(track)
	if string(got) != "existing track" {
		t.Fatal("overwrote existing track")
	}
	if err := checkAudioFile(source); err != nil {
		t.Fatal("removed recovery source despite damaged track")
	}
}

func TestSplitFailureStillSavesFullRecordingPlaylist(t *testing.T) {
	t.Setenv("CLIAMP_IDENTIFY", "0")
	dir := t.TempDir()
	source := filepath.Join(dir, "Full stream.mp3")
	if err := os.WriteFile(source, []byte("audio fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	result, err := organizeYouTubeAudio(downloadedYouTubeAudio{Filepath: source, Duration: 10, Chapters: []audioChapter{{Start: 0}, {Start: 5}}})
	if err != nil || result.Warning == "" || result.Tracks != 1 || result.Method != "full recording" {
		t.Fatalf("failed to preserve download after split failure: %+v, %v", result, err)
	}
	if err := checkAudioFile(result.PlaylistPath); err != nil {
		t.Fatal(err)
	}
	temps, _ := filepath.Glob(filepath.Join(dir, "Tracks", ".cliamp-track-*"))
	if len(temps) != 0 {
		t.Fatalf("left partial tracks: %v", temps)
	}
}
