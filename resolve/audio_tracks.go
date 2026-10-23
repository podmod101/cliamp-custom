package resolve

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type audioChapter struct {
	Title string  `json:"title"`
	Start float64 `json:"start_time"`
	End   float64 `json:"end_time"`
}

const minimumDetectedTrack = 60.0

var silenceEndPattern = regexp.MustCompile(`silence_end:\s*([0-9.eE+\-]+)\s*\|\s*silence_duration:\s*([0-9.eE+\-]+)`)

func organizeYouTubeAudio(audio downloadedYouTubeAudio) (YouTubeAudioDownload, error) {
	result := YouTubeAudioDownload{AudioPath: audio.Filepath, Method: "full recording"}
	cache := loadRecognitionRecord(audio.Filepath)
	if duration, err := probeAudioDuration(audio.Filepath); err == nil {
		// Prefer the actual saved audio length so approximate site metadata
		// cannot truncate the final track. A failed probe still keeps the file.
		audio.Duration = duration
	}
	segments := chapterTracks(audio.Chapters, audio.Duration)
	if len(segments) > 1 {
		result.Method = "YouTube chapters"
	} else if audio.Duration >= 2*minimumDetectedTrack {
		var err error
		segments, err = detectSilenceTracks(audio.Filepath, audio.Duration)
		if err != nil {
			result.Warning = "Silence detection failed; full recording kept"
		} else if len(segments) > 1 {
			result.Method = "silence (estimated)"
		}
	}

	var paths []string
	splitFailed := false
	if len(segments) > 1 {
		for i, segment := range segments {
			if track, ok := cache.track(audio.Filepath, i+1, segment); ok {
				paths = append(paths, filepath.Join(filepath.Dir(audio.Filepath), track.Path))
				continue
			}
			path, err := splitAudioTrack(audio.Filepath, i+1, segment)
			if err != nil {
				// A failed split must not hide or discard a successful download.
				result.Warning = "Track splitting failed; full recording kept"
				paths = nil
				splitFailed = true
				break
			}
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		paths = []string{audio.Filepath}
		segments = []audioChapter{{Title: "Full stream", End: audio.Duration}}
		result.Method = "full recording"
	}
	result.Identification = os.Getenv("CLIAMP_IDENTIFY") != "0"
	identified, warning, originals := identifyDownloadedTracks(audio.Filepath, paths, segments, cache)
	result.Identified = identified
	if warning != "" {
		if result.Warning != "" {
			result.Warning += "; "
		}
		result.Warning += warning
	}
	result.Tracks = len(paths)
	folder := filepath.Dir(audio.Filepath)
	result.PlaylistPath = filepath.Join(folder, filepath.Base(folder)+".m3u8")
	var playlist strings.Builder
	fmt.Fprintf(&playlist, "#EXTM3U\n# Split method: %s\n", result.Method)
	for i, path := range paths {
		relative, err := filepath.Rel(folder, path)
		if err != nil {
			return result, err
		}
		duration := -1
		if segments[i].End > segments[i].Start {
			duration = int(math.Round(segments[i].End - segments[i].Start))
		}
		fmt.Fprintf(&playlist, "#EXTINF:%d,%s\n%s\n", duration, cleanTrackName(segments[i].Title), filepath.ToSlash(relative))
	}
	savedPlaylist, err := writeNewPlaylist(result.PlaylistPath, playlist.String())
	if err != nil {
		return result, fmt.Errorf("full audio saved to %s, but playlist could not be saved: %w", audio.Filepath, err)
	}
	result.PlaylistPath = savedPlaylist
	removeUnusedOriginals(folder, originals)
	if !splitFailed {
		removed, err := removeCompletedRecording(audio.Filepath, paths, segments)
		result.FullRecordingRemoved = removed
		if err != nil {
			if result.Warning != "" {
				result.Warning += "; "
			}
			result.Warning += "Full recording kept: " + err.Error()
		}
	}
	return result, nil
}

// Chapter starts define contiguous tracks, preserving intros, gaps and outros.
func chapterTracks(chapters []audioChapter, duration float64) []audioChapter {
	if len(chapters) < 2 {
		return nil
	}
	tracks := append([]audioChapter(nil), chapters...)
	for i := range tracks {
		if tracks[i].Start < 0 || (duration > 0 && tracks[i].Start >= duration) || (i > 0 && tracks[i].Start <= tracks[i-1].Start) {
			return nil
		}
		if tracks[i].Title == "" {
			tracks[i].Title = fmt.Sprintf("Track %03d", i+1)
		}
		if i+1 < len(tracks) {
			tracks[i].End = tracks[i+1].Start
		} else {
			tracks[i].End = duration // zero means continue through EOF
		}
	}
	if tracks[0].Start > 0 {
		tracks = append([]audioChapter{{Title: "Intro", End: tracks[0].Start}}, tracks...)
	}
	return tracks
}

func probeAudioDuration(path string) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		return 0, err
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return 0, fmt.Errorf("audio duration is unavailable")
	}
	return duration, nil
}

