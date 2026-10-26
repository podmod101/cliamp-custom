"""Discover real public candidates; model output never supplies stream URLs."""
import concurrent.futures
import json
import re
import random
import socket
from functools import lru_cache
import urllib.parse
from .song_lookup import song_match, song_video
from .common import request_json, request_text, run, public_url, clean_text


def youtube_search(query, count=4, live=False):
    target = "ytsearch%d:%s" % (count, query)
    if live:
        target = "https://www.youtube.com/results?" + urllib.parse.urlencode({
            "search_query": query, "sp": "EgJAAQ=="})
    data = json.loads(run(["yt-dlp", "--ignore-config", "--flat-playlist", "--dump-single-json",
        "--no-warnings", "--socket-timeout", "12", "--retries", "1", "--playlist-end", str(count),
        "--", target], timeout=75))
    return data.get("entries", [])[:count]


def words(value):
    return set(re.findall(r"[^\W_]+", str(value).casefold())) - {
        "official", "audio", "video", "music", "lyrics", "feat", "ft", "the", "topic"}


@lru_cache(maxsize=1)
def radio_servers():
    servers = set()
    try:
        for entry in socket.getaddrinfo("all.api.radio-browser.info", 443, type=socket.SOCK_STREAM):
            try:
                host = socket.gethostbyaddr(entry[4][0])[0]
                if host.endswith(".api.radio-browser.info"):
                    servers.add(host)
            except OSError:
                pass
    except OSError:
        pass
    servers.add("de1.api.radio-browser.info")
    values = list(servers)
    random.SystemRandom().shuffle(values)
    return values[:4]


def radio_candidates(tag):
    query = urllib.parse.urlencode({"tag": tag, "hidebroken": "true", "order": "votes",
                                    "reverse": "true", "limit": 14})
    entries = None
    # A public community mirror has the same open Radio Browser schema and
    # remains available when the official cluster's TLS/front door is down.
    for server in radio_servers() + ["stations.radioss.app"]:
        try:
            entries = request_json("https://" + server + "/json/stations/search?" + query)
            break
        except Exception:
            continue
    if entries is None:
        raise RuntimeError("Radio Browser mirrors unavailable")
    return [{"name": clean_text(s.get("name", "")), "url": s.get("url_resolved", ""),
             "source": "Radio", "tags": clean_text(s.get("tags", "")),
             "homepage": s.get("homepage", ""), "station_id": s.get("stationuuid", "")}
            for s in entries if s.get("name") and public_url(s.get("url_resolved", ""))]


def youtube_candidates(query):
    out = []
    for entry in youtube_search(query + " radio live 24/7", 8, live=True):
        if not re.fullmatch(r"[\w-]{11}", entry.get("id", "")):
            continue
        # The Live search filter alone is insufficient; require explicit metadata.
        if entry.get("live_status") != "is_live" and entry.get("is_live") is not True:
            continue
        if not candidate_relevant(query, entry):
            continue
        out.append({"name": clean_text(entry.get("title", "")), "source": "YouTube Live",
                    "url": "https://www.youtube.com/watch?v=" + entry["id"],
                    "tags": clean_text(query), "homepage": entry.get("channel_url", "")})
    return out


def candidate_relevant(query, entry):
    vague = {"music", "radio", "live", "24", "7", "nonstop", "relaxing", "electronic",
             "melodic", "deep", "atmospheric", "chill", "mix", "stream"}
    target = words(query) - vague
    actual = words(entry.get("title", "")) | words(entry.get("channel", ""))
    distinctive = {"dubstep", "chillstep", "psybass", "psydub", "psychill", "classical", "jazz",
                   "rock", "metal", "country", "house", "techno", "trance", "reggae", "ambient"}
    if target & actual & distinctive:
        return True
    return not target or len(target & actual) / len(target) >= .5


def soma_candidates(query):
    """An independent broadcaster catalog keeps radio discovery useful in outages."""
    channels = request_json("https://somafm.com/channels.json").get("channels", [])
    target = words(query)
    channels.sort(key=lambda c: len(target & words(c.get("title", "") + " " +
        c.get("description", "") + " " + c.get("genre", ""))), reverse=True)
    channels = [c for c in channels if target & words(c.get("title", "") + " " +
        c.get("description", "") + " " + c.get("genre", ""))]
    out = []
    for channel in channels[:14]:
        playlists = [p for p in channel.get("playlists", []) if p.get("format") == "mp3"]
        if not playlists:
            continue
        try:
            text = request_text(playlists[0]["url"])
            urls = re.findall(r"(?im)^File\d+=(https?://[^\r\n]+)", text)
            if not urls or not public_url(urls[0].strip()):
                continue
            out.append({"name": "SomaFM: " + clean_text(channel["title"]), "url": urls[0].strip(),
                "source": "Radio", "tags": clean_text(channel.get("description", "")),
                "homepage": "https://somafm.com/" + channel["id"] + "/"})
        except Exception:
            continue
    return out


def discover(queries):
    aliases = {"psychedelic bass": ["dubstep", "downtempo", "bass"],
               "deep dubstep": ["dubstep"], "glitch hop": ["glitch", "trip hop"],
               "psydub": ["psychill", "downtempo"], "experimental bass": ["dubstep", "bass"],
               "future garage": ["chillstep", "dubstep"], "liquid drum and bass": ["drum and bass"]}
    tags = []
    for raw in queries["radio_tags"]:
        tag = raw.lower().replace("-", " ").strip()
        for value in [tag] + aliases.get(tag, []):
            if value not in tags:
                tags.append(value)
    tasks = [(radio_candidates, tag) for tag in tags[:9]]
    tasks += [(youtube_candidates, query) for query in queries["youtube_queries"]]
    tasks += [(soma_candidates, " ".join(tags))]
    groups, warnings = [], []
    with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
        futures = [pool.submit(fn, value) for fn, value in tasks]
        for future, (fn, value) in zip(futures, tasks):
            try:
                groups.append(future.result())
            except Exception:
                warnings.append(f"Discovery failed for {clean_text(value, 80)}")
    # Round-robin avoids allowing the first genre to crowd out mixed playlists.
    result, seen = [], set()
    for row in range(14):
        for group in groups:
            if row >= len(group):
                continue
            item = group[row]
            identity = item.get("station_id") or item["url"]
            name = item["name"].casefold()
            if identity not in seen and name not in seen:
                seen.update((identity, name))
                result.append(item)
    # Deduplicate alternate bitrate mirrors too, using the station's name.
    return result[:36], warnings
