package resolve

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bjarneo/cliamp/internal/ytdlcookies"
	"github.com/bjarneo/cliamp/playlist"
)

// YouTubeAudioDownload describes the complete recording and its local playlist.
type YouTubeAudioDownload struct {
	AudioPath            string
	PlaylistPath         string
	Tracks               int
	Method               string
	Warning              string
	Identified           int
	Identification       bool
	FullRecordingRemoved bool
}

type downloadedYouTubeAudio struct {
	Filepath string         `json:"filepath"`
	Duration float64        `json:"duration"`
	Chapters []audioChapter `json:"chapters"`
}

// DownloadYouTubeAudio saves complete non-live audio in a title-named folder
// under saveDir, then creates tracks and a playlist. Redundant full audio is
// removed only after complete, playable replacements and playlists are saved.
func DownloadYouTubeAudio(pageURL, saveDir string) (YouTubeAudioDownload, error) {
	var result YouTubeAudioDownload
	if !playlist.IsYouTubeURL(pageURL) && !playlist.IsYouTubeMusicURL(pageURL) {
		return result, fmt.Errorf("a YouTube video URL is required")
	}
	for _, binary := range []string{"yt-dlp", "ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(binary); err != nil {
			return result, fmt.Errorf("%s not found in PATH", binary)
		}
	}
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		return result, fmt.Errorf("creating Music directory: %w", err)
	}

	// Ignore standalone yt-dlp config so ranges, filters and output overrides
	// cannot turn this whole-item download into a clip or a live recording.
	args := []string{
		"--ignore-config",
		"--no-playlist",
		"--match-filters", "!is_live & live_status!=is_upcoming",
		"-f", "bestaudio/best",
		"--extract-audio", "--audio-format", "best",
		"--no-simulate",
		"--no-overwrites",
		"--no-post-overwrites",
		"--abort-on-unavailable-fragments",
		"--socket-timeout", "15",
		"--print", "after_move:%(.{filepath,duration,chapters})j",
		"-o", filepath.Join(saveDir, "%(title).120B [%(id)s]", "Full stream.%(ext)s"),
	}
	if browser := ytdlcookies.ForURL(pageURL); browser != "" {
		args = append(args, "--cookies-from-browser", browser)
	}
	args = append(args, "--", pageURL)
	cmd := exec.Command("yt-dlp", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return result, fmt.Errorf("yt-dlp: %s", msg)
		}
		return result, fmt.Errorf("yt-dlp: %w", err)
	}
	// A rejected match filter exits successfully, but produces no saved path.
	if strings.TrimSpace(string(stdout)) == "" {
		return result, fmt.Errorf("nothing downloaded: live and upcoming YouTube broadcasts cannot be saved")
	}
	var audio downloadedYouTubeAudio
	if err := json.Unmarshal(stdout, &audio); err != nil {
		return result, fmt.Errorf("parsing saved audio metadata: %w", err)
	}
	if err := checkAudioFile(audio.Filepath); err != nil {
		return result, fmt.Errorf("checking saved audio: %w", err)
	}
	return organizeYouTubeAudio(audio)
}
