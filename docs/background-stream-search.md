# Search for streams while listening

Start with **Alt+R**, pick a Spotify playlist and press Enter. Once it is
searching, **Esc or m** returns to the screen you were using. Browse music,
manage your queue and use your normal playback controls. The search keeps
running with the same local models, progress, deadline and results.

| Control | Action |
|---|---|
| **Esc / m**, on search screen | Minimize; keep searching and retain results |
| **Alt+L**, from player or browser | Reopen current progress/results, at the same selection |
| **↑↓ / Enter**, on results | Browse/play a stream; minimizes and keeps searching |
| **s**, on search screen | Cancel the search; retain verified matches |
| **Alt+R** | Open a new playlist search; running work needs **y** confirmation |
| **n / Esc**, on replacement question | Keep the current search |
| **b**, on results | Return to playlists, with the same replacement confirmation |
| **g / d**, after search ends | Choose playlist genre / search deeper |

The status line shows **AI searching (count) · Alt+L**, then **AI results ready**.
An error shows **AI needs attention**; Alt+L opens its details. Background progress,
results and errors never open a screen, grab keyboard input, start a stream or
change what is playing. Choosing Enter on a result is what starts playback.
While the search is hidden, **s** retains its normal player action: stop music.
To cancel AI work, reopen it with Alt+L, then press s.

Escape first leaves playlist filter editing or genre selection; another Escape
minimizes. The letter m can still be typed into the playlist filter. Reopening
over a queue/browser screen leaves that screen and its cursor intact underneath.
Changing genres needs the current pass to finish or be cancelled first.

Completed results, and usable partial results saved when minimizing, playing or
canceling, can be restored with Alt+L after restarting CLIamp. Saved audio
measurements retain their original age; they are not silently refreshed. Active
searches run only while CLIamp is open: quitting cancels work, rather than leaving
a detached model process. This feature does not install scheduled searches.

For discovery coverage, audio matching and setup, see
[worldwide streams](worldwide-streams.md) and [playlist matching](playlist-streams.md).
