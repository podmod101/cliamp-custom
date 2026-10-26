# Worldwide discovery with local reasoning and measured audio

Open **Alt+R**, browse your Spotify playlists and press Enter. Playback continues
while the local models prepare the playlist sound and search for streams.
Recommendations aim to find the same atmosphere and musical sound, including
unfamiliar artists and songs. Similarity is measured from audio rather than
whether a station mentions a familiar artist.

Verified matches appear as they become available. Browse with arrows and press
**Enter to play immediately**, which minimizes the search and preserves your
existing queue. The cursor stays on the same station when live rankings change.
Searching continues in the background. Press **Esc or m** to return to your
music at any time, then **Alt+L — search/results** to return directly to
the analyzed list at your last selection. Use arrows and Enter to try another
stream. Alt+L never restarts the search. It also restores saved results after
restarting CLIamp. **Alt+R starts the new playlist search workflow**, asking for
confirmation before replacing running work. Browsing new playlists
does not erase the previous results before another search completes.
Press **s** to stop searching while keeping verified results; **d** starts a
deeper pass after the current one finishes/stops. **b** returns to playlists,
with the same confirmation if work is running. A later error/timeout leaves
already verified results available. See [background navigation](background-stream-search.md)
for controls, indicators and cancellation. Genre changes are available after
the current pass finishes or is explicitly stopped.

The most recent usable result list and cursor are saved privately as
`stream-matching-results.json` in the CLIamp config directory (owner-only,
atomically written). Alt+L can recover the most recent legacy worker result on
standard installations that do not yet have this snapshot. This retains one last
list, not a full search history; previously overwritten legacy lists cannot be
reconstructed. Restored results show their original measurement date/time where
available and are marked saved: they are not silently reanalyzed or presented as
new measurements. A stream's programming or availability can change afterward.

Press **g** on results to choose a genre found in the playlist. For an electronic
blues blend, choices can include **Electronic / EDM**, **Glitch / Bass**, and
**Blues**, plus **Playlist blend**. Arrows/Enter start a focused search using the
retained playlist reference sample, without forcing you to repeat its preparation.
This searches and measures new streams; it is not just a text filter on ten rows.
The previous verified list remains under its original genre label until new
focused results arrive, so a failed search does not destroy it.

Large playlists may contain many unrelated later additions. The Spotify matcher
now loads at most six metadata pages: the first two in order and up to four
random later pages, at most 300 items. Playback's full-playlist loader is unchanged.
The reference sample balances the ordered opening with later selections; its
first batch uses eight opening and four later songs when enough are available.
The curator's description is passed to the local model and anchors genre choices.
Metadata coverage and original item count are recorded privately. This bounded
sample does not claim to represent every song in a very large mixed playlist.

## What is searched

The local OpenClaw text model reasons over sampled song/artist names, the measured
playlist audio profile, optional taste hints and available cached station history.
It generates conventional and adjacent genre searches and multilingual live
YouTube queries. Radio Browser is searched without geographic/language filters
or popularity ordering, alongside SomaFM and YouTube's live filter. Additional
broad tags and random catalog entries leave room for unusual or poorly labeled
stations. Only actual public catalog/search URLs can become candidates.

Metadata and directory queries are bounded. Radio Browser requests are serialized
and spaced, with mirror failover and six-hour query caches. An unavailable
directory may use metadata no more than three days old; candidate audio still
needs verification. YouTube search concurrency is bounded, and only explicitly
live results are eligible. Poor genre labels no longer cause hard exclusion.
Duplicate station entries/bitrate mirrors are removed before sampling.

The text model compares a promising, diverse metadata pool, using descriptions,
genres, reported countries/languages, available schedule fields and historical
audio similarity. Schedule information is often absent: the app does not invent
schedules or scrape arbitrary station websites. Soft diversity penalties reduce
one country's, language's or broadcaster's dominance. Roughly a quarter of the
shortlist is reserved for uncertain/exploration candidates and interleaved early.
This is an exploration strategy, not a guarantee of equal coverage of the world.

If the station-reasoning reply fails, a diverse metadata/history shortlist is
used with an explicit warning. Final recommendations still require measured
audio. A recent search plan can be reused if its same-input planner call fails;
there is no cloud fallback.

## Audio verification and uncertainty

Each shortlisted station initially supplies 20 seconds of audio, analyzed in
local CLAP windows. Silence/unusable audio is excluded. Separate CLAP content
prompts flag possible speech, advertisements and noise; non-music-like excerpts
are withheld, rather than treating their descriptions as evidence of music.
This classification is approximate and can confuse singing or music with speech.

