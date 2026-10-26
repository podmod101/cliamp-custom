# Playlist stream customization history

## 2026-10-26: Background search and simple navigation

Esc/m now minimizes without cancelling. Alt+L returns to active progress or the
latest analyzed list without reprocessing. Enter plays and minimizes while work
continues. Normal playback, queue and provider controls work while hidden, with
a compact searching/results-ready indicator. New hidden results/errors never
open a screen or change playback. On the search screen s explicitly cancels and
keeps results. Alt+R/b ask before replacing a running search; y confirms and
n/Esc keeps it. This supersedes earlier Escape/Enter cancellation behavior.
See [background navigation](background-stream-search.md).

## 2026-10-26: Separate new search from saved results

Alt+R now always starts the playlist search workflow; Alt+L returns directly to
the saved analyzed list with the last selection and no provider/model/capture
work. Results are private, persist across restart and retain original measurement
timestamps. This supersedes the earlier session-only Alt+R reopening behavior.
See [saved-result report](../REPORT-last-results.md) and [guide](worldwide-streams.md).

## 2026-10-25: Playlist genre intent and varied compatible results

Alt+R now carries the curator's description into local reasoning and uses bounded,
stratified playlist metadata: first two ordered pages plus up to four random later
pages. This prevents an enormous mixed playlist's later additions from entirely
displacing its stated musical direction. Reference preparation balances opening
and later selections. Normal Spotify playback's full-track loading is unchanged.

Press g on results to choose a locally derived genre such as Electronic / EDM,
Glitch / Bass or Blues and start a focused verified search. Existing playlist
reference embeddings are reused; prior results remain available under their own
label until replacements arrive. Measured genre compatibility, not general cosine
alone, determines eligibility. A broader electronic caption set covers DnB/house/
glitch and related subgenres. Genre detection remains approximate, with overlap
disclosed. Fewer results are returned instead of off-genre padding.

Raw scores are preserved and explicitly labeled; 0.8 is not 80% genre confidence.
Compatible near-matches are selected for measured audio variety, so rows need not
be in strictly descending score order. No artificial score spreading or metadata
boost. Regression and live validation details: REPORT-genres.md.

## 2026-10-25: Return to analyzed results

After playing a recommendation, Alt+R reopens the same analyzed list at the last
selection without provider, AI or audio work. Escape also preserves results for
reopening. b explicitly returns to playlists; result memory lasts only for the
current CLIamp session. Regression tests cover completed/progressive results,
another playback choice, queue preservation and normal playlist browsing.

## 2026-10-25: Worldwide discovery, progressive verification and deeper search

Alt+R now uses local playlist/scene reasoning, worldwide metadata search and a
diverse shortlist before measuring candidate audio. Radio searches use random
ordering without geographic/language restrictions; broad/adjacent searches and
an interleaved exploration lane retain weakly labeled stations. Live YouTube
results require an explicit live flag, without a hard genre gate.

Audio-only CLAP scores stay separate from metadata predictions. Verified results
appear progressively and play immediately; s stops and keeps results, d starts
a deeper pass, b cancels back to playlists and Escape cancels/closes. Selection
stays on the same station through ranking updates; timeout retains verified
previews. Current measurements, sample duration, uncertainty and catalog coverage
are visible. Speech/ad/noise-like samples are withheld and selected candidates
can receive delayed rechecks; this content classification remains approximate.

Machine measurements selected six radio captures at once, initial 20-second
samples and 30-second rechecks. Resource settings remain local; uncalibrated
machines use conservative defaults. Shortlist size derives from measured costs,
with time/audio-duration budgets, failure backoff and diminishing-return stops.
Old station profiles guide selection; only measurements less than two minutes
old can supply a current score. No background scan, dependency installation,
cloud inference or Spotify-account changes. Existing custom features preserved.

Implementation: `scripts/cliamp_vibe/world_*.py`, `station_profiles.py`,
`reference_cache.py`, audio/engine integration and `ui/model/vibe_*.go`.
Offline tests: `scripts/test_worldwide.py`, existing audio/search tests and
progressive Go worker/UI tests. Optional live tuning: `scripts/benchmark-worldwide.py`.
Policy, privacy, limitations and controls: [worldwide guide](worldwide-streams.md).

## 2026-10-25: Local AI song searches and replacement sampling

Alt+R uses the configured local OpenClaw model to generate alternate searches
from selected song/artist names, with a bounded retry informed by actual YouTube
results. It handles collaborator lists, stylized names and store-only version
labels while retaining primary-artist, title, remix and duration validation.
Failed captures can try a second verified upload. Fresh random songs replace
failed references, up to 36 distinct songs and a six-minute search budget;
twelve usable references remain the target and five the minimum (all for tiny
playlists). Shared authentication/rate blocks stop further batches.

Private cache diagnostics distinguish lookup, capture and model failures;
subprocess errors expose categories instead of signed URLs. Audio still comes
from independent public YouTube recordings and is measured locally with CLAP.
No Spotify audio, cloud inference, account/config changes or new dependencies.
This is a Python-helper update; the existing custom Go executable is unchanged.
Offline regressions: `scripts/test_song_search.py` and the existing audio suite.

## 2026-10-25: Spotify playlist-loading and rate-limit fix

Alt+R now requests Spotify playlists directly, without blocking on profile,
Liked Songs or saved-album requests. Loading is bounded to 30 seconds and can
be cancelled. Failed loads are distinguished from empty libraries, and long
errors wrap so the retry/setup guidance remains visible.

Spotify 429 responses preserve the server's retry time and show long cooldowns
immediately. Refresh/reopen retains the provider and respects its cooldown.
The picker-owned Spotify session is released on player exit. Rate-limit errors
no longer suggest resetting valid credentials. Personal app client-ID setup is
documented; existing music, login data and unrelated features are preserved.

Code/tests: `external/spotify/matching_playlists*.go`, `rate_limit.go`,
provider context/retry integration, `ui/model/vibe_playlists_test.go` and picker
integration. Complete `make check` passed with offline regression fixtures.

## 2026-10-25: Spotify playlist audio matching (Alt+R)

This build additionally provides **Alt+R** on the player/provider screen: browse
Spotify playlists, select one, and find up to ten live internet-radio/YouTube
matches using independently sourced audio samples. Enter plays a selected match
without discarding the existing queue. Escape cancels. Uses the optional isolated
CLAP runtime and a tools-disabled local OpenClaw `agent exec` run,
with no cloud fallbacks. Existing download/history/identification features remain.

Implementation: `ui/model/vibe_*.go`, `scripts/playlist_streams.py`,
`scripts/cliamp_vibe/`, optional installer `scripts/install-playlist-streams.py`.
Integration touches model state, key dispatch/registry, Update and View. Tests are
`ui/model/vibe_streams_test.go` and `scripts/test_playlist_streams.py` (offline).
Setup, privacy, limitations and undo: [docs/playlist-streams.md](playlist-streams.md).
The source website, README and keybinding/provider docs describe the feature.
Custom build version: `v2.0.1+search-history.youtube-cleanup.playlist-streams.genres`.

The audio runtime is user-local and optional; neither it nor model weights,
song samples, Spotify credentials or cache data belongs in the repository.
Source upgrades must preserve these additions and recheck OpenClaw CLI behavior.
