"""Measured machine policy and bounded adaptive search decisions."""
import json
import statistics
from .common import CACHE


def policy(deeper=False):
    try:
        measured = json.loads((CACHE / "worldwide-tuning.json").read_text())
    except (OSError, ValueError):
        measured = {}
    workers = max(2, min(6, int(measured.get("capture_workers", 3))))
    sample = max(20, min(30, int(measured.get("sample_seconds", 20))))
    # Capture is usually the bottleneck; leave time for failures and rechecks.
    cost = max(sample / 2, float(measured.get("capture_p75_seconds", sample)))
    infer = max(.1, float(measured.get("inference_seconds", 1)))
    budget = 300 if deeper else 180
    capacity = int(budget / (cost + infer) * workers * .7)
    return {"workers": workers, "sample_seconds": sample, "confirm_seconds": 30,
            "candidate_limit": max(24, min(96 if deeper else 64, capacity)),
            "audio_budget_seconds": budget, "audio_seconds_limit": 2400 if deeper else 1400,
            "minimum_attempts": 36 if deeper else 24, "confirm_limit": 10 if deeper else 6,
            "calibrated": bool(measured), "capture_cost": cost}


def should_stop(history, verified, attempted, policy_value):
    # Require exploration and ten playable results before a diminishing-return stop.
    if verified < 10 or attempted < policy_value["minimum_attempts"] or len(history) < 4:
        return False
    return max(history[-3:]) - history[-4] < .003


def observe_progress(history, scores):
    # Filling a list from six to ten matches naturally lowers its mean. Compare
    # complete lists only, rather than misreading that as diminishing returns.
    if len(scores) < 10:
        history.clear()
    else:
        history.append(sum(scores) / len(scores))


def update_timing(samples):
    """Summarize observed capture costs for the private coverage report."""
    good = [s for s in samples if s.get("capture_seconds") and not s.get("failed")]
    if not good:
        return
    return {"capture_median_seconds": statistics.median(s["capture_seconds"] for s in good),
            "captures": len(samples), "failed": sum(bool(s.get("failed")) for s in samples)}
