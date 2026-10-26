"""Progressive current-audio verification and bounded, delayed rechecks."""
import concurrent.futures
from collections import Counter
import time
import threading
import re
import numpy as np
from .audio import sample_url, resolve_youtube
from .common import command_budget, progress, STOPPED
from .station_profiles import recommendation
from .world_catalog import resolve_station
from .world_policy import should_stop, update_timing, observe_progress
from .genres import GENRES, audio_genres, genre_compatible, diverse_matches

YOUTUBE_CAPTURES = threading.BoundedSemaphore(2)


def capture_candidate(item, directory, index, seconds, deadline):
    with command_budget(min(seconds + 30, max(0, deadline-time.monotonic()))):
        source = resolve_station(item)
        path = directory / f"world-{index}.wav"
        started = time.monotonic()
        if item["source"] == "YouTube Live":
            if not YOUTUBE_CAPTURES.acquire(timeout=max(0, deadline-time.monotonic())):
                raise TimeoutError("Live capture budget reached")
            try:
                source = resolve_youtube(source)
                sample_url(source, path, seconds=seconds)
            finally:
                YOUTUBE_CAPTURES.release()
        else:
            sample_url(source, path, seconds=seconds)
        return path, time.monotonic()-started, source


def combine_measurements(previous, fresh):
    if not previous:
        return dict(fresh, captures=1)
    return dict(fresh, windows=previous["windows"] + fresh["windows"],
                seconds=previous["seconds"] + fresh["seconds"],
                music_windows=previous["music_windows"] + fresh["music_windows"],
                window_count=previous["window_count"] + fresh["window_count"],
                music_margin=min(previous["music_margin"], fresh["music_margin"]),
                captures=previous.get("captures", 1) + 1)


