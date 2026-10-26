# Playlist genre fidelity — 2026-10-25

The reported high-cosine country-heavy search used mostly pop/rock reference
recordings despite a curator-described electronic/blues direction. The public
playlist contained roughly 11,000 items; uniform random whole-list sampling could
miss its coherent opening. Curator descriptions were not reaching local reasoning,
and the original CLAP descriptor list omitted blues and treated broad cosine as
sufficient for final ranking. A general 0.8 cosine is not 80% genre confidence.

Implemented bounded matching-only Spotify metadata retrieval: first two ordered
pages and up to four random later pages, at most 300 items/six requests, cached by
playlist and invalidated on snapshot change/refresh. Passed curator descriptions,
selected genre and original item count to the worker. Playback's full-list loader
is unchanged. The first reference batch balances eight opening with four later
songs when available. References are deduplicated and metadata coverage is recorded.

Local reasoning derives playlist genre choices anchored by explicit description.
g opens a genre picker; arrows/Enter focus the search (for example Electronic/EDM,
Glitch/Bass or Blues). Existing reference embeddings can be reused; prior verified
results retain their own label while new ones are prepared. Failure preserves the
prior list. Alt+R reopening, playback/queue, stop/deeper/cancellation and all earlier
customizations remain.

Local CLAP now embeds a separate family of genre captions. Broad EDM includes
DnB, house, techno, chillstep, psychedelic bass and glitch captions; each family
uses its strongest measured caption instead of requiring every subgenre to match.
Eligibility compares requested versus competing measured genres. Close evidence
within .025 cosine remains eligible, disclosed as overlap. This is an approximate
zero-shot classifier, not a genre guarantee. Historical profile contradictions
reduce shortlist priority but never supply current scores; uncertain candidates
remain eligible for exploration. Current audio determines final eligibility.

Raw playlist similarity is unchanged. Selection among compatible near-matches
prefers distinct measured audio, keeping every displayed raw score intact. No
artificial score spread or metadata score boost. UI labels raw cosine, measured
genre, selected filter, uncertainty and genre exclusions. Fewer than ten can be
returned instead of inserting mismatched styles.

Validation: 39 offline Python tests, including curator intent despite noisy model
tags, specific-subgenre EDM coverage, high-cosine country exclusion inside the
actual sampling path, unchanged scores under variety selection, stratified
reference sampling, and all previous suites. Go tests cover six-request sampling
of an 11,000-item fixture, ordered opening, no duplicate pages, cached isolation,
description preservation/refresh, worker input propagation, genre selection/
cancellation, and prior-result retention. Complete make check passed.

Measured calibration used the actual cached problematic run: all ten old raw
.746-.814 results failed the EDM gate, while five cached electronic/DnB examples
remained eligible after expanding electronic captions and choosing .025 overlap
tolerance. These are small observed checks, not broad classifier accuracy claims.

Separate live local-Qwen/CLAP run used eight independently matched recordings
from the playlist's opening, retrieved 348 distinct station candidates and
shortlisted 58. It compared 30 first captures plus five rechecks and ultimately
returned nine EDM-compatible results; rechecking removed one candidate. One or
more possible speech/ad excerpts were withheld. Audio work took about 130s;
reference capture/model work plus reasoning/discovery/audio took about 211s after
query planning. Radio and YouTube were represented. Raw returned scores ranged
about .461-.636 with varied electronic/house/trance/bass-oriented candidates.
No claim of perfect subjective fit: genre captions can misclassify hybrids and
station labels can disagree with the sampled programming. Latest follow-up UI/
coverage refinements and historical-priority handling were checked offline;
the entire live run was not repeated after those small changes.

Build: v2.0.1+search-history.youtube-cleanup.playlist-streams.genres, same upstream
v2.0.1 base. Both local source copies, isolated helper and installed binary updated
with backups/log entry; no dependencies, credentials/config changes, git/commit/
push, website publication or background tasks. Guides/site/version references
updated; Violin Hub student/teacher manuals unaffected. Exact backup and selective
rollback are in the central application customization log.

Final offline refinement: diminishing-return checks compare complete ten-item
lists only; a growing or shrinking list resets history so filling the shortlist
cannot be mistaken for a stalled search. The added regression passed.
