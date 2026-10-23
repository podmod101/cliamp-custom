package resolve

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const recognitionRecordName = ".cliamp-identification.json"

type songIdentity struct {
	Key    string `json:"key"`
	Artist string `json:"artist"`
	Title  string `json:"title"`
}

type identifiedTrack struct {
	Number int          `json:"number"`
	Start  float64      `json:"start"`
	End    float64      `json:"end"`
	Path   string       `json:"path"`
	Status string       `json:"status"`
	Song   songIdentity `json:"song,omitempty"`
	Size   int64        `json:"size"`
	Mtime  int64        `json:"mtime"`
}

type recognitionRecord struct {
	Version       int               `json:"version"`
	SourceSize    int64             `json:"source_size"`
	SourceMtime   int64             `json:"source_mtime"`
	SourceDigest  string            `json:"source_sha256,omitempty"`
	SourceRemoved bool              `json:"source_removed,omitempty"`
	Tracks        []identifiedTrack `json:"tracks"`
}

func loadRecognitionRecord(source string) recognitionRecord {
	var record recognitionRecord
	data, err := os.ReadFile(filepath.Join(filepath.Dir(source), recognitionRecordName))
	if err != nil || json.Unmarshal(data, &record) != nil || record.Version != 1 {
		return recognitionRecord{}
	}
	info, err := os.Stat(source)
	if !record.SourceRemoved && err == nil && info.Size() == record.SourceSize && info.ModTime().UnixNano() == record.SourceMtime {
		return record
	}
	if record.SourceRemoved && record.SourceDigest != "" {
		if os.IsNotExist(err) {
			return record
		}
		if err == nil {
			if digest, err := recordingDigest(source); err == nil && digest == record.SourceDigest {
				return record
			}
		}
	}
	return recognitionRecord{}
}

// A cached result is reused only for the same source, boundaries, and unchanged
// output file. In particular, another save must not recreate the old generic name.
func (r recognitionRecord) track(source string, number int, segment audioChapter) (identifiedTrack, bool) {
	for _, track := range r.Tracks {
		if track.Status != "identified" || track.Song.Key == "" || track.Song.Artist == "" || track.Song.Title == "" || track.Number != number || math.Abs(track.Start-segment.Start) > 0.1 || math.Abs(track.End-segment.End) > 0.1 || !filepath.IsLocal(track.Path) || filepath.Dir(track.Path) != "Tracks" {
			continue
		}
		path := filepath.Join(filepath.Dir(source), track.Path)
		info, err := os.Stat(path)
		if err == nil && info.Mode().IsRegular() && info.Size() == track.Size && info.ModTime().UnixNano() == track.Mtime {
			return track, true
		}
	}
	return identifiedTrack{}, false
}

func songrecBinary() (string, error) {
	if override := os.Getenv("CLIAMP_SONGREC"); override != "" {
		return exec.LookPath(override)
	}
	if home, err := os.UserHomeDir(); err == nil {
		if path, err := exec.LookPath(filepath.Join(home, ".local", "libexec", "cliamp", "songrec")); err == nil {
			return path, nil
		}
	}
	return exec.LookPath("songrec")
}

type songRecognizer struct {
	binary      string
	lastRequest time.Time
}

func (r *songRecognizer) sample(path string, offset float64) (songIdentity, error) {
	var song songIdentity
	temp, err := os.CreateTemp("", "cliamp-recognition-*.wav")
	if err != nil {
		return song, err
	}
	name := temp.Name()
	if err := temp.Close(); err != nil {
		os.Remove(name)
		return song, err
	}
	defer os.Remove(name)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	// Only a short mono sample is decoded. SongRec sends its fingerprint, not
	// this WAV or the original recording, to Shazam for the database lookup.
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-ss", fmt.Sprintf("%.6f", offset), "-i", path, "-t", "12", "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", name)
	if err := cmd.Run(); err != nil {
		return song, fmt.Errorf("preparing recognition sample: %w", err)
	}
	// Match SongRec's default request spacing, avoiding bursts across tracks.
	if delay := 10*time.Second - time.Since(r.lastRequest); delay > 0 {
		time.Sleep(delay)
	}
	r.lastRequest = time.Now()
	requestCtx, requestCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer requestCancel()
	request := exec.CommandContext(requestCtx, r.binary, "audio-file-to-recognized-song", name)
	request.WaitDelay = 2 * time.Second
	output, err := request.Output()
	if err != nil {
		return song, fmt.Errorf("song lookup unavailable: %w", err)
	}
	return parseSongRecognition(output)
}