def verify_candidates(candidates, references, model, directory, profiles, settings, publish):
    started = time.monotonic()
    deadline = started + settings["audio_budget_seconds"]
    observations, ranked, failures, rejected, attempts, history, timings = {}, [], Counter(), [], 0, [], []
    audio_seconds, captured_seconds = 0, 0
    current_workers = settings["workers"]
    stop_reason = "search in progress"
    cache_hits, index = 0, 0

    def build_ranked():
        results, vectors = [], {}
        for item, measured in observations.values():
            # A cached historical description cannot rescue speech or silence.
            if measured["music_margin"] <= 0 or measured["silent_fraction"] > .8:
                continue
            evidence = audio_genres(measured["windows"], model) if settings.get("allowed_genres") else {}
            if not genre_compatible(evidence, settings.get("allowed_genres", [])):
                continue
            reason = model.describe([np.mean(measured["windows"], axis=0)], 1)[0]["description"]
            result = recommendation(item, measured, references, reason)
            result["audio_genres"] = evidence
            if evidence:
                top = sorted(evidence, key=evidence.get, reverse=True)[:2]
                result["audio_genre_label"] = ", ".join(GENRES[g][0] for g in top)
                allowed = settings.get("allowed_genres", [])
                if allowed and max(evidence.get(g,-1) for g in allowed) < max(evidence.values()):
                    result["uncertainty"] += "; genre evidence overlaps another style"
            results.append(result)
            vector = np.mean(measured["windows"], axis=0)
            vectors[item["id"]] = vector/max(1e-8, np.linalg.norm(vector))
        return diverse_matches(results, vectors)

    def coverage():
        incompatible = sum(not genre_compatible(audio_genres(m["windows"], model), settings["allowed_genres"])
                           for _, m in observations.values() if m["music_margin"] > 0) if settings.get("allowed_genres") else 0
        return {"attempted": attempts, "verified": sum(m["music_margin"] > 0 and m["silent_fraction"] <= .8 for _, m in observations.values()),
            "audio_rejected": len(rejected), "genre_verified": len(ranked),
            "genre_rejected": incompatible,
            "failures": dict(failures), "cache_reused": cache_hits,
            "audio_seconds_reserved": audio_seconds, "captured_audio_seconds": round(captured_seconds),
            "elapsed_seconds": round(time.monotonic()-started),
            "countries_measured": len({i.get("countrycode") or i.get("country") for i, _ in observations.values() if i.get("countrycode") or i.get("country")}),
            "languages_measured": len({language.strip().casefold() for i, _ in observations.values()
                for language in re.split(r"[,;]", i.get("language", "")) if language.strip()}),
            "workers": current_workers, "stop_reason": stop_reason, "timing": update_timing(timings)}

    def process_batch(batch, seconds, recheck=False):
        nonlocal attempts, audio_seconds, captured_seconds, cache_hits, index, ranked, current_workers
        work = []
        for item in batch:
            if not recheck:
                cached = profiles.fresh(item)
                if cached:
                    attempts += 1
                    observations[item["id"]] = (item, dict(cached, windows=[np.asarray(v) for v in cached["windows"]]))
                    cache_hits += 1
                    continue
            # Reserve the full requested audio duration, even if capture fails.
            if audio_seconds + seconds > settings["audio_seconds_limit"]:
                break
            audio_seconds += seconds
            if not recheck:
                attempts += 1
            index += 1
            work.append((item, index))
        with concurrent.futures.ThreadPoolExecutor(max_workers=current_workers) as pool:
            futures = {pool.submit(capture_candidate, item, directory, n, seconds, deadline): item for item, n in work}
            for future in concurrent.futures.as_completed(futures):
                item = futures[future]
                path = None
                try:
                    path, capture_time, source = future.result()
                    analysis = model.analyze(path)
                    captured_seconds += analysis["seconds"]
                    timings.append({"capture_seconds": capture_time})
                    analysis["at"] = int(time.time())
                    analysis["_observed_monotonic"] = time.monotonic()
                    prior = observations.get(item["id"], (None, None))[1] if recheck else None
                    # A speech/advertisement-first capture is not averaged into a
                    # later clean music excerpt, and is never cherry-picked as music.
                    if prior and prior["music_margin"] <= 0:
                        prior = None
                    if analysis["music_margin"] <= 0 or analysis["silent_fraction"] > .8:
                        rejected.append(item["id"])
                        if not recheck:
                            observations[item["id"]] = (item, dict(analysis, captures=1))
                        elif prior:
                            prior["recheck_interrupted"] = True
                    else:
                        combined = combine_measurements(prior, analysis)
                        observations[item["id"]] = (item, combined)
                        # Bound persisted windows to this latest capture, so old
                        # programming never masquerades as fresh combined evidence.
                        profiles.save(item, analysis)
                except Exception as exc:
                    kind = getattr(exc, "kind", "capture_or_audio_failed")
                    failures[kind] += 1
                    timings.append({"failed": True})
                finally:
                    if path:
                        path.unlink(missing_ok=True)
                ranked = build_ranked()
                if ranked:
                    publish(ranked, coverage())
                progress(f"Audio verified: {coverage()['verified']} stations; {attempts}/{len(candidates)} first samples; {len(rejected)} speech/noise excerpts")
        # Back off after actual throttling; do not refill with more requests.
        if failures["rate_limited"] or failures["youtube_auth_required"] >= 2:
            current_workers = max(2, current_workers // 2)
        ranked = build_ranked()
        if ranked:
            publish(ranked, coverage())

    for offset in range(0, len(candidates), settings["workers"]):
        if STOPPED.is_set():
            break
        reserve = settings["confirm_seconds"] + 10 if ranked else settings["sample_seconds"]
        if time.monotonic() >= deadline - reserve:
            stop_reason = "audio time budget reached"
            break
        if audio_seconds >= settings["audio_seconds_limit"]:
            stop_reason = "audio bandwidth budget reached"
            break
        batch = candidates[offset:offset+settings["workers"]]
        # Avoid repeatedly hammering a shared service after an observed block.
        if failures["youtube_auth_required"] >= 2 or failures["rate_limited"]:
            batch = [i for i in batch if i["source"] != "YouTube Live"]
        process_batch(batch, settings["sample_seconds"])
        observe_progress(history, [i["score"] for i in ranked])
        if should_stop(history, len(ranked), attempts, settings):
            stop_reason = "little improvement over three diverse batches"
            break
    else:
        stop_reason = "candidate shortlist exhausted"
    # Spend remaining effort on top/borderline candidates and promising speech
    # interruptions. Other captures created a natural delay before this pass.
    priority = [c for c in candidates if c["id"] in {r["id"] for r in ranked[:4]}]
    priority += [c for c in candidates if c["id"] in rejected and c not in priority][:2]
    for c in ranked[4:]:
        item = next(i for i in candidates if i["id"] == c["id"])
        if c["temporal_spread"] > .04 and item not in priority:
            priority.append(item)
    remaining = deadline - time.monotonic()
    priority = [c for c in priority if time.monotonic() - observations[c["id"]][1].get(
        "_observed_monotonic", started-settings["sample_seconds"]) >= settings["sample_seconds"]]
    if priority and remaining > settings["confirm_seconds"] + 10 and audio_seconds < settings["audio_seconds_limit"]:
        progress("Rechecking promising/variable stations and possible presenter interruptions")
        process_batch(priority[:settings["confirm_limit"]], settings["confirm_seconds"], recheck=True)
    return ranked, coverage()
