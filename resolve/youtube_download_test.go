package resolve

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// Exercise the actual installed downloader against local audio and metadata.
// This checks yt-dlp's filtering and full-file behavior without contacting
// YouTube or relying on a particular public video's availability.
func TestDownloadYouTubeAudioWithYTDLP(t *testing.T) {
	t.Setenv("CLIAMP_IDENTIFY", "0") // download/split fixtures never contact Shazam
	if runtime.GOOS == "windows" {
		t.Skip("Unix command shim")
	}
	realYTDLP, err := exec.LookPath("yt-dlp")
	if err != nil {
		t.Skip("yt-dlp is not installed")
	}
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("bash is not installed")
	}
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skip(name + " is not installed")
		}
	}
	// A valid short WAV, larger than a single download buffer.
	var audio bytes.Buffer
	audio.WriteString("RIFF")
	binary.Write(&audio, binary.LittleEndian, uint32(36+320000))
	audio.WriteString("WAVEfmt ")
	for _, field := range []any{uint32(16), uint16(1), uint16(1), uint32(16000), uint32(32000), uint16(2), uint16(16)} {
		binary.Write(&audio, binary.LittleEndian, field)
	}
	audio.WriteString("data")
	binary.Write(&audio, binary.LittleEndian, uint32(320000))
	audio.Write(make([]byte, 320000))
	encode := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "wav", "-i", "pipe:0", "-f", "mp3", "pipe:1")
	encode.Stdin = bytes.NewReader(audio.Bytes())
	audioData, err := encode.Output()
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write(audioData)
	}))
	defer server.Close()

	bin := t.TempDir()
	// Replace only network extraction with a local info file. All download
	// options, including the live filter, are interpreted by real yt-dlp.
	shim := "#!/bin/bash\nargs=(\"${@:1:$#-2}\")\nexec \"$CLIAMP_TEST_YTDLP\" \"${args[@]}\" --load-info-json \"$CLIAMP_TEST_INFO\"\n"
	if err := os.WriteFile(filepath.Join(bin, "yt-dlp"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CLIAMP_TEST_YTDLP", realYTDLP)
	infoPath := filepath.Join(bin, "info.json")
	t.Setenv("CLIAMP_TEST_INFO", infoPath)
	// A user's standalone yt-dlp config must not clip or redirect the file.
	t.Setenv("XDG_CONFIG_HOME", bin)
	// The canonical yt-dlp.conf path avoids colliding with our command shim.
	if err := os.WriteFile(filepath.Join(bin, "yt-dlp.conf"), []byte("--simulate\n--download-sections *1-2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		status   string
		live     bool
		duration any
		allowed  bool
		chapters []audioChapter
	}{
		{"not_live", false, 10, true, nil},
		{"was_live", false, 86400, true, nil},
		{"was_live", false, nil, true, nil},
		{"post_live", false, nil, true, nil},
		{"is_live", true, nil, false, nil},
		{"is_upcoming", false, nil, false, nil},
		{"was_live", false, 10, true, []audioChapter{{Title: "First song", Start: 0, End: 5}, {Title: "Second song", Start: 5, End: 10}}},
	} {
		t.Run(fmt.Sprintf("%s/duration=%v", tc.status, tc.duration), func(t *testing.T) {
			info := map[string]any{
				"id": "test1234567", "title": strings.Repeat("Full show 音楽 ", 20), "extractor": "youtube", "extractor_key": "Youtube",
				"webpage_url": "https://www.youtube.com/watch?v=test1234567", "live_status": tc.status, "is_live": tc.live,
				"duration": tc.duration,
				"chapters": tc.chapters,
				"formats":  []map[string]any{{"format_id": "test", "url": server.URL + "/audio.mp3", "ext": "mp3", "vcodec": "none", "acodec": "mp3", "protocol": "http"}},
			}
			if err := os.WriteFile(infoPath, mustJSON(t, info), 0o600); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(t.TempDir(), "Music")
			before := requests.Load()
			result, err := DownloadYouTubeAudio("https://www.youtube.com/watch?v=test1234567&t=120&list=ignored", dest)
			if !tc.allowed {
				if err == nil || !strings.Contains(err.Error(), "live and upcoming") {
					t.Fatalf("blocked broadcast = %+v, %v", result, err)
				}
				if requests.Load() != before {
					t.Fatal("blocked broadcast requested media")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			path := result.AudioPath
			folder := filepath.Dir(path)
			if filepath.Dir(folder) != dest || !strings.HasPrefix(filepath.Base(folder), "Full show") || len(filepath.Base(folder)) > 140 {
				t.Fatalf("incorrect stream folder: %s", folder)
			}
			got, err := os.ReadFile(path)
			if !result.FullRecordingRemoved && (err != nil || !bytes.Equal(got, audioData)) {
				t.Fatalf("full audio was not saved: %d bytes, %v", len(got), err)
			}
			if result.FullRecordingRemoved && !os.IsNotExist(err) {
				t.Fatal("redundant full recording still exists")
			}
			manifest, err := os.ReadFile(result.PlaylistPath)
			if err != nil {
				t.Fatal(err)
			}
			if len(tc.chapters) > 1 {
				if result.Tracks != 2 || result.Method != "YouTube chapters" || !strings.Contains(string(manifest), "Tracks/001 - First song.mp3") || strings.Contains(string(manifest), "Full stream.mp3") {
					t.Fatalf("incorrect chapter playlist: %+v\n%s", result, manifest)
				}
				for _, name := range []string{"001 - First song.mp3", "002 - Second song.mp3"} {
					duration, err := probeAudioDuration(filepath.Join(folder, "Tracks", name))
					if err != nil || duration < 4.8 || duration > 5.3 {
						t.Fatalf("bad split duration: %v, %v", duration, err)
					}
				}
			} else if result.Tracks != 1 || result.Method != "full recording" || !strings.Contains(string(manifest), "Full stream.mp3") {
				t.Fatalf("incorrect full recording playlist: %+v\n%s", result, manifest)
			}
			// Saving the same item again must preserve the existing file.
			if result.FullRecordingRemoved {
				// A repeat download can recreate the source for processing, then
				// clean it up again after validating the existing split tracks.
				result, err := DownloadYouTubeAudio("https://www.youtube.com/watch?v=test1234567", dest)
				if err != nil || !result.FullRecordingRemoved {
					t.Fatalf("repeat split download: %+v, %v", result, err)
				}
				return
			}
			markedAudio := append(append([]byte(nil), audioData...), []byte("existing audio marker")...)
			if err := os.WriteFile(path, markedAudio, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := DownloadYouTubeAudio("https://www.youtube.com/watch?v=test1234567", dest); err != nil {
				t.Fatal(err)
			}
			got, _ = os.ReadFile(path)
			if !bytes.Equal(got, markedAudio) {
				t.Fatal("existing file was overwritten")
			}
		})
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDownloadYouTubeAudioRejectsOtherSources(t *testing.T) {
	for _, source := range []string{"https://example.com/audio.mp3", "--exec=bad", "ytsearch:music"} {
		if _, err := DownloadYouTubeAudio(source, t.TempDir()); err == nil {
			t.Fatalf("accepted %q", source)
		}
	}
}