func parseSongRecognition(data []byte) (songIdentity, error) {
	var response struct {
		Matches []json.RawMessage `json:"matches"`
		Track   *struct {
			Key      string `json:"key"`
			Title    string `json:"title"`
			Subtitle string `json:"subtitle"`
		} `json:"track"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return songIdentity{}, fmt.Errorf("invalid recognition response: %w", err)
	}
	if response.Matches == nil {
		return songIdentity{}, fmt.Errorf("recognition service returned an unexpected response")
	}
	if len(response.Matches) == 0 || response.Track == nil {
		return songIdentity{}, nil
	}
	song := songIdentity{Key: strings.TrimSpace(response.Track.Key), Artist: strings.TrimSpace(response.Track.Subtitle), Title: strings.TrimSpace(response.Track.Title)}
	if song.Key == "" || song.Artist == "" || song.Title == "" {
		return songIdentity{}, nil
	}
	return song, nil
}

// Require two matching independent samples and reject any conflicting song ID.
// If only one of the first two matches, a third sample can confirm it.
func identifySong(path string, duration float64, sample func(string, float64) (songIdentity, error)) (songIdentity, string, error) {
	var found songIdentity
	if duration < 24 {
		return found, "too short", nil
	}
	votes := 0
	for i, fraction := range []float64{0.25, 0.75, 0.5} {
		if i == 2 && (votes == 0 || votes == 2) {
			break
		}
		offset := math.Max(0, math.Min(duration-12, duration*fraction-6))
		song, err := sample(path, offset)
		if err != nil {
			return songIdentity{}, "unavailable", err
		}
		if song.Key == "" {
			continue
		}
		if votes > 0 && song.Key != found.Key {
			return songIdentity{}, "conflicting matches", nil
		}
		found = song
		votes++
	}
	if votes < 2 {
		return songIdentity{}, "unmatched", nil
	}
	return found, "identified", nil
}

// nameIdentifiedTrack remuxes tags without re-encoding, then publishes a new
// numbered name without overwriting any existing file. The caller retains the
// old file until the playlist is safely written. Full-audio cleanup happens only
// after all finished tracks and playlists have been validated.
func nameIdentifiedTrack(source, path string, number int, song songIdentity) (string, error) {
	dir := filepath.Join(filepath.Dir(source), "Tracks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ext := filepath.Ext(path)
	temp, err := os.CreateTemp(dir, ".cliamp-tagged-*"+ext)
	if err != nil {
		return "", err
	}
	tempName := temp.Name()
	if err := temp.Close(); err != nil {
		os.Remove(tempName)
		return "", err
	}
	defer os.Remove(tempName)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", path,
		"-map", "0:a:0", "-map_metadata", "0", "-c:a", "copy",
		"-metadata", "artist="+song.Artist, "-metadata", "title="+song.Title,
		"-metadata", fmt.Sprintf("track=%d", number), "-metadata", "album="+filepath.Base(filepath.Dir(source)), tempName)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("writing audio tags: %w", err)
	}
	if err := checkAudioFile(tempName); err != nil {
		return "", err
	}
	base := fmt.Sprintf("%03d - %s", number, cleanTrackName(song.Artist+" - "+song.Title))
	for suffix := 0; suffix < 1000; suffix++ {
		name := base
		if suffix > 0 {
			name += fmt.Sprintf(" (%d)", suffix+1)
		}
		dest := filepath.Join(dir, name+ext)
		if err := os.Link(tempName, dest); err == nil {
			return dest, nil
		} else if !os.IsExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("too many existing files with this track name")
}

func identifyDownloadedTracks(source string, paths []string, segments []audioChapter, cache recognitionRecord) (int, string, []string) {
	if os.Getenv("CLIAMP_IDENTIFY") == "0" {
		identified := 0
		for i := range paths {
			if track, ok := cache.track(source, i+1, segments[i]); ok {
				paths[i] = filepath.Join(filepath.Dir(source), track.Path)
				segments[i].Title = track.Song.Artist + " - " + track.Song.Title
				identified++
			}
		}
		return identified, "", nil
	}
	binary, binaryErr := songrecBinary()
	recognizer := songRecognizer{binary: binary}
	record := recognitionRecord{Version: 1}
	if info, err := os.Stat(source); err == nil {
		record.SourceSize, record.SourceMtime = info.Size(), info.ModTime().UnixNano()
	}
	identified := 0
	warning := ""
	unavailable := binaryErr != nil
	var originals []string
	for i, path := range paths {
		track, cached := cache.track(source, i+1, segments[i])
		if cached {
			paths[i] = filepath.Join(filepath.Dir(source), track.Path)
		}
		if !cached {
			track = identifiedTrack{Number: i + 1, Start: segments[i].Start, End: segments[i].End, Status: "unmatched"}
			if unavailable {
				track.Status = "unavailable"
				warning = "Identification unavailable; downloaded audio kept"
			} else if path == source && segments[i].End > 20*60 {
				// An unsplit long mix must not be named after just one song in it.
				track.Status = "unsplit mix"
			} else {
				duration, err := probeAudioDuration(path)
				if err != nil {
					track.Status = "audio could not be analyzed"
				} else {
					track.Song, track.Status, err = identifySong(path, duration, recognizer.sample)
					if err != nil {
						unavailable = true // fail fast for the rest of an offline/rate-limited batch
						warning = "Identification unavailable; downloaded audio kept"
					}
				}
				if track.Status == "identified" {
					if named, err := nameIdentifiedTrack(source, path, i+1, track.Song); err == nil {
						paths[i] = named
						if path != source {
							originals = append(originals, path)
						}
					} else {
						track.Status = "could not write tags"
						warning = "Some tracks could not be named; original audio kept"
					}
				}
			}
		}
		if track.Status == "identified" {
			identified++
			segments[i].Title = track.Song.Artist + " - " + track.Song.Title
		}
		track.Path, _ = filepath.Rel(filepath.Dir(source), paths[i])
		if info, err := os.Stat(paths[i]); err == nil {
			track.Size, track.Mtime = info.Size(), info.ModTime().UnixNano()
		}
		record.Tracks = append(record.Tracks, track)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err == nil {
		err = saveRecognitionRecord(filepath.Join(filepath.Dir(source), recognitionRecordName), append(data, '\n'))
	}
	if err != nil {
		warning = "Recognition record could not be saved; audio kept"
		originals = nil
	}
	return identified, warning, originals
}

func saveRecognitionRecord(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".cliamp-recognition-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

// Preserve numbered originals if a prior local playlist still references them.
func removeUnusedOriginals(folder string, originals []string) {
	playlists, err := localPlaylistPaths(folder)
	if err != nil {
		return
	}
	for _, original := range originals {
		relative, err := filepath.Rel(folder, original)
		if err != nil || !filepath.IsLocal(relative) || filepath.Dir(relative) != "Tracks" {
			continue
		}
		used := false
		for _, playlist := range playlists {
			for _, listed := range playlist {
				if listed == filepath.Clean(original) {
					used = true
				}
			}
		}
		if !used {
			_ = os.Remove(original)
		}
	}
}
