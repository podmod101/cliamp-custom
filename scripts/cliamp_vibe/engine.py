"""Playlist sampling, stream capture, and measured audio similarity ranking."""
import concurrent.futures
import random
import tempfile
from pathlib import Path
import time
import numpy as np
from .audio import AudioModel, cached_embedding, cached_source, save_embedding, sample_url, resolve_youtube
from .common import progress, clean_text, command_budget
from .song_lookup import song_videos, song_match
from .references import collect_references
from .preferences import playlist_hint
from .reference_cache import load_references, save_references


def sample_songs(tracks, count=12, rng=None):
    unique = {}
    for track in tracks:
        title, artist = clean_text(track.get("title", "")), clean_text(track.get("artist", ""))
        if title and artist:
            unique.setdefault((title.casefold(), artist.casefold()), {
                "title": title, "artist": artist, "duration": track.get("duration", 0)})
    values = list(unique.values())
    return (rng or random.SystemRandom()).sample(values, min(count, len(values)))


def prepare_song(song, directory, index):
    key = "song:" + song["artist"].casefold() + ":" + song["title"].casefold()
    cached = cached_embedding(key, 30 * 86400)
    provenance = cached_source(key)
    if cached is not None and provenance and song_match(song, provenance):
        song["_source"] = provenance
        return key, cached, None
    allowance = min(65, max(0, song.get("_deadline", time.monotonic() + 65) - time.monotonic()))
    with command_budget(allowance):
        matches = song_videos(song)
        for match in matches:
            try:
                source = resolve_youtube(match["url"])
                path = directory / f"song-{index}.wav"
                sample_url(source, path, start=min(45, max(0, (match["duration"] - 30) // 2)))
                song["_source"] = match
                return key, None, path
            except Exception as exc:
                last_error = exc
        raise last_error


def prepare_stream(candidate, directory, index):
    key = "stream:" + candidate["url"]
    # Short-lived cache only: a live station changes music throughout the day.
    cached = cached_embedding(key, 1800)
    if cached is not None:
        return key, cached, None
    source = candidate["url"]
    if candidate["source"] == "YouTube Live":
        source = resolve_youtube(source)
    path = directory / f"stream-{index}.wav"
    sample_url(source, path)
    return key, None, path


def collect_embeddings(items, prepare, model, directory, label):
    successes, failures = [], 0
    with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
        futures = {pool.submit(prepare, item, directory, i): item for i, item in enumerate(items)}
        for completed, future in enumerate(concurrent.futures.as_completed(futures), 1):
            item = futures[future]
            try:
                key, embedding, path = future.result()
                if embedding is None:
                    embedding = model.embed(path)
                    save_embedding(key, embedding, item.get("_source"))
                successes.append((item, embedding))
            except Exception as exc:
                failures += 1
                item["_failure_kind"] = getattr(exc, "kind", "audio_processing_failed")
                # Progress contains only a bounded description, no credentials/URLs.
                progress(f"{label}: skipped a sample ({clean_text(exc, 90)})")
            finally:
                progress(f"{label}: {completed}/{len(items)} checked; {len(successes)} usable")
    return successes, failures


def rank_streams(references, candidates):
    """Average + lower-quartile similarity rewards coverage of a mixed playlist."""
    refs = np.asarray(references)
    ranked = []
    for item, vector in candidates:
        similarities = refs @ vector
        score = float(0.75 * similarities.mean() + 0.25 * np.quantile(similarities, .25))
        ranked.append(dict(item, score=round(score, 4)))
    return sorted(ranked, key=lambda entry: entry["score"], reverse=True)[:10]


def reference_songs(request, rng=None):
    tracks = request.get("tracks", [])
    if len(tracks) > 100:
        # Preserve curator intent from the ordered opening while still exploring
        # across the sampled later pages. Avoid averaging 11,000 mixed additions.
        head = sample_songs(tracks[:100], count=24, rng=rng)
        tail = sample_songs(tracks[100:], count=12, rng=rng)
        songs = head[:8] + tail[:4] + head[8:] + tail[4:]
    else:
        songs = sample_songs(tracks, count=36, rng=rng)
    return list({(s["title"].casefold(), s["artist"].casefold()): s for s in songs}.values())


def match(request):
    songs = reference_songs(request)
    if not songs:
        raise RuntimeError("Playlist has no songs with usable artist and title metadata")
    progress("Loading the local CLAP audio model")
    model = AudioModel()
    warnings = []
    with tempfile.TemporaryDirectory(prefix="cliamp-vibe-") as temp:
        directory = Path(temp)
        cached_references = load_references(request)
        if cached_references:
            progress("Reusing the verified playlist audio profile for deeper discovery")
            references, reference_report = cached_references
        else:
            references, reference_report = collect_references(
                songs, prepare_song, model, directory, collect_embeddings)
            save_references(request, references, reference_report)
        failed = reference_report["failed"]
        reference_report["metadata_sampled"] = len(request.get("tracks", []))
        reference_report["playlist_total"] = request.get("playlist_total") or len(request.get("tracks", []))
        if failed:
            warnings.append(f"{failed} songs had no usable audio after alternate searches/replacement sampling")
        vectors = [vector for _, vector in references]
        profile = model.describe(vectors)
        from .world_engine import discover_worldwide
        return discover_worldwide(request, references, reference_report, profile, model, directory,
                                  playlist_hint(request), warnings)
