#!/usr/bin/env python3
"""Opt-in live capture benchmark; writes machine-local tuning, never runs in tests."""
import concurrent.futures
import json
from pathlib import Path
import statistics
import tempfile
import time
from cliamp_vibe.audio import AudioModel, sample_url
from cliamp_vibe.common import CACHE, private_json
from cliamp_vibe.world_catalog import soma_catalog, resolve_station


def calibrate(report):
    runs = [r for r in report["runs"] if len([s for s in r["measurements"] if not s.get("failed")]) >= 4]
    if not runs:
        raise RuntimeError("Not enough successful captures to calibrate safely")
    # Select the most efficient successful 20s run; confirm promising stations
    # using 30s rather than paying that extra cost for every candidate.
    choices = [r for r in runs if r["sample_seconds"] == 20] or runs
    best = min(choices, key=lambda r: (sum(bool(s.get("failed")) for s in r["measurements"]),
               r["wall_seconds"] / sum(not s.get("failed") for s in r["measurements"])))
    good = [s for s in best["measurements"] if not s.get("failed")]
    return {"capture_workers": best["concurrency"], "sample_seconds": best["sample_seconds"],
            "capture_p75_seconds": float(__import__("numpy").quantile([s["capture_seconds"] for s in good], .75)),
            "inference_seconds": statistics.median(s["inference_seconds"] for s in good),
            "at": int(time.time()), "benchmark_stations": len(good)}


def main():
    model = AudioModel()
    stations = soma_catalog()[:6]
    report = {"runs": []}
    with tempfile.TemporaryDirectory(prefix="cliamp-benchmark-") as folder:
        for workers, seconds in ((3, 20), (6, 20), (6, 30)):
            started = time.monotonic()
            def capture(pair):
                n, item = pair
                path = Path(folder) / f"{n}.wav"
                begin = time.monotonic()
                try:
                    sample_url(resolve_station(item), path, seconds=seconds)
                    return path, time.monotonic()-begin
                except Exception:
                    return None, time.monotonic()-begin
            measurements = []
            with concurrent.futures.ThreadPoolExecutor(max_workers=workers) as pool:
                for path, elapsed in pool.map(capture, enumerate(stations)):
                    if path is None:
                        measurements.append({"failed": True, "capture_seconds": elapsed})
                        continue
                    begin = time.monotonic()
                    model.analyze(path)
                    measurements.append({"capture_seconds": elapsed, "inference_seconds": time.monotonic()-begin})
            run = {"concurrency": workers, "sample_seconds": seconds, "wall_seconds": time.monotonic()-started,
                   "measurements": measurements}
            report["runs"].append(run)
            print(json.dumps(run), flush=True)
    settings = calibrate(report)
    private_json(CACHE / "worldwide-tuning.json", settings)
    print(json.dumps({"tuning": settings, "path": str(CACHE / "worldwide-tuning.json")}), flush=True)


if __name__ == "__main__":
    main()