func detectSilenceTracks(path string, duration float64) ([]audioChapter, error) {
	cmd := exec.Command("ffmpeg", "-hide_banner", "-nostdin", "-nostats", "-i", path,
		"-vn", "-sn", "-af", "silencedetect=noise=-45dB:duration=2", "-f", "null", "-")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return silenceTracks(stderr.String(), duration), nil
}

func silenceTracks(log string, duration float64) []audioChapter {
	boundaries := []float64{0}
	for _, match := range silenceEndPattern.FindAllStringSubmatch(log, -1) {
		end, _ := strconv.ParseFloat(match[1], 64)
		length, _ := strconv.ParseFloat(match[2], 64)
		start := end - length
		cut := (start + end) / 2
		// Ignore leading/trailing silence and avoid short fragments. Cuts are
		// estimates, so no part of the original recording is removed.
		if length >= 2 && start > 0.5 && end < duration-0.5 && cut-boundaries[len(boundaries)-1] >= minimumDetectedTrack && duration-cut >= minimumDetectedTrack {
			boundaries = append(boundaries, cut)
		}
	}
	if len(boundaries) == 1 {
		return nil
	}
	boundaries = append(boundaries, duration)
	var tracks []audioChapter
	for i := 0; i+1 < len(boundaries); i++ {
		tracks = append(tracks, audioChapter{
			Title: fmt.Sprintf("Track %03d (estimated)", i+1), Start: boundaries[i], End: boundaries[i+1],
		})
	}
	return tracks
}

func cleanTrackName(title string) string {
	title = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, title))
	for len(title) > 150 {
		_, size := utf8.DecodeLastRuneInString(title)
		title = title[:len(title)-size]
	}
	if title == "" || title == "." || title == ".." {
		return "Track"
	}
	return title
}

func splitAudioTrack(source string, number int, segment audioChapter) (string, error) {
	dir := filepath.Join(filepath.Dir(source), "Tracks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ext := filepath.Ext(source)
	dest := filepath.Join(dir, fmt.Sprintf("%03d - %s%s", number, cleanTrackName(segment.Title), ext))
	if err := checkAudioFile(dest); err == nil {
		return dest, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	temp, err := os.CreateTemp(dir, ".cliamp-track-*"+ext)
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		os.Remove(tempPath)
		return "", err
	}
	defer os.Remove(tempPath)
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-ss", strconv.FormatFloat(segment.Start, 'f', 6, 64), "-i", source}
	if segment.End > segment.Start {
		args = append(args, "-t", strconv.FormatFloat(segment.End-segment.Start, 'f', 6, 64))
	}
	args = append(args, "-map", "0:a:0", "-vn", "-c:a", "copy", tempPath)
	if output, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("splitting track: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := checkAudioFile(tempPath); err != nil {
		return "", err
	}
	// Publishing a finished temporary file avoids leaving partial tracks or
	// overwriting an existing file, including when two saves run concurrently.
	if err := os.Link(tempPath, dest); err != nil {
		if !os.IsExist(err) {
			return "", err
		}
		if err := checkAudioFile(dest); err != nil {
			return "", err
		}
	}
	return dest, nil
}

func checkAudioFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("%s is not a nonempty audio file", path)
	}
	return nil
}

func writeNewPlaylist(path, contents string) (string, error) {
	temp, err := os.CreateTemp(filepath.Dir(path), ".cliamp-playlist-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.WriteString(contents); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	base := strings.TrimSuffix(path, filepath.Ext(path))
	for suffix := 0; suffix < 1000; suffix++ {
		dest := path
		if suffix > 0 {
			dest = base + " - Identified"
			if suffix > 1 {
				dest += fmt.Sprintf(" (%d)", suffix)
			}
			dest += ".m3u8"
		}
		if err := os.Link(temp.Name(), dest); err == nil {
			return dest, nil
		} else if !os.IsExist(err) {
			return "", err
		}
		if data, err := os.ReadFile(dest); err == nil && string(data) == contents {
			return dest, nil
		}
	}
	return "", fmt.Errorf("too many existing playlists with this name")
}
