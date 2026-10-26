# Source publication review — 2026-10-23

This repository is a fresh source snapshot of CLIamp v2.0.1 plus the custom
features documented in `LOCAL-CHANGES.md`. The original local Git directory,
commits, build outputs and ignored files were not imported. The upstream base
commit is recorded in `omarchy.md`; upstream copyright and MIT terms remain.

The export was checked for machine-specific paths, the local owner's private
email/name, private-key markers, personal application configuration, downloaded
music, browser cookies, recognition caches, search history and backup archives.
None were included. The public GitHub account name and repository URL are
intentional. The included demo audio/images are unchanged upstream project assets.

Gitleaks 8.30.1 scanned the complete export before upload. It reported three
findings: two synthetic keys in upstream `external/qobuz/bundle_test.go` and the
publicly shipped fallback OAuth client credential in upstream
`external/tidal/creds.go`. Both files were verified byte-for-byte against the
upstream base. These are inherited test/shared-client values, not the local
owner's credentials. No findings were suppressed in the scanner configuration.

No proprietary code or model weights were added. The custom Go code uses the
standard library and existing CLIamp packages; it invokes yt-dlp, FFmpeg and
SongRec as separate tools. Linux build dependencies were checked against their
local source license files, including permissive licenses and the GPL Spotify
dependency. Dependencies are not vendored here. See `omarchy.md` for license
details and the distinction between open-source SongRec and Shazam's external
online recognition service.

The new repository's GitHub Actions are disabled. The inherited workflow files
are retained as source references; release, AUR and Pages publishing need a
separate review before enabling them for this repository. No website or binary
release was published as part of the source upload.

## 2026-10-26: Playlist discovery and background navigation

The custom-v2.0.1 branch adds the integrated Spotify playlist picker, local
OpenClaw search planning, independent public YouTube song samples, local CLAP
stream verification, worldwide radio/live-YouTube discovery, genre searches,
private saved results and minimizable background searching. The newer main
branch uses a different upstream base; this publication does not replace it.

The pending source inventory was checked for unexpected file types, large files,
machine paths, personal playlist identifiers/titles, private-key markers and
credential assignments. Reviewed credential-related matches are runtime field
names or synthetic test fixtures. No personal configuration, account tokens,
cookies, music, downloaded weights, runtime caches, local backups or binaries are
included. This is a source/content review, not a new Gitleaks scan; the original
scanner findings above concern unchanged upstream values.

Audio/model dependencies remain optional and isolated; weights are downloaded
only by the opt-in runtime setup and are not bundled in Git. Existing upstream
and dependency licenses remain applicable. Offline Go/Python tests and the
background-worker race fixtures verify behavior without calling live services
or running model inference. The branch-specific clone command and site guide
link select the source version containing these features.