When time remains, promising/variable stations and a small number of interrupted
stations receive another 30-second sample. Rechecks happen after other sampling,
with enough separation to reduce repeated buffered audio. Clean music captures
are combined with equal window weighting. An earlier speech/ad excerpt is not
averaged into later music. A later interruption is disclosed on the result.
Rechecking does not prove a station's whole schedule resembles the playlist.

Scores use only measured audio: average and lower-quartile similarity across
playlist references and captured windows. Text-model predictions, descriptions,
popularity and location never increase a measured score. The result view labels
**Audio**, **Metadata prediction**, sample duration and uncertainty separately.
Short or variable excerpts remain identified as such; cosine scores are not
percentages or confidence probabilities.

Genre eligibility is checked separately with local CLAP captions spanning the
selected genre and competing genres. EDM includes electronic subgenres such as
DnB, house, chillstep and glitch, rather than one generic synth caption. A high
general cosine does not override clearly stronger evidence of an unrelated genre.
Close genre evidence remains eligible with an uncertainty note; zero-shot genre
classification is approximate and can confuse hybrids. The app can return fewer
than ten results rather than insert off-genre replacements.

The displayed **raw cosine scores are unchanged**. A value of 0.8 is not 80%
confidence that a station fits your genre or taste. Among compatible near-matches,
selection favors varied measured audio over almost identical results, so the
list need not be in strictly descending raw-score order. No artificial score
spreading or metadata similarity boost is applied. The result view also shows
measured genre labels and excluded-genre counts.

The picker reports distinct candidates found, shortlisted candidates and stations
verified as music-like. Catalog country/language coverage uses reported metadata,
not audio language recognition. The private latest result includes individual
query return counts, capture failures, cache reuse, reserved/received audio
duration, elapsed sampling time and the reason searching stopped.

## Limits chosen from this machine's tests

An opt-in benchmark compares three and six concurrent captures and 20/30-second
samples, including CPU CLAP analysis. On the development machine, six 20-second
radio samples took about 14 seconds at concurrency six versus 25 seconds at
concurrency three; each CLAP analysis took about 0.4 seconds. Buffered broadcasters
can deliver audio faster than real time. Other stations and networks may differ.

Machine-local tuning lives in `~/.cache/cliamp/vibe/worldwide-tuning.json`.
Without calibration the app conservatively uses three capture workers. On the
tested machine it uses six workers, with at most two YouTube captures concurrently.
The measured capture/inference costs determine shortlist capacity within a
three-minute normal/five-minute deeper audio budget, capped at 64/96 candidates.
The tested normal capacity was 58, rather than a universal fixed station count.

Initial audio-duration budgets are 1,400/2,400 seconds for normal/deeper search,
including requested durations of failed captures and rechecks. These are bandwidth
proxies, not exact byte caps: bitrate, buffering and protocols vary. Rate/auth
blocks reduce concurrency and stop further affected YouTube batches. Analysis
runs on one CPU model instance, while capture workers handle network/decode IO.

After at least 24 attempted stations (36 deeper) and ten verified matches, three
diverse batches with little improvement can end the first pass. Time/audio limits
also stop it. Remaining time is reserved for rechecks. The full worker deadlines
are 15 minutes normally and 20 minutes for deeper search, including playlist
preparation and local reasoning. First useful results may take several minutes
on a cold playlist; progressive results begin during candidate verification.

To benchmark another installation explicitly, run this from the source checkout
with its isolated interpreter and model-cache environment:

```sh
HF_HUB_CACHE="$HOME/.local/share/cliamp-vibe/models" \
  "$HOME/.local/libexec/cliamp-vibe/venv/bin/python" scripts/benchmark-worldwide.py
```

This opt-in command contacts public stations and writes local tuning. It is never
run by tests, timers or startup. No background scanning is installed.

## Cache privacy and freshness

Station audio profiles expire after fourteen days for search prioritization.
Only a music-like measurement less than two minutes old may be reused as a final
measurement, and changed schedule metadata invalidates that reuse. Older cached
profiles can suggest a station but cannot supply its current score. Profiles are
bounded to 500 entries and retain the latest capture, not audio recordings.
An explicit deeper pass can reuse the verified playlist reference sample for up
to a day when the playlist's song/artist/duration content matches exactly.

Caches, queries and latest results are owner-only files under the private vibe
cache. Playlist-related data stays out of source exports. Temporary station audio
is deleted after analysis; playlist excerpts are removed when the worker exits.
Spotify audio/previews are not ingested. Models run locally through the existing
OpenClaw provider and CPU CLAP runtime.

**Best matches found worldwide** means the best current audio matches among the
stations successfully searched, shortlisted and measured in that run. It does
not mean every stream worldwide was heard or that subjective taste is guaranteed.
Directory coverage, model knowledge, short excerpts and changing programming all
limit the result. The deeper control spends more effort; it does not remove
those limits.
