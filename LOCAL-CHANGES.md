# Local YouTube search history, downloads, and track identification

Based on CLIamp v2.0.1, with the last 20 submitted YouTube queries saved in
`~/.config/cliamp/youtube-search-history.json` (or the configured CLIamp config
directory). Up recalls older queries; Down recalls newer queries and finally
restores the unfinished draft. Recalled queries can be edited and submitted.

On the main playback screen, **Alt+S** saves the current YouTube or YouTube
Music item's entire audio to `~/Music/<stream title shortened to 120 bytes> [video ID]/`.
It uses the same S-for-save mnemonic as the existing Ctrl+S, which still saves
to `~/Music/cliamp`. Live and upcoming broadcasts are rejected by yt-dlp;
archived broadcasts and unknown-duration videos are allowed. There is no
duration limit and playback position does not limit the download.

The stream folder initially contains `Full stream.<audio extension>`, a matching-name
`.m3u8` playlist, and a `Tracks/` folder when boundaries are found. Prefer
YouTube chapters; otherwise use installed FFmpeg's silence detector (below
-45 dB for at least two seconds, with at least 60 seconds between cuts and
before the end). Split files retain their original audio encoding. Chapters
name the numbered tracks; silence-based tracks have generic names marked
`estimated`. Crossfades may remain joined, and pauses inside songs can cause
incorrect splits. No acoustic-change model is used. Failures to detect or split
tracks fall back to a playlist of the full recording. Existing audio, tracks,
and playlists are preserved. Wait for completion before closing CLIamp.

After splitting, SongRec identifies tracks using Shazam's online service.
Two separate 12-second samples must match the same song ID; a third sample
can confirm one otherwise uncorroborated match. Conflicting or missing results
keep their chapter/numbered names. Tracks under 24 seconds are not renamed,
and an unsplit mix longer than 20 minutes is not labeled as one song.
Confirmed files become `Tracks/001 - Artist - Song Title.<extension>` and receive
artist, title, track-number, and stream-folder album tags without re-encoding.
The playlist uses those names in stream order. If an existing playlist differs,
it is preserved and a separate `- Identified.m3u8` is written. Older numbered
tracks stay when an existing local playlist still references them.

After trimming, naming, tags, and playlist creation complete, remove the full
continuous recording only if all replacement tracks are playable, cover its
entire duration without gaps, and appear in a saved playlist. Keep it if any
local M3U/PLS playlist still references it, splitting failed, or outputs cannot
be verified. This also works for a complete tagged standalone song. Unmatched
split tracks retain their audio and can be named later; a master is not needed
solely because a song could not be identified. No background cleanup is used.
The recognition record stores the source's SHA-256 digest and removal marker so
confirmed identities survive deletion and identical re-downloads.

Each stream folder's `.cliamp-identification.json` caches confirmed identities
for the same source file, track boundaries, and unchanged tagged files. Missing
matches are not invented, and offline/rate-limited batches stop further requests.
Requests are sequential, at least ten seconds apart, with a 30-second timeout.
Only audio fingerprints are sent to Shazam; there are no downloaded Hugging Face
weights. `CLIAMP_IDENTIFY=0` skips online recognition while reusing cached names.

Dependencies: `yt-dlp`, `ffmpeg`, and `ffprobe` on PATH. The new download ignores
standalone yt-dlp config to avoid accidental clips or destination overrides,
while retaining cookies configured within CLIamp.

Install `songrec` through the target machine's package manager. The code finds
`CLIAMP_SONGREC`, then `~/.local/libexec/cliamp/songrec`, then `songrec` on PATH.
The helper is not bundled. See [the Omarchy setup guide](docs/omarchy.md).

Source additions: `resolve/youtube_download.go`, `resolve/audio_tracks.go`,
their tests, and `ui/model/youtube_download.go` and its tests. The hotkey/help,
completion message, and status handling also change `ui/model/keys.go`,
`ui/model/command_registry.go`, `ui/model/commands.go`, and `ui/model/update.go`.
Identification code/tests are in `resolve/track_identification.go` and
`resolve/track_identification_test.go`. Cleanup is in `resolve/recording_cleanup.go`
and `resolve/recording_cleanup_test.go`; its exported maintenance helper validates
previously completed downloads using their saved track record. User docs and the source-local website
are updated in `docs/keybindings.md`, `docs/yt-dlp.md`, and `site/index.html`.

The custom executable is installed at `~/.local/bin/cliamp`. Its version is
`v2.0.1+search-history.youtube-cleanup`. The portable installer, launcher setup,
Claude prompt, update instructions, backups, and rollback are documented in
[docs/omarchy.md](docs/omarchy.md). It builds this checkout and never fetches an
upstream prebuilt CLIamp binary.

The system package at `/usr/bin/cliamp` stays managed by pacman. New upstream
versions need the patch ported and the executable rebuilt. Do not use upstream
self-update to maintain this custom build.

Verification uses real installed yt-dlp with local metadata/audio fixtures,
real FFmpeg/ffprobe, and synthetic audio with a known silence gap. It covers
live/upcoming rejection, archived/unknown-duration acceptance, complete audio,
title shortening, chapter and silence splits, playable split durations,
playlist ordering, preserving originals and existing files, error fallback,
hotkey handling, and the existing search-history suite. Normal tests do not
contact YouTube or Shazam. Recognition fixtures verify sample agreement,
conflicting/absent results, tags and names, collision preservation, offline
fallback, retained recovery sources and playlists, cache reuse, and temporary cleanup.
The opt-in `TestSongIdentificationOnlineSmoke` was run using a public Apple song
preview; real SongRec/Shazam identified Daft Punk's One More Time and completed
the artist/title tagging and playlist pipeline. No actual YouTube download or
manual terminal UI session was used, and real DJ mix accuracy is not guaranteed.

Cleanup tests verify full-duration replacements, missing/empty/truncated files,
gaps and omitted endings, absent/incomplete playlists, relative/file-URI/PLS
references to the master, failed metadata writes, track symlinks, and
identity-cache survival after removal or identical re-download. A changed source
with the same size and timestamp must still fail the digest check.

To return to the packaged player, follow the selective rollback instructions
in [docs/omarchy.md](docs/omarchy.md). Keep music, playlists, and history.
