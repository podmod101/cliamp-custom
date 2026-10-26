# New searches and direct saved-result return — 2026-10-26

Josh clarified the controls: Alt+R means a new playlist search; returning to an
already analyzed list must be a separate action with no playlist/model/capture
work. Read-only process inspection confirmed the running CLIamp was the older
worldwide executable, despite newer builds already being installed on disk.
The prior session-only Alt+R reopening behavior did not meet this workflow.

Alt+R now always opens the new-playlist workflow. Alt+L opens the last analyzed
result list directly, preserving highlighted station, ordering and raw scores.
Selecting another saved result uses normal playback and preserves the queue.
Playlist browsing does not discard the last result snapshot. Alt+L can interrupt
new playlist loading and return to the prior list without executing its request.

Results are saved atomically and owner-only as stream-matching-results.json in
the active CLIamp config directory, after usable completion and when closing/
playing results. They survive restart. The standard config can recover the most
recent legacy worker latest.json if no app snapshot exists; this is the most
recent available output, not a full history or recovery of overwritten lists.
Explicit separate profiles do not fall back to another profile's worker cache.
Saved measurements are dated and labeled original, not silently refreshed.

Alt+L does not construct a Spotify provider or return a Bubbletea work command.
Provider initialization is deferred until an explicit new/focused/deeper search
or playlist navigation. Missing/invalid/oversized snapshots show a status message
and never trigger a search. Only bounded, validated public-stream result targets
are restored. No audio models, dependencies, account/config credentials, caches,
music or background jobs changed. The running process was not interrupted.

Validation: full make check/build; regressions cover separate Alt+R/Alt+L behavior,
same-session and post-restart restoration, saved selection, normal second playback,
queue preservation, pending-work cancellation, private file permissions, corrupt/
unsafe/oversized input rejection and legacy migration without changing timestamps.
Existing genre/discovery/playback tests also pass. Tests use local fixtures only;
no live search/model run or replacement of the user's cached results was needed.

Updated both source copies, custom executable, key registry/help, guides/site,
LOCAL-CHANGES and installer version. Base remains upstream v2.0.1, with prior
custom features preserved. New version:
v2.0.1+search-history.youtube-cleanup.playlist-streams.saved-results.
Machine backup, verification and selective rollback are in the central log.
No git/commit/push or website publication. Violin Hub guides unaffected.
