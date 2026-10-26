# Return to analyzed results — 2026-10-25

Fixed Alt+R discarding analyzed results after selecting a stream. It now reopens
the existing in-memory result list with the same selection/title/playlist, ends
any outstanding old work and issues no provider/AI/audio request. Another result
plays normally and appends without replacing the queue. Escape preserves results
for reopening; b explicitly returns to playlists. Result memory is scoped to the
current process; restart persistence is not claimed.

Validation: full make check (format, vet, all Go tests). New regression cases
exercise completed/progressive results, playback then reopen, last selection,
another playback selection, queue preservation, Escape and explicit playlist
return. No network/model calls or new dependencies. Existing Python pipeline and
measurements unchanged.

Updated worldwide/keybinding guides, source website, LOCAL-CHANGES and version
references. Both local source checkouts and installed custom executable updated
with backup; central customization log records current version, validation and
rollback. Existing customizations, accounts/config, caches and music preserved.
No commit/push or website publication. Violin Hub student/teacher guides unaffected.
