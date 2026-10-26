"""Worldwide catalogs, bounded metadata caches, and gentle directory requests."""
import concurrent.futures
import hashlib
import json
import random
import re
import threading
import time
import urllib.error
import urllib.parse
from .common import CACHE, clean_text, private_json, public_url, request_json, request_text, progress
from .discovery import radio_servers, youtube_search


class RadioDirectory:
    def __init__(self):
        self.server = None
        self.disabled = False
        self.lock = threading.Lock()
        self.next_request = 0
        self.requests = 0

    def search(self, field, term, limit=60):
        params = {"hidebroken": "true", "order": "random", "limit": limit}
        if field and term:
            params[field] = term
        key = hashlib.sha256(json.dumps(params, sort_keys=True).encode()).hexdigest()
        path = CACHE / "catalog" / (key + ".json")
        cached = None
        try:
            cached = json.loads(path.read_text())
            if time.time() - cached["at"] < 6 * 3600:
                return cached["items"], "cached metadata"
        except (OSError, ValueError, KeyError):
            pass
        with self.lock:
            if self.disabled:
                raise RuntimeError("Radio directory cooling down")
            # One directory request at a time; no popularity sort or region filter.
            time.sleep(max(0, self.next_request - time.monotonic()))
            servers = [self.server] if self.server else radio_servers()[:2] + ["stations.radioss.app"]
            for server in servers:
                try:
                    self.requests += 1
                    raw = request_json("https://" + server + "/json/stations/search?" +
                                       urllib.parse.urlencode(params), timeout=5)
                    if not isinstance(raw, list):
                        raise ValueError("Invalid directory response")
                    entries = [radio_item(s) for s in raw if isinstance(s, dict)]
                    entries = [s for s in entries if s["name"] and public_url(s["url"])]
                    self.server = server
                    private_json(path, {"at": int(time.time()), "items": entries})
                    self.next_request = time.monotonic() + .4
                    # Bound cache growth without changing cached query freshness.
                    for old in sorted(path.parent.glob("*.json"), key=lambda p: p.stat().st_mtime)[:-96]:
                        old.unlink(missing_ok=True)
                    return entries, "live metadata"
                except urllib.error.HTTPError as exc:
                    if exc.code == 429:
                        self.disabled = True
                        break
                except Exception:
                    continue
            self.server = None
        if cached and time.time() - cached.get("at", 0) < 3 * 86400:
            return cached["items"], "stale metadata; audio will be checked"
        raise RuntimeError("Radio directory unavailable or rate limited")


def radio_item(s):
    return {"name": clean_text(s.get("name", "")), "url": s.get("url_resolved") or s.get("url", ""),
        "source": "Radio", "tags": clean_text(s.get("tags", ""), 400),
        "description": clean_text(s.get("description", ""), 400),
        "country": clean_text(s.get("country", "")), "language": clean_text(s.get("language", "")),
        "countrycode": clean_text(s.get("countrycode", "")), "homepage": s.get("homepage", ""),
        "station_id": s.get("stationuuid", ""), "bitrate": s.get("bitrate", 0),
        "schedule": clean_text(s.get("schedule", ""), 500)}


def live_youtube(query):
    out = []
    for v in youtube_search(query + " music radio live", 12, live=True):
        if not re.fullmatch(r"[\w-]{11}", v.get("id", "")):
            continue
        if v.get("live_status") != "is_live" and v.get("is_live") is not True:
            continue
        # Weak genre labels remain eligible; metadata is a prediction, not proof.
        out.append({"name": clean_text(v.get("title", "")), "source": "YouTube Live",
            "url": "https://www.youtube.com/watch?v=" + v["id"], "tags": query,
            "description": clean_text(v.get("description", ""), 400),
            "homepage": v.get("channel_url", ""), "country": "", "language": "",
            "channel": clean_text(v.get("channel", "")), "schedule": ""})
    return out


def soma_catalog():
    result = []
    for s in request_json("https://somafm.com/channels.json").get("channels", []):
        choices = [p for p in s.get("playlists", []) if p.get("format") == "mp3"]
        if not choices:
            continue
        # Public SomaFM channel URLs follow the broadcaster's PLS convention;
        # fetch only the station playlist when selected, before any capture.
        result.append({"name": "SomaFM: " + clean_text(s["title"]), "source": "Radio",
            "url": choices[0]["url"], "playlist_url": True, "station_id": "somafm:" + s["id"],
            "tags": clean_text(s.get("genre", "")), "description": clean_text(s.get("description", ""), 500),
            "country": "United States", "countrycode": "US", "language": "", "homepage": "https://somafm.com/" + s["id"] + "/",
            "schedule": clean_text(s.get("schedule", ""), 500)})
    return result


def resolve_station(item):
    if not item.get("playlist_url"):
        return item["url"]
    urls = re.findall(r"(?im)^File\d+=(https?://[^\r\n]+)", request_text(item["url"]))
    if not urls or not public_url(urls[0].strip()):
        raise ValueError("Station playlist has no public stream")
    return urls[0].strip()


def deduplicate(items):
    result, seen = [], set()
    for item in items:
        if not item.get("name") or not public_url(item.get("url", "")):
            continue
        host = urllib.parse.urlsplit(item.get("homepage", "")).netloc.casefold()
        name_key = (item["name"].casefold(), item.get("country", "").casefold(), host)
        identity = item.get("station_id") or item["url"]
        if identity in seen or item["url"] in seen or name_key in seen:
            continue
        seen.update((identity, item["url"], name_key))
        result.append(dict(item, id="c" + str(len(result))))
    return result


def worldwide_catalog(plan, deeper=False):
    directory = RadioDirectory()
    tasks = [("radio tag", lambda tag=tag: directory.search("tag", tag)[0]) for tag in plan["radio_tags"][:8]]
    broad = [tag for tag in ("bass", "electronic", "dubstep", "ambient", "jazz", "rock", "classical", "folk", "house", "techno")
             if tag not in plan["radio_tags"] and any(tag in genre.lower() for genre in plan["radio_tags"])]
    tasks += [("radio adjacent/broad tag", lambda tag=tag: directory.search("tag", tag)[0]) for tag in broad[:2]]
    tasks += [("radio name/scene", lambda term=term: directory.search("name", term, 35)[0])
              for term in plan.get("station_terms", [])[:2]]
    tasks += [("radio exploration", lambda: directory.search("", "", 100 if deeper else 60)[0]),
              ("SomaFM catalog", soma_catalog)]
    tasks += [("YouTube Live", lambda q=q: live_youtube(q)) for q in plan["youtube_queries"][:4]]
    items, issues, sources = [], [], []
    with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
        futures = {pool.submit(fn): label for label, fn in tasks}
        for future in concurrent.futures.as_completed(futures):
            label = futures[future]
            try:
                entries = future.result()
                items.extend(entries)
                sources.append({"source": label, "returned": len(entries)})
            except Exception:
                issues.append(label + " unavailable; discovery coverage reduced")
            progress(f"Worldwide catalogs: {len(items)} entries retrieved; {len(sources)}/{len(tasks)} searches succeeded")
    random.SystemRandom().shuffle(items)
    return deduplicate(items), {"searches": sources, "issues": list(dict.fromkeys(issues)),
                                "directory_requests": directory.requests, "scope": "worldwide sources; sampled search coverage"}
