# YouTube, SoundCloud, Mixcloud, NetEase, Bandcamp and Bilibili

Install [yt-dlp](https://github.com/yt-dlp/yt-dlp), then play YouTube, SoundCloud, Mixcloud, NetEase, Bandcamp, and Bilibili URLs:

```sh
cliamp https://www.youtube.com/watch?v=dQw4w9WgXcQ
cliamp https://soundcloud.com/artist/track
cliamp https://www.mixcloud.com/creator/show-name/
cliamp 'https://music.163.com/#/song?id=1973665667'
cliamp https://artist.bandcamp.com/album/name
cliamp https://www.bilibili.com/video/BV1xxxxxxxxx
cliamp https://space.bilibili.com/uid/lists/id  # season/series playlists
```

You can play playlists and albums. Press `Ctrl+S` to save a downloaded track to `~/Music/cliamp/`.

## Full YouTube downloads with track identification (local customization)

On the main player screen, **Alt+S** downloads the current YouTube/YouTube Music
item's complete audio. Live and upcoming broadcasts are rejected; archived
broadcasts and unknown-duration items are allowed without a fixed duration limit.
Playback position does not trim the saved audio.

Each download goes into `~/Music/<shortened stream title> [video ID]/`. CLIamp
temporarily saves `Full stream.<extension>`, splits tracks using YouTube chapter starts, and
otherwise tries FFmpeg silence detection. Silence-based cuts are estimates:
quiet gaps below -45 dB lasting two seconds qualify, with at least 60 seconds
between cuts and before the end. Crossfades can remain joined.

After splitting, CLIamp uses [SongRec](https://github.com/marin-m/SongRec) to
identify the tracks through Shazam's online service. It sends fingerprints of
short audio samples, not the recording itself. Two separate samples must agree
on a song ID; a third is tried when only one sample matches. Conflicting,
unmatched, and very short tracks keep their chapter/numbered names. An unsplit
recording longer than 20 minutes is kept as a mix rather than named after a song
sampled from it. Internet access and a matching recording in Shazam are required.

Confirmed tracks are written as `Tracks/001 - Artist - Song Title.<extension>`
with embedded artist, title, track number, and stream-folder album tags. Audio
is copied without re-encoding. The `.m3u8` playlist follows the resulting names
and preserves stream order. Existing audio and playlists are never overwritten;
an `- Identified` playlist is created if an older playlist has different entries.
Old numbered tracks are retained when an existing playlist in the stream folder
still references them.

Once processing and playlist creation finish, CLIamp removes the redundant full
recording only when playable files in `Tracks/` cover its entire duration. It
checks every track's duration, requires a saved playlist containing every track,
and preserves the full recording if a local M3U/PLS playlist still references it.
Missing, empty, damaged, incomplete or unsplit output keeps the recovery source.
A confirmed standalone song can replace the continuous file with a complete
tagged track. Unmatched split tracks remain playable and can be identified later
without the full recording. Split failures keep the original recording.

`.cliamp-identification.json` in the stream folder records matches and lets later
saves reuse unchanged, confirmed tracks. Cleanup records the source's SHA-256
digest so identities survive removal and an identical re-download, while changed
source audio invalidates them. Recognition errors leave the audio and
playlist usable; the status line reports a warning or the number identified.
Keep CLIamp open until downloading, splitting, and identification have finished.
Lookups are sequential, spaced at least ten seconds apart, and each has a
30-second timeout. Offline/rate-limited batches stop further requests.

Dependencies: `yt-dlp`, `ffmpeg`, `ffprobe`, and SongRec. This machine's verified
SongRec 0.7.5 helper is at `~/.local/libexec/cliamp/songrec`; a `songrec` executable
on PATH is also supported. `CLIAMP_SONGREC` can override the executable path.
Set `CLIAMP_IDENTIFY=0` before launching CLIamp to download/split without online
identification. No Hugging Face weights or local song catalog are used.

You can also play live streams, such as 24/7 YouTube lofi radios. They have no audio-only formats. cliamp uses the best muxed stream and plays only its audio track. This also applies to live-stream URLs used as stations in `radios.toml`.

## Search

Search and play from the command line:

```sh
cliamp search "never gonna give you up"       # search YouTube
cliamp search-sc "lofi beats"                  # search SoundCloud
```

In the TUI, press `Ctrl+F` to search the active provider. This searches YouTube for YouTube/YT-Music, SoundCloud for SoundCloud, Mixcloud for Mixcloud, and NetEase for NetEase. These providers have signed-in playback guides: [SoundCloud](soundcloud.md), [Mixcloud](mixcloud.md), and [NetEase](netease.md).

## Disclaimer

**Use at your own risk.** Downloading or streaming copyrighted content can violate the terms of service of these platforms. You are responsible for use of this feature.
