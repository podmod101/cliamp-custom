# Match live streams to a Spotify playlist

Press **Alt+R** from the player or its browser/queue views. Browse your Spotify
playlists, type `/` to filter, then press Enter on a playlist. The picker shows
progress while current playback continues. Arrow keys browse the resulting
matches; Enter appends the chosen stream to your queue and plays it immediately.
**Esc or m** minimizes the screen while searching continues. Normal playback,
queue and browser controls remain available. **s** cancels only when the search
screen is open; usable results remain. `b` returns to playlists and asks before
replacing running work.

**Alt+R starts a new search**, with confirmation before replacing an active job.
**Alt+L returns directly to the ongoing search or its latest results** without
choosing a playlist or processing again, including saved results after restart.
See [background navigation](background-stream-search.md) for a short control guide.

CLIamp uses its own Spotify login. If needed, Enter opens its existing browser
sign-in flow. Your Spotify connection in ChatGPT is separate. Spotify playback
does not need to be enabled in config.toml just to open this picker. Existing
configured Spotify providers and credentials are reused. Saved albums and podcast
episodes are excluded. Spotify's own account/API restrictions still apply.

The picker requests playlists directly, without first loading your profile,
Liked Songs or saved albums. Playlist loading has a 30-second deadline; **s**
cancels the request and Escape minimizes it. A failed request is shown as a load failure rather than an
empty library. Spotify HTTP 429 responses display the server's retry time; long
cooldowns are returned immediately, and refreshing or reopening the picker
respects the cooldown while CLIamp remains open.

If the error mentions the shared app quota, configure a personal Spotify app's
Client ID using [the Spotify setup guide](spotify.md#recommended-bring-your-own-client-id).
Register `http://127.0.0.1:19872/login` as its redirect URI. CLIamp's PKCE flow
does not need a Client Secret. Changing client IDs requires signing in again;
preserve a private backup of existing credentials first. A personal app receives
its own quota; it does not remove Spotify's rate limits.

Optional taste hints are stored privately in `vibe-preferences.json` in the
CLIamp config directory, keyed by Spotify playlist ID. A `description` guides
the local model; optional `radio_tags` and `youtube_queries` arrays calibrate
discovery for that playlist. Other playlists still receive automatic analysis.
These hints contain personal preferences and must stay out of source exports.

## Actual audio matching

The worker aims for twelve usable songs, balancing the ordered opening with
randomly sampled later pages. Matching metadata is bounded to six pages/300 songs,
while normal Spotify playback can still load the whole playlist. Your local OpenClaw text model creates concise alternate
YouTube searches from the selected song/artist names, handling collaborator lists,
stylized titles and remix names. A bounded feedback pass uses actual unsuccessful
search results to improve queries. Failed songs are replaced with fresh random
selections, up to 36 distinct songs and a six-minute reference-search budget.
Matching checks title, primary artist, duration, and alternate-version labels;
ambiguous matches are skipped. It captures a 30-second excerpt from each accepted
recording and computes local CLAP embeddings across up to three 10-second windows.
Spotify audio/previews are never captured or passed to an AI model.

The CLAP profile is sent to an isolated OpenClaw `agent exec` run using one
configured loopback model. Its temporary configuration contains only that local
provider, disables tools and plugins, and supplies no cloud fallbacks. OpenClaw
manages its existing localService startup/cleanup. Ordinary agent conversations,
channels, stored credentials, and gateway settings are not changed. The text
model receives selected song/artist names for search planning, and audio-derived
descriptors and independently retrieved YouTube metadata for stream discovery.
Inference remains local; there is no cloud fallback.

Worldwide discovery now uses a local metadata-reasoning shortlist followed by
progressive current-audio verification. It searches Radio Browser, SomaFM and
explicitly live YouTube channels without geographic/language restrictions or
popularity ordering. Metadata predictions never increase measured scores.
Initial station samples are 20 seconds, with selected 30-second rechecks.
Verified matches are playable while searching continues; s stops and keeps them,
and d starts a deeper pass. At least five usable playlist references are still
required (all for tiny playlists); metadata-only recommendations are never used.

See [worldwide discovery](worldwide-streams.md) for tested resource policies,
adaptive stopping, cache freshness, uncertainty and the meaning of worldwide.

First runs take several minutes. Subsequent runs reuse song embeddings for 30
days; station profiles guide future search for up to fourteen days, with only
measurements less than two minutes old eligible for final reuse; the random song selection changes.
Live programming and music vary, and a short sample may not represent an entire
station. Recording identity checks are conservative, not infallible. Speech,
remixes and intros can still affect results. CLAP is a general audio model and
does not measure subjective taste perfectly.

## Optional audio runtime setup

This opt-in feature needs Python 3, ffmpeg, yt-dlp, OpenClaw with `agent exec`,
and a configured local HTTP model provider. The existing source installer does
not automatically download the audio runtime or model. Run:

```sh
python3 scripts/install-playlist-streams.py
```

This creates an isolated virtual environment under
`~/.local/libexec/cliamp-vibe/`, installs CPU-only PyTorch and the pinned audio
runtime, and downloads `laion/clap-htsat-unfused` into
`~/.local/share/cliamp-vibe/models/`. It uses no sudo or paid inference service.
The CLAP model is Apache-2.0; dependencies keep their respective licenses.
The installer preserves an existing helper in a dated sibling backup.

Rebuild CLIamp from this source; do not replace the custom player with an
upstream binary. The helper's model inference is offline after installation.
Normal use still contacts Spotify for playlists, YouTube for independent samples,
and Radio Browser, SomaFM and station hosts for discovery and audio. No song recordings are
retained. Bounded embedding caches and the latest private result are stored in
`${XDG_CACHE_HOME:-~/.cache}/cliamp/vibe/`; temporary audio is deleted after each
run. The latest reference-search diagnostic is saved privately as
`latest-reference-search.json` in that cache, including queries, returned titles
and failure categories. It contains playlist-related names; keep it private.
Errors distinguish ambiguous matches, authentication blocks, unavailable formats
and timeouts without exposing signed media URLs. Explicit cancellation or quitting
terminates worker children; minimizing keeps them running. There is a 15-minute
matching deadline (20 minutes for deeper search).

`OPENCLAW_CONFIG_PATH` can select a different OpenClaw config. The first configured
loopback HTTP provider with a model is used; remote providers are never selected.
`CLIAMP_VIBE_HELPER` can override the helper executable path. Credentials, search
history, and playlist data must never be copied into the source repository.

## Verification and rollback

Run `make check` for the Go application, and run the offline Python tests with
the isolated interpreter:

```sh
~/.local/libexec/cliamp-vibe/venv/bin/python scripts/test_playlist_streams.py
~/.local/libexec/cliamp-vibe/venv/bin/python scripts/test_song_search.py
```

Tests do not call streaming services or load model weights. Actual network/audio
matching is a separate manual smoke test. To undo, restore the previous custom
CLIamp executable and the changed source files from your installation backup.
Keep existing Spotify credentials, music, history, and customizations. The helper
and its model/cache directories can be archived separately. Upstream upgrades
need this feature ported and retested; OpenClaw CLI or model-runtime changes may
also require updating the helper.
