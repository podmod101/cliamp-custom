package model

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/player"
	"github.com/bjarneo/cliamp/playlist"
)

type streamErrFakeEngine struct {
	*playbackFakeEngine
	err error
}

func (f *streamErrFakeEngine) StreamErr() error { return f.err }

func TestTickStreamErrorReconnectsUnlessAudioOutputFailed(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantReconnect bool
	}{
		{name: "connection drop reconnects", err: errors.New("connection reset"), wantReconnect: true},
		{name: "failed audio output is shown", err: fmt.Errorf("%w: ALSA error", player.ErrAudioOutput), wantReconnect: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pl := playlist.New()
			pl.Add(playlist.Track{Title: "Radio", Path: "http://radio.example/stream", Stream: true})
			m := Model{
				player:   &streamErrFakeEngine{playbackFakeEngine: &playbackFakeEngine{playing: true}, err: tt.err},
				playlist: pl,
			}

			updated, _ := m.Update(tickMsg(time.Now()))
			m = updated.(Model)

			if got := !m.reconnect.at.IsZero(); got != tt.wantReconnect {
				t.Fatalf("reconnect scheduled = %v, want %v (err = %v)", got, tt.wantReconnect, m.err)
			}
			if !tt.wantReconnect && !errors.Is(m.err, player.ErrAudioOutput) {
				t.Fatalf("m.err = %v, want ErrAudioOutput", m.err)
			}
		})
	}
}
