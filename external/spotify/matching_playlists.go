package spotify

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/bjarneo/cliamp/playlist"
)

// MatchingPlaylists fetches only playlists. Liked Songs, saved albums and the
// profile endpoint are not prerequisites for the audio-matching picker.
func (p *SpotifyProvider) MatchingPlaylists(ctx context.Context) ([]playlist.PlaylistInfo, error) {
	if err := p.ensureSessionContext(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	if p.matchingListCache != nil && time.Since(p.matchingListCacheAt) < playlistListCacheTTL {
		cached := slices.Clone(p.matchingListCache)
		p.mu.Unlock()
		return cached, nil
	}
	p.mu.Unlock()
	all := make([]playlist.PlaylistInfo, 0)
	for offset := 0; ; {
		query := url.Values{"limit": {strconv.Itoa(spotifyPlaylistPageSize)}, "offset": {strconv.Itoa(offset)}}
		resp, err := p.webAPI(ctx, "GET", "/v1/me/playlists", query)
		if err != nil {
			return nil, fmt.Errorf("spotify: load playlists: %w", err)
		}
		var page struct {
			Items []spotifyPlaylistItem `json:"items"`
			Total *int                  `json:"total"`
		}
		if err := decodeBody(resp, &page); err != nil {
			return nil, fmt.Errorf("spotify: parse playlists: %w", err)
		}
		p.mu.Lock()
		if p.matchingDescriptions == nil {
			p.matchingDescriptions = make(map[string]string)
		}
		for _, item := range page.Items {
			if item.ID == "" {
				continue
			}
			count := 0
			if item.Items != nil {
				count = item.Items.Total
			}
			all = append(all, playlist.PlaylistInfo{ID: item.ID, Name: item.Name, TrackCount: count, Section: "Playlists"})
			p.matchingDescriptions[item.ID] = item.Description
			cached := p.trackCache[item.ID]
			if cached == nil || cached.snapshotID != item.SnapshotID {
				p.trackCache[item.ID] = &playlistCache{snapshotID: item.SnapshotID}
				delete(p.matchingTrackCache, item.ID)
			}
		}
		p.mu.Unlock()
		offset += len(page.Items)
		if len(page.Items) < spotifyPlaylistPageSize || (page.Total != nil && offset >= *page.Total) {
			break
		}
	}
	p.mu.Lock()
	p.matchingListCache, p.matchingListCacheAt = all, time.Now()
	p.mu.Unlock()
	return slices.Clone(all), nil
}

// Refresh invalidates browse/track caches, while respecting any active Spotify
// cooldown. Pressing refresh must never bypass the server's Retry-After.
func (p *SpotifyProvider) Refresh() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.listCache = nil
	p.matchingListCache = nil
	p.matchingTrackCache = nil
	p.matchingDescriptions = nil
	p.trackCache = make(map[string]*playlistCache)
}
