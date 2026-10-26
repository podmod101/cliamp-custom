# Playlist stream matching — 2026-10-25

## Worldwide discovery improvement

The new pipeline reasons locally over sampled playlist names/artists, available
audio descriptors and station history, searches worldwide radio directories and
live YouTube, and compares a diverse metadata pool before capturing candidate
audio. Radio Browser uses random ordering instead of votes, broad/adjacent tags,
random exploration and metadata caching. Soft country/language/broadcaster
diversity plus an interleaved uncertain lane avoid a hard genre gate. Candidate
URLs always come from catalogs/searches. Available schedule fields are used;
there is no arbitrary website scraper or invented schedule information.

Station scores contain measured CLAP audio only. Metadata predictions/history
allocate search effort but never increase final similarity. Initial 20-second
captures classify music versus possible speech/ad/noise, with delayed 30-second
rechecks when useful. Content classification remains approximate. Sample duration,
variability, coverage and prediction/measurement distinction are explicit.

Verified matches are published progressively and play normally with Enter.
Selection stays on the same station during reranking. s cancels/keeps verified
results; d starts deeper discovery; b cancels back to playlists; Escape closes.
Failures/timeouts retain verified previews. Shared service blocks back off.
Station history is bounded and expires after fourteen days for prioritization;
only same-schedule measurements less than two minutes old can supply a score.
Deeper search can reuse matching verified playlist references for a day.

Machine benchmark: 16 logical CPUs, CPU CLAP. Six 20-second radio samples took
25.1s with three capture workers and 14.0s with six; six 30-second samples took
25.1s. CLAP inference was about 0.4s/20s sample and 0.6s/30s sample. Buffered streams
can deliver faster than real time. These measurements selected six workers,
20-second first samples, 30-second rechecks and a normal shortlist capacity of
58. Capacity derives from timing within bounded 180s/300s audio budgets and
64/96 caps; uncalibrated machines use conservative defaults. YouTube captures
are limited to two concurrently. Audio-duration budgets proxy bandwidth rather
than claiming an exact byte cap. Diminishing-return stopping requires adequate
coverage before ending a pass, with time reserved for rechecks.

Validation: full make check passed; 32 offline Python tests passed, including
metadata/score separation, stale/schedule-changing cache rejection, random-order
catalog caching, weak-label inclusion, diverse selection, AI query inputs,
progressive verification, speech exclusion, policy calibration and variability.
Go tests cover validated preview delivery, unsafe targets, stale events, cursor
stability, immediate playback/queue preservation, stop/deeper/back controls,
timeout preservation and prediction/measurement labeling.

Separate live local-Qwen/CLAP run retrieved 276 distinct candidates across 18
catalog/search tasks, shortlisted 58, obtained music-like audio from 22 of 24
stations attempted, returned ten verified matches including radio and YouTube,
then stopped after three low-improvement batches and performed six rechecks.
Seven of the final ten had explicit local-AI station predictions; others remained
eligible as uncertain/exploration candidates. One possible speech/ad excerpt was
withheld and one capture failed. Final sample durations were 20 or 50 seconds.
Catalog metadata indicated ten countries and eleven languages sampled. Candidate
audio work took 114s, with first verified audio at about 9s; reasoning/discovery
plus audio took 225s using existing verified playlist references. Cold playlist
preparation adds time. Earlier live testing exposed an oversized reasoning reply;
the tested smaller shortlist response fixed it. No claim of exhaustive worldwide
coverage or subjective taste accuracy. No full authenticated Spotify UI session
was manually repeated; existing picker/auth behavior remains covered by tests.

Build: v2.0.1+search-history.youtube-cleanup.playlist-streams.worldwide, preserving
the upstream v2.0.1 base and all prior custom changes. Both local source copies,
isolated helper modules and user-local executable updated with rollback backups;
no commit/push, package installation or website publication. Spotify credentials,
configuration, music, model weights and gateway settings are preserved.
Documentation updated in worldwide/playlist/keybinding/Omarchy guides, README,
LOCAL-CHANGES and source website. Violin Hub student/teacher guides unaffected.
Exact installed paths, backup and rollback are in the central customization log.

## Follow-up: Local AI search planning and reference recovery

Long collaborator/version strings plus a forced "official audio" suffix produced
empty or wrong-song YouTube searches. The local Qwen model now produces concise
alternate queries from selected title/artist names. One feedback pass uses actual
returned titles to repair misses. Primary artist, meaningful title/version tokens,
duration and live/cover/speed checks still validate recordings independently.
Stylized spacing, accents, heart glyphs and store-only Original Mix labels are
normalized. Cached identities are rechecked; capture failures can try a second
verified upload. Malformed local-model JSON missing only closing containers is
handled without inventing names or values.

The worker aims for twelve usable references, replacing failed selections with
fresh random songs up to 36 distinct tracks and a six-minute reference budget.
The minimum stays five for larger playlists, all for smaller ones. Shared auth
or rate blocks stop further batches. Private cache diagnostics include search
queries, returned recording metadata and safe failure categories; raw subprocess
stderr and signed media URLs are never exposed. No metadata-only ranking or cloud
fallback was added. Spotify credentials/configuration and the Go binary remain
unchanged; this update installs Python helper modules only.

