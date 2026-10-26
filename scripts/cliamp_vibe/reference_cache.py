"""Reuse the same verified playlist sample for an explicit deeper pass."""
import hashlib
import json
import time
import numpy as np
from .audio import MODEL_ID
from .common import CACHE, private_json
from .song_lookup import song_match


def cache_path(request):
    identity = json.dumps(sorted((s.get("title", ""), s.get("artist", ""), s.get("duration", 0))
                                 for s in request.get("tracks", [])), ensure_ascii=False)
    return CACHE / "playlist-references" / (hashlib.sha256((MODEL_ID+identity+str(request.get("description", ""))).encode()).hexdigest()+".json")


def load_references(request):
    if not request.get("deeper") and not request.get("genre"):
        return None
    try:
        data = json.loads(cache_path(request).read_text())
        if time.time() - data["at"] > 86400:
            return None
        references = []
        for entry in data["references"]:
            vector = np.asarray(entry["vector"], dtype=np.float32)
            song = entry["song"]
            if vector.shape != (512,) or not np.isfinite(vector).all() or np.linalg.norm(vector) < 1e-8 or not song_match(song, song["_source"]):
                return None
            references.append((song, vector))
        unique = {(s.get("title"), s.get("artist")) for s in request.get("tracks", []) if s.get("title") and s.get("artist")}
        if len(references) < min(5, len(unique)) or not references:
            return None
        return references, data["report"]
    except (OSError, ValueError, KeyError):
        return None


def save_references(request, references, report):
    path = cache_path(request)
    private_json(path, {"at": int(time.time()), "report": report, "references": [{
        "song": {k: s[k] for k in ("title", "artist", "duration", "_source") if k in s},
        "vector": v.tolist()} for s, v in references]})
    for old in sorted(path.parent.glob("*.json"), key=lambda p: p.stat().st_mtime)[:-20]:
        old.unlink(missing_ok=True)
