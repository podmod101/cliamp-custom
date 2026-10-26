"""Bounded replacement sampling and feedback-guided local AI searches."""
from collections import Counter
import time
from .common import CACHE, private_json, progress
from .local_model import plan_song_queries


def collect_references(songs, prepare, model, directory, collector, budget=360):
    started = time.monotonic()
    target, minimum = min(12, len(songs)), min(5, len(songs))
    references, attempted, failures, models = [], [], [], set()
    retried, offset = False, 0

    def report():
        return {"attempted": len(attempted), "usable": len(references),
                "failed": len(attempted) - len(references), "target": target,
                "failure_counts": dict(Counter(s.get("_failure_kind", "unknown") for s in attempted
                    if s not in [item for item, _ in references])),
                "local_models": sorted(models), "feedback_retry": retried}

    def persist():
        private_json(CACHE / "latest-reference-search.json", dict(report(), songs=[{
            "title": s["title"], "artist": s["artist"], "queries": s.get("_queries", []),
            "failure": s.get("_failure_kind"), "search_results": s.get("_lookup_feedback", []),
            "source": s.get("_source")} for s in attempted]))

    while len(references) < target and time.monotonic() - started < budget:
        batch = songs[offset:offset + (12 if offset == 0 else 6)]
        offset += len(batch)
        # One bounded feedback pass, alongside fresh random replacement songs.
        retry = []
        if offset > 12 and not retried:
            retry = [s for s in failures if s.get("_failure_kind") == "no_confident_match"][:4]
        if not batch and not retry:
            # Small playlists also get a chance to repair their unsuccessful queries.
            if not retried:
                retry = [s for s in failures if s.get("_failure_kind") == "no_confident_match"][:4]
            if not retry:
                break
        work = batch + retry
        progress(f"Local AI: planning alternate searches for {len(work)} songs" +
                 (" using previous YouTube results" if retry else ""))
        feedback = [{"id": str(i), "searches": s.get("_lookup_feedback", [])}
                    for i, s in enumerate(work) if s in retry]
        remaining = budget - (time.monotonic() - started)
        if remaining < 15:
            break
        try:
            plans, name = plan_song_queries(work, feedback, timeout=min(90, int(remaining)))
        except Exception:
            persist()
            raise RuntimeError("Local AI song-search planning failed. Check the local OpenClaw model and retry; no cloud fallback was used.") from None
        models.add(name)
        for song, queries in zip(work, plans):
            song["_queries"] = queries
            song["_deadline"] = started + budget
            song.pop("_failure_kind", None)
        if retry:
            retried = True
        attempted.extend(batch)
        progress(f"Finding independently verified audio; {len(references)}/{target} songs usable")
        found, _ = collector(work, prepare, model, directory, "Playlist audio")
        references.extend(found)
        failures = [s for s in attempted if s.get("_failure_kind")]
        persist()
        # A shared service block is not repaired by sending more searches.
        blocked = [s for s in work if s.get("_failure_kind") in ("youtube_auth_required", "rate_limited")]
        if len(blocked) >= 3 and len(blocked) >= len(work) / 2:
            break
    summary = report()
    persist()
    if len(references) < minimum:
        reasons = ", ".join(f"{kind.replace('_', ' ')}: {count}" for kind, count in summary["failure_counts"].items())
        raise RuntimeError(f"Only {len(references)}/{len(attempted)} song samples worked; need {minimum}. "
                           f"{reasons or 'Reference search time limit reached'}. "
                           "Search details saved in the private vibe cache. No metadata-only ranking was substituted.")
    return references, summary
