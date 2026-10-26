# Background playlist-stream search — 2026-10-26

The search screen now has a separate lifetime from its worker. Esc/m hides the
screen; Alt+L reopens the same active progress or latest results/cursor without
another provider/model/capture command. Enter plays and minimizes while work
continues. Normal keyboard routing resumes immediately on the underlying screen.
The existing event waiter continues through hidden previews, completion and errors.
No hidden event opens the screen or changes playback.

On the search screen, s cancels even before the first verified match and retains
usable results. Alt+R/b request confirmation before replacing running work: y
cancels and enters a new playlist workflow, n/Esc keeps it, m minimizes. Genre
and deeper passes require the current pass to end first. A compact status shows
searching/count, results ready or an error needing attention, plus Alt+L. The
indicator also appears in simplified/full-visualizer layouts and queue/browser
screens. Narrow layouts prioritize the shortcut over network-byte statistics.

Playlist filter text accepts m/s normally. Escape leaves filter/genre editing
first. Underlying browser/queue selection survives reopening the search. New
playlist browsing retains the last saved result snapshot; usable results remain
available after restart under the existing owner-only atomic snapshot policy.
Active jobs are not restart-resumable: quitting CLIamp cancels the worker.

Preserved: Spotify login/picker, OpenClaw and CLAP pipeline, worldwide discovery,
genre/variety/raw-score behavior, saved-result compatibility, custom YouTube
history/downloads/identification/cleanup, queue and standard playback behavior.
No Python/model/dependency/config/credential/cache changes or live search runs.

Validation: make check/build (gofmt, vet, all Go packages); focused race checks;
new regression tests cover a gated real helper subprocess delivering progress,
preview and completion while minimized, one playlist fetch, pause/track change,
queue/provider navigation, independent music stop, reopen before/after finish,
second-result playback without reprocessing, pre-result cancellation, stale-event
rejection, replacement confirmation and failure without focus/playback changes.
Frame tests cover visible indicators and dimensions at 40x10, 80x24 and 120x32
in player, queue, simplified and visualizer layouts. Existing snapshot and
discovery tests also pass. The helper fixture is offline; it verifies lifecycle
and routing rather than live Spotify/YouTube availability or match quality.

User-facing docs, keymap descriptions, static site and portable installer version
are synchronized. This build is based on v2.0.1 and identifies as
v2.0.1+search-history.youtube-cleanup.playlist-streams.background. Read the central
Application-Customizations.md CLIamp entry for installed paths and dated backup.
Upgrades must carry these UI lifecycle/key-routing patches and all prior custom
source. Selective rollback restores the backed-up executable and affected source
files; result snapshots, music, credentials and unrelated later work are retained.
