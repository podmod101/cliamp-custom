"""AI-assisted search with independent recording-identity checks."""
import re
import unicodedata
from .common import clean_text, CommandError


def tokens(value):
    value = unicodedata.normalize("NFKD", str(value).casefold().replace("ø", "o"))
    value = "".join(c for c in value if not unicodedata.combining(c))
    # Stylized H U M A N and HUMAN describe the same title.
    value = re.sub(r"\b(?:[a-z]\s+){2,}[a-z]\b", lambda m: m[0].replace(" ", ""), value)
    return set(re.findall(r"[^\W_]+", value)) - {
        "official", "audio", "video", "music", "lyrics", "feat", "ft", "the", "topic"}


def title_tokens(value):
    value = re.sub(r"(?:<|ᐸ|‹|〈|＜)\s*3", "", value)
    value = re.sub(r"(?i)\b(original mix|original version|remastered(?: \d{4})?)\b", "", value)
    return tokens(value)


def song_match(song, video):
    if video.get("live_status") in ("is_live", "is_upcoming", "was_live") or video.get("is_live"):
        return False
    title, artist = title_tokens(song["title"]), tokens(song["artist"].split(",")[0])
    visible_title = title_tokens(video.get("title", ""))
    visible = visible_title | tokens(video.get("channel", ""))
    if not title or not artist or not artist.issubset(visible):
        return False
    # All meaningful title/version tokens must be present. Channel names cannot
    # supply missing song-title words or a missing remixer.
    if not title.issubset(visible_title):
        return False
    if (visible_title - title) & {"cover", "karaoke", "sped", "slowed", "nightcore", "remix", "live"}:
        return False
    duration, expected = video.get("duration"), song.get("duration", 0)
    return bool(isinstance(duration, (int, float)) and 45 <= duration <= 900 and
                (not expected or abs(duration - expected) <= max(35, expected * .2)))


def song_videos(song):
    # Lazy import preserves the discovery API and makes searches replaceable in tests.
    from .discovery import youtube_search
    fallback = song["artist"].split(",")[0].strip() + " " + song["title"]
    queries = list(dict.fromkeys(song.get("_queries", []) + [fallback]))[:3]
    feedback, matches, seen = [], [], set()
    last_error = None
    for query in queries:
        try:
            videos = youtube_search(query, 6)
        except CommandError as exc:
            last_error = exc
            feedback.append({"query": query, "error": exc.kind})
            if exc.kind in ("youtube_auth_required", "rate_limited", "timeout"):
                break
            continue
        feedback.append({"query": query, "results": [{
            "title": clean_text(v.get("title", "")), "channel": clean_text(v.get("channel", "")),
            "duration": v.get("duration")} for v in videos[:4]]})
        for video in videos:
            identity = video.get("id", "")
            if identity in seen or not re.fullmatch(r"[\w-]{11}", identity) or not song_match(song, video):
                continue
            seen.add(identity)
            matches.append({"url": "https://www.youtube.com/watch?v=" + identity,
                "duration": video["duration"], "title": clean_text(video.get("title", "")),
                "channel": clean_text(video.get("channel", "")),
                "description": clean_text(video.get("description", ""), 400)})
        if matches:
            break
    song["_lookup_feedback"] = feedback
    if not matches:
        if last_error:
            raise last_error
        raise CommandError("YouTube search", "no_confident_match")
    return matches[:2]


def song_video(song):
    return song_videos(song)[0]