Validation: all 20 offline Python tests passed, including query mapping, actual
result feedback, replacement sampling after a 2/12 first batch, wrong-version
rejection, alternate-upload capture, cached identity validation, model JSON
handling, shared service blocks and redacted errors. Full `make check` passed.
Live local-Qwen/YouTube/CLAP checks obtained usable audio for five of six difficult
examples, then sixteen of eighteen random references with the feedback pass.
The latter tested reference preparation end to end; stream discovery/ranking
was covered by existing offline tests and prior live checks, not rerun here.
Authenticated playlist browsing is now confirmed by the user's report.

Both local source copies and the installed isolated helper were updated, with
backup and rollback details in the central customization log. Docs, LOCAL-CHANGES
and the source website were synchronized; no publication, package installation,
account reset or dependency change. Violin Hub student/teacher guides unaffected.

## Follow-up: Spotify playlist timeout

The reported `/v1/me/tracks: context deadline exceeded` was preceded by repeated
HTTP 429 responses. The existing full-library loader requested the profile and
Liked Songs before actual playlists, so an unrelated endpoint and long retry
waits blocked Alt+R. The picker now uses a playlist-only paginated loader with a
30-second deadline, request cancellation and an independent browse cache.

HTTP 429 errors preserve Retry-After (seconds or HTTP date), report long waits
immediately, and retain the provider cooldown across refresh/reopen. Error text
wraps and loading failures are distinct from empty libraries. Picker-owned
sessions are reused and released on quit. Existing playback/library behavior,
credentials and previous local customizations are preserved.

Validation: complete `make check` passed. New offline tests cover endpoint
selection, multi-page results/cache refresh, cancellation, 24-hour cooldowns,
refresh respecting cooldowns, rate limits surviving short request deadlines,
retry-header parsing, provider reuse and actionable UI error text. No live API
retry was issued against the known shared-client cooldown. Personal-client app
creation and browser OAuth approval require the user's Spotify account action;
authenticated live verification remains pending until setup is completed.

Docs and source website updated. Machine-specific backups, installed version,
personal-client setup state and rollback are maintained in the central log.

## Original feature

Added Alt+R on the player/provider screen. The picker browses Spotify playlists,
supports filtering and its existing browser sign-in flow, runs local audio
matching with progress/cancellation, and plays a selected result while retaining
the existing queue. Previous YouTube history, download, splitting, identification
and source-cleanup customizations are preserved.

The optional isolated Python worker samples random playlist songs, finds
independent public YouTube recordings, and compares their actual audio with live
radio/YouTube candidates using local CLAP embeddings. OpenClaw runs the configured
local text model in an isolated tools-disabled invocation with no cloud fallback.
Per-playlist taste hints are private configuration; ordinary conversations,
gateway settings and credentials are not changed. Candidate URLs come from live
directories/broadcasters/search metadata, never model generation.

Radio Browser server discovery and a public community mirror handle directory
outages. SomaFM supplies a second independent radio catalog. Candidate metadata
must provide relevant genre evidence; unrelated catalog entries do not pad the
results. Audio scores rank the successfully measured candidates, up to ten.

Validation: full `make check` passed (formatting, vet, all Go tests). Twelve
offline Python tests passed, including sampling/deduplication, audio-based
ordering, source matching, live filtering, genre evidence, broadcaster filtering,
private hint scoping, no cloud fallback and checkpoint validation. Go tests also
exercise cancellation of a real worker/child process, stale results, queue
preservation, auth handling and unsafe playback-target rejection. The isolated
runtime passes `pip check` and an offline model-load/self-check.

Live checks exercised public YouTube reference matching, short audio capture,
local model embeddings, isolated Qwen startup/reply/cleanup, both stream sources,
and measured ranking. A terminal session verified Alt+R, the sign-in screen,
Escape returning to the player, and normal quit. Browser OAuth approval and
end-to-end browsing inside an authenticated CLIamp session await the user's first
sign-in; this chat's Spotify credentials cannot be transferred to CLIamp.

The `laion/larger_clap_music` checkpoint was rejected after producing collapsed
embeddings, consistent with [benchmark issue 5069](https://github.com/embeddings-benchmark/mteb/issues/5069).
The installed feature uses `laion/clap-htsat-unfused`; startup rejects collapsed
text embeddings. Metadata and audio alone cannot guarantee subjective taste.
Sample availability, ambiguous recording identities and changing station music
limit results; failures are shown, and no metadata-only ranking is substituted.

Build: `v2.0.1+search-history.youtube-cleanup.playlist-streams.spotify-fix`, same upstream
v2.0.1 base. Both local source checkouts were updated without committing or
publishing. The previous executable and complete source checkouts were backed up
before promotion. Exact machine-specific backup locations and selective rollback
instructions are in the central Application Customizations log.

Guides updated: README, LOCAL-CHANGES, Spotify/keybinding/Omarchy setup docs,
new `docs/playlist-streams.md`, and the source website. No website was published.
Violin Hub's student/teacher guides are unaffected by this separate application
change. The audio runtime remains an explicit optional install for other machines.
