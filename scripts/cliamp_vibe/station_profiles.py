"""Audio-only station history. Old profiles guide selection, never current scores."""
import hashlib
import json
import time
import numpy as np
from .audio import MODEL_ID, normalize
from .common import CACHE, private_json


def station_key(item):
    return hashlib.sha256(item["url"].encode()).hexdigest()


class StationProfiles:
    def __init__(self, path=None):
        self.path = path or CACHE / "station-profiles.json"
        try:
            data = json.loads(self.path.read_text())
            self.entries = data["entries"] if data.get("model") == MODEL_ID and isinstance(data.get("entries"), dict) else {}
        except (OSError, ValueError, KeyError):
            self.entries = {}
        now = time.time()
        self.entries = {k: v for k, v in self.entries.items() if isinstance(v, dict) and
                        isinstance(v.get("at"), (int, float)) and now - v["at"] < 14 * 86400}

    def get(self, item):
        value = self.entries.get(station_key(item))
        if not value:
            return None
        try:
            windows = np.asarray(value["windows"], dtype=np.float32)
            if windows.ndim != 2 or windows.shape[0] == 0 or windows.shape[1] != 512 or not np.isfinite(windows).all() or np.any(np.linalg.norm(windows, axis=1) < 1e-8):
                return None
            return value
        except (ValueError, KeyError):
            return None

    def prior(self, item, references):
        value = self.get(item)
        if not value:
            return None
        return float((np.asarray(references) @ normalize(np.mean(value["windows"], axis=0))).mean())

    def fresh(self, item, now=None):
        value = self.get(item)
        # Short replay window only. Older programming is a search hint, never a
        # present-day measurement. A scheduled show boundary invalidates reuse.
        if not value or (now or time.time()) - value["at"] > 120:
            return None
        if value.get("schedule", "") != item.get("schedule", ""):
            return None
        if value.get("seconds", 0) < 18 or value.get("music_margin", -1) <= 0:
            return None
        return dict(value, cached=True)

    def save(self, item, measured):
        record = {k: measured[k] for k in ("seconds", "music_margin", "music_windows", "window_count", "content", "silent_fraction")}
        record.update(at=int(time.time()), windows=[np.asarray(v).tolist() for v in measured["windows"]],
                      schedule=item.get("schedule", ""), name=item["name"],
                      country=item.get("country", ""), language=item.get("language", ""))
        self.entries[station_key(item)] = record
        self.entries = dict(sorted(self.entries.items(), key=lambda pair: pair[1]["at"], reverse=True)[:500])
        private_json(self.path, {"model": MODEL_ID, "entries": self.entries})
        return record


def measured_score(references, windows):
    """Metadata-free similarity, equal weight per reference and audio window."""
    similarities = np.asarray(references) @ np.asarray(windows).T
    per_reference = similarities.mean(axis=1)
    score = float(.75 * per_reference.mean() + .25 * np.quantile(per_reference, .25))
    temporal = float(np.ptp(similarities.mean(axis=0))) if similarities.shape[1] > 1 else 0
    return score, temporal, per_reference


def recommendation(item, measured, references, reason):
    score, temporal, per_reference = measured_score(references, measured["windows"])
    seconds = measured["seconds"]
    uncertainty = "short excerpt"
    if temporal > .06:
        uncertainty = "variable excerpt"
    elif measured.get("captures", 1) > 1:
        uncertainty = "rechecked; station programming may change"
    if measured["music_windows"] < measured["window_count"]:
        uncertainty += "; mixed music/speech evidence"
    if measured.get("cached"):
        uncertainty += "; measured within 2 minutes"
    if measured.get("recheck_interrupted"):
        uncertainty += "; later sample may contain speech/ad"
    # Keep predictions separate; no score boost for AI wording, popularity,
    # country, cached historical similarity or station identity.
    return {k: v for k, v in dict(item, score=round(score, 4), reason=reason,
        prediction=item.get("prediction", "Uncertain metadata"), uncertainty=uncertainty,
        sample_seconds=round(seconds), captures=measured.get("captures", 1),
        measured_at=measured["at"], reference_min=round(float(per_reference.min()), 4),
        reference_max=round(float(per_reference.max()), 4), temporal_spread=round(temporal, 4)).items()
        if not k.startswith("_")}
