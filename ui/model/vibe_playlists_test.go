package model

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/external/spotify"
	"github.com/bjarneo/cliamp/playlist"
)

type matchingOnlyProvider struct {
	commandsTestProvider
	ctx context.Context
}

func (p *matchingOnlyProvider) Playlists() ([]playlist.PlaylistInfo, error) {
	return nil, errors.New("full library loader must not be used")
}

func (p *matchingOnlyProvider) MatchingPlaylists(ctx context.Context) ([]playlist.PlaylistInfo, error) {
	p.ctx = ctx
	return []playlist.PlaylistInfo{{ID: "one", Name: "Sample playlist"}}, nil
}

func TestVibeUsesPlaylistOnlyLoaderWithBoundedContext(t *testing.T) {
	p := &matchingOnlyProvider{}
	event := vibeListsCmd(context.Background(), p, 3, false)().(vibeEvent)
	deadline, bounded := p.ctx.Deadline()
	if event.err != nil || len(event.lists) != 1 || !bounded || time.Until(deadline) > 30*time.Second {
		t.Fatalf("Wrong loader or unbounded request: %+v", event)
	}
}

func TestVibeReusesItsSpotifyProviderAfterClosing(t *testing.T) {
	m := keybindingTestModel()
	m.openVibeStreams()
	source := m.vibe.provider
	m.closeVibeStreams()
	m.openVibeStreams()
	if m.vibe.provider != source {
		t.Fatal("Reopening discarded provider and its server cooldown")
	}
	m.closeVibeStreams()
	m.releaseVibeProvider()
}

func TestVibeDisplaysLoadFailureAndReadableRateLimit(t *testing.T) {
	m := keybindingTestModel()
	m.width, m.height = 80, 24
	m.vibe = vibeState{visible: true, gen: 1}
	m.handleVibeEvent(vibeEvent{gen: 1, finished: true, err: &spotify.RateLimitError{
		Until: time.Now().Add(time.Hour), SharedClient: true,
	}})
	view := m.renderVibeStreams()
	if !strings.Contains(view, "Could not load") || strings.Contains(view, "No playlists found") ||
		!strings.Contains(view, "cliamp setup") {
		t.Fatalf("Missing actionable, untruncated error: %q", view)
	}
}
