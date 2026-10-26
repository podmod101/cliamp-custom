"""Orchestrate metadata reasoning, worldwide discovery and progressive audio."""
import json
import time
import sys
import random
from .common import CACHE, clean_text, private_json, progress
from .station_profiles import StationProfiles
from .world_catalog import worldwide_catalog
from .world_reasoning import search_plan, shortlist
from .world_policy import policy
from .world_sampling import verify_candidates
from .genres import GENRES, audio_genres, genre_compatible


def discover_worldwide(request, references, reference_report, profile, model, directory, hint, warnings):
    vectors = [v for _, v in references]
    settings = policy(bool(request.get("deeper")))
    profiles = StationProfiles()
    cache_summary = [{k: s.get(k) for k in ("name", "country", "language", "at")}
                     for s in sorted(profiles.entries.values(), key=lambda s: s["at"], reverse=True)[:12]]
    progress("Local AI: reasoning about playlist sound and worldwide search directions")
    all_tracks = request.get("tracks", [])
    planning_songs = [s for s, _ in references] + random.SystemRandom().sample(all_tracks, min(12, len(all_tracks)))
    hint = dict(hint, playlist_description=clean_text(request.get("description", ""), 1000),
                selected_genre=request.get("genre", ""))
    plan = search_plan(planning_songs, profile, hint, cache_summary)
    selected_genre = request.get("genre", "")
    if selected_genre and selected_genre not in GENRES:
        raise ValueError("Unknown genre filter")
    plan["genre"] = selected_genre or "blend"
    settings["allowed_genres"] = [selected_genre] if selected_genre else plan.get("genres", [])
    if plan.get("warning"):
        warnings.append(plan["warning"])
    # Explicit user calibration augments searches, never restricts all discovery.
    for key in ("radio_tags", "youtube_queries"):
        if isinstance(hint.get(key), list):
            plan[key] = list(dict.fromkeys([clean_text(x, 100) for x in hint[key]] + plan[key]))[:8 if key == "radio_tags" else 4]
    progress("Searching worldwide radio catalogs and live YouTube channels")
    candidates, search_coverage = worldwide_catalog(plan, bool(request.get("deeper")))
    warnings += search_coverage["issues"]
    if not candidates:
        raise RuntimeError("Worldwide catalogs returned no public live candidates; try again later")
    for candidate in candidates:
        historical = profiles.get(candidate)
        if historical:
            candidate["_historical_genres"] = audio_genres(historical["windows"], model)
    plan["allowed_genres"] = settings["allowed_genres"]
    progress(f"Local AI: comparing metadata/history for {len(candidates)} worldwide candidates")
    selected, reasoning = shortlist(candidates, plan, profiles, vectors, settings["candidate_limit"])
    if reasoning["warning"]:
        warnings.append(reasoning["warning"])
    result_base = {"name": clean_text(request.get("name", "Playlist")),
        "playlist_id": request.get("id", ""), "summary": plan["summary"], "profile": profile,
        "sampled": len(references), "requested": reference_report["attempted"], "reference_search": reference_report,
        "model": plan["model"], "search_tags": plan["radio_tags"], "youtube_queries": plan["youtube_queries"],
        "sample_sources": [s.get("_source") for s, _ in references],
        "score_note": "Audio-only CLAP similarity; short samples, not a global ranking or percentage",
        "genre": selected_genre, "genres": [{"id": g, "name": GENRES[g][0]} for g in plan.get("genres", [])],
        "selection_note": "Audio-compatible variety; displayed raw cosine scores are unchanged",
        "searched": len(candidates), "shortlisted": len(selected), "ai_considered": reasoning["ai_considered"],
        "ai_predictions": reasoning["ai_predictions"],
        "search_coverage": search_coverage, "policy": settings, "deeper": bool(request.get("deeper"))}

    def make_result(matches, coverage, partial):
        notes = list(warnings)
        if len(matches) < 10:
            notes.append(f"{len(matches)} current audio-verified matches available")
        if coverage["audio_rejected"]:
            notes.append("Possible speech/ad/noise excerpts excluded; content detection is approximate")
        return dict(result_base, matches=matches, compared=coverage["verified"],
                    coverage=coverage, partial=partial, warnings=notes, at=int(time.time()))

    def publish(matches, coverage):
        # Progress payloads use stderr; stdout stays one final JSON object.
        preview = make_result(matches, coverage, True)
        for key in ("sample_sources", "reference_search", "profile", "search_coverage", "policy"):
            preview.pop(key, None)
        print(json.dumps({"preview": preview}), file=sys.stderr, flush=True)

    progress(f"Measuring current audio from up to {len(selected)} diverse candidates ({settings['workers']} captures at once)")
    ranked, coverage = verify_candidates(selected, vectors, model, directory, profiles, settings, publish)
    if not ranked:
        raise RuntimeError("No stream supplied usable audio compatible with this playlist genre. Try another genre or deeper search; off-genre results were not substituted.")
    result = make_result(ranked, coverage, False)
    private_json(CACHE / "latest.json", result)
    return result
