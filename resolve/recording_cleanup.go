package resolve

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func recordingDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// localPlaylistPaths handles the relative, absolute and file-URI entries used
// in M3U/PLS files. Errors keep the recording rather than risk breaking a list.
func localPlaylistPaths(folder string) ([][]string, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	var playlists [][]string
	for _, entry := range entries {
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if entry.IsDir() || (ext != ".m3u" && ext != ".m3u8" && ext != ".pls") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(folder, entry.Name()))
		if err != nil {
			return nil, err
		}
		var paths []string
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "\uFEFF"))
			if ext == ".pls" {
				key, value, ok := strings.Cut(line, "=")
				if !ok || !strings.HasPrefix(strings.ToLower(key), "file") {
					continue
				}
				line = strings.TrimSpace(value)
			}
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if strings.HasPrefix(line, "file:") {
				parsed, err := url.Parse(line)
				if err != nil || (parsed.Host != "" && parsed.Host != "localhost") {
					return nil, fmt.Errorf("unreadable local playlist URI")
				}
				line = parsed.Path
			} else if strings.Contains(line, "://") {
				continue
			}
			path := filepath.FromSlash(line)
			if !filepath.IsAbs(path) {
				path = filepath.Join(folder, path)
			}
			paths = append(paths, filepath.Clean(path))
		}
		playlists = append(playlists, paths)
	}
	return playlists, nil
}

// removeCompletedRecording runs only after splitting/naming and playlist
// publication. Unknown or incomplete coverage leaves the full audio available.
func removeCompletedRecording(source string, paths []string, segments []audioChapter) (bool, error) {
	if len(paths) == 0 || len(paths) != len(segments) || !strings.HasPrefix(filepath.Base(source), "Full stream.") {
		return false, nil
	}
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return false, nil
	}
	duration, err := probeAudioDuration(source)
	if err != nil || math.Abs(segments[0].Start) > 0.05 || math.Abs(segments[len(segments)-1].End-duration) > 0.5 {
		return false, nil
	}
	folder := filepath.Dir(source)
	var outputs []os.FileInfo
	seen := make(map[string]bool)
	for i, path := range paths {
		relative, err := filepath.Rel(folder, path)
		if err != nil || !filepath.IsLocal(relative) || filepath.Dir(relative) != "Tracks" || seen[path] {
			return false, nil
		}
		seen[path] = true
		segment := segments[i]
		if segment.End <= segment.Start || (i > 0 && math.Abs(segments[i-1].End-segment.Start) > 0.05) {
			return false, nil
		}
		output, err := os.Lstat(path)
		if err != nil || !output.Mode().IsRegular() || output.Size() == 0 || os.SameFile(info, output) {
			return false, nil
		}
		length, err := probeAudioDuration(path)
		if err != nil || math.Abs(length-(segment.End-segment.Start)) > 0.5 {
			return false, nil
		}
		outputs = append(outputs, output)
	}
	playlists, err := localPlaylistPaths(folder)
	if err != nil {
		return false, fmt.Errorf("checking playlists before full recording cleanup: %w", err)
	}
	completePlaylist := false
	for _, playlist := range playlists {
		included := make(map[string]bool)
		for _, path := range playlist {
			if path == filepath.Clean(source) {
				return false, nil
			}
			included[path] = true
		}
		complete := true
		for _, path := range paths {
			complete = complete && included[filepath.Clean(path)]
		}
		completePlaylist = completePlaylist || complete
	}
	if !completePlaylist {
		return false, nil
	}
	// Preserve identities across an intentional deletion or identical re-download.
	// The digest prevents cache reuse when an upload's audio has actually changed.
	digest, err := recordingDigest(source)
	if err != nil {
		return false, err
	}
	record := loadRecognitionRecord(source)
	previous := record.Tracks
	record.Version, record.SourceSize, record.SourceMtime = 1, info.Size(), info.ModTime().UnixNano()
	record.SourceDigest, record.SourceRemoved = digest, true
	record.Tracks = nil
	for i, path := range paths {
		relative, _ := filepath.Rel(folder, path)
		track := identifiedTrack{Number: i + 1, Start: segments[i].Start, End: segments[i].End, Path: relative, Status: "unmatched", Size: outputs[i].Size(), Mtime: outputs[i].ModTime().UnixNano()}
		for _, old := range previous {
			if old.Number == track.Number && old.Path == track.Path && old.Size == track.Size && old.Mtime == track.Mtime && math.Abs(old.Start-track.Start) < 0.05 && math.Abs(old.End-track.End) < 0.05 {
				track.Song, track.Status = old.Song, old.Status
				break
			}
		}
		record.Tracks = append(record.Tracks, track)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return false, err
	}
	if err := saveRecognitionRecord(filepath.Join(folder, recognitionRecordName), append(data, '\n')); err != nil {
		return false, fmt.Errorf("saving completed track record: %w", err)
	}
	// Re-check after probing/hashing so a changing recording or replaced track
	// cannot be removed based on a stale validation result.
	current, err := os.Lstat(source)
	if err != nil || !os.SameFile(info, current) || current.Size() != info.Size() || !current.ModTime().Equal(info.ModTime()) {
		return false, nil
	}
	for i, path := range paths {
		current, err := os.Lstat(path)
		if err != nil || !os.SameFile(outputs[i], current) || current.Size() != outputs[i].Size() || !current.ModTime().Equal(outputs[i].ModTime()) {
			return false, nil
		}
	}
	if err := os.Remove(source); err != nil {
		return false, fmt.Errorf("removing redundant full recording: %w", err)
	}
	return true, nil
}

// CleanupCompletedYouTubeRecording applies the same validation to a previously
// finished download. A missing/stale record leaves its recording untouched.
func CleanupCompletedYouTubeRecording(source string) (bool, error) {
	record := loadRecognitionRecord(source)
	if record.Version != 1 || len(record.Tracks) == 0 {
		return false, nil
	}
	var paths []string
	var segments []audioChapter
	for i, track := range record.Tracks {
		if track.Number != i+1 || !filepath.IsLocal(track.Path) {
			return false, nil
		}
		path := filepath.Join(filepath.Dir(source), track.Path)
		info, err := os.Stat(path)
		if err != nil || info.Size() != track.Size || info.ModTime().UnixNano() != track.Mtime {
			return false, nil
		}
		paths = append(paths, path)
		segments = append(segments, audioChapter{Start: track.Start, End: track.End})
	}
	return removeCompletedRecording(source, paths, segments)
}
