package spotify

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/url"
	"slices"
	"strconv"

	"github.com/bjarneo/cliamp/playlist"
)

// MatchingTracks reads a bounded, ordered-head-plus-random-pages metadata sample.
// Playback's Tracks method remains unchanged and can still load the full list.
func (p *SpotifyProvider) MatchingTracks(ctx context.Context, id string) ([]playlist.Track, error) {
	if err := p.ensureSessionContext(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	if tracks := p.matchingTrackCache[id]; tracks != nil {
		p.mu.Unlock()
		return slices.Clone(tracks), nil
	}
	p.mu.Unlock()
	var tracks []playlist.Track
	offsets := []int{0}
	for pageIndex := 0; pageIndex < len(offsets); pageIndex++ {
		offset := offsets[pageIndex]
		query := url.Values{"limit": {strconv.Itoa(spotifyTrackPageSize)}, "offset": {strconv.Itoa(offset)},
			"fields": {"items(item(id,name,type,uri,artists(name),album(name,release_date),duration_ms,track_number,is_playable,restrictions(reason))),total"}}
		resp, err := p.webAPI(ctx, "GET", "/v1/playlists/"+id+"/items", query)
		if err != nil {
			return nil, fmt.Errorf("spotify: sample playlist metadata: %w", err)
		}
		var result struct {
			Items []struct{ Item, Track *spotifyItem }
			Total int
		}
		if err := decodeBody(resp, &result); err != nil {
			return nil, err
		}
		for _, item := range result.Items {
			t := item.Item
			if t == nil {
				t = item.Track
			}
			if t != nil && t.ID != "" {
				tracks = append(tracks, trackFromItem(t))
			}
		}
		if pageIndex == 0 {
			pages := (result.Total + spotifyTrackPageSize - 1) / spotifyTrackPageSize
			if pages > 1 {
				offsets = append(offsets, spotifyTrackPageSize)
			}
			if pages > 2 {
				for _, page := range rand.Perm(pages - 2)[:min(4, pages-2)] {
					offsets = append(offsets, (page+2)*spotifyTrackPageSize)
				}
			}
		}
	}
	p.mu.Lock()
	if p.matchingTrackCache == nil {
		p.matchingTrackCache = make(map[string][]playlist.Track)
	}
	p.matchingTrackCache[id] = slices.Clone(tracks)
	p.mu.Unlock()
	return tracks, nil
}

// MatchingPlaylistDescription carries curator intent into local matching without
// an extra request or exposing unrelated account/library data.
func (p *SpotifyProvider) MatchingPlaylistDescription(id string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.matchingDescriptions[id]
}
