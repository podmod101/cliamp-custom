"""Bounded IO, cancellation, and private cache helpers."""
import atexit
import json
import os
from pathlib import Path
import signal
import subprocess
import threading
import tempfile
import urllib.request
import urllib.parse
import ipaddress
import time
from contextlib import contextmanager

CHILDREN = set()
LOCK = threading.Lock()
STOPPED = threading.Event()
COMMAND_CONTEXT = threading.local()
CACHE = Path(os.environ.get("XDG_CACHE_HOME", str(Path.home() / ".cache"))) / "cliamp/vibe"


def progress(message):
    print(json.dumps({"progress": message}), file=__import__("sys").stderr, flush=True)


def stop_children(*_):
    STOPPED.set()
    with LOCK:
        for child in list(CHILDREN):
            if child.poll() is None:
                child.terminate()
    if _:
        raise SystemExit(130)


atexit.register(stop_children)
signal.signal(signal.SIGTERM, stop_children)


class CommandError(RuntimeError):
    def __init__(self, program, kind):
        self.kind = kind
        super().__init__(f"{program}: {kind.replace('_', ' ')}")


def command_error_kind(stderr):
    text = stderr.lower()
    if any(x in text for x in ("sign in to confirm", "login required", "authentication required", "cookies are no longer valid")):
        return "youtube_auth_required"
    if any(x in text for x in ("too many requests", "http error 429", "rate-limited")):
        return "rate_limited"
    if "requested format is not available" in text:
        return "format_unavailable"
    if "403" in text or "forbidden" in text:
        return "access_denied"
    if any(x in text for x in ("video unavailable", "private video", "not available")):
        return "unavailable"
    return "command_failed"


@contextmanager
def command_budget(seconds):
    previous = getattr(COMMAND_CONTEXT, "deadline", None)
    COMMAND_CONTEXT.deadline = time.monotonic() + max(0, seconds)
    try:
        yield
    finally:
        COMMAND_CONTEXT.deadline = previous


def run(args, timeout=45, binary=False, env=None):
    deadline = getattr(COMMAND_CONTEXT, "deadline", None)
    if deadline is not None:
        timeout = min(timeout, deadline - time.monotonic())
    if timeout <= 0:
        raise CommandError(Path(args[0]).name, "timeout")
    with LOCK:
        if STOPPED.is_set():
            raise RuntimeError("Matching cancelled")
        child = subprocess.Popen(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
        CHILDREN.add(child)
    try:
        try:
            out, stderr = child.communicate(timeout=timeout)
        except subprocess.TimeoutExpired:
            raise CommandError(Path(args[0]).name, "timeout") from None
        if child.returncode:
            # Raw stderr can contain signed media URLs or authentication details.
            # Only a bounded category escapes the subprocess wrapper.
            raise CommandError(Path(args[0]).name, command_error_kind(stderr.decode("utf-8", errors="replace")))
        if len(out) > 8_000_000:
            raise RuntimeError("Response exceeds size limit")
        return out if binary else out.decode("utf-8")
    finally:
        if child.poll() is None:
            child.kill()
            child.wait()
        with LOCK:
            CHILDREN.discard(child)


def public_url(value):
    try:
        parsed = urllib.parse.urlsplit(value)
        if parsed.scheme not in ("http", "https") or not parsed.hostname or parsed.username:
            return False
        host = parsed.hostname.lower()
        if host == "localhost" or host.endswith((".local", ".localhost")):
            return False
        try:
            return ipaddress.ip_address(host).is_global
        except ValueError:
            return "." in host
    except ValueError:
        return False


def request_json(url, payload=None, timeout=20):
    if STOPPED.is_set():
        raise RuntimeError("Matching cancelled")
    data = None if payload is None else json.dumps(payload).encode()
    request = urllib.request.Request(url, data=data, headers={
        "User-Agent": "CLIamp-playlist-vibe/1.0", "Content-Type": "application/json"})
    with urllib.request.urlopen(request, timeout=timeout) as response:
        raw = response.read(2_000_001)
    if len(raw) > 2_000_000:
        raise RuntimeError("Response exceeds size limit")
    return json.loads(raw)


def request_text(url):
    if STOPPED.is_set():
        raise RuntimeError("Matching cancelled")
    request = urllib.request.Request(url, headers={"User-Agent": "CLIamp-playlist-vibe/1.0"})
    with urllib.request.urlopen(request, timeout=12) as response:
        raw = response.read(100001)
    if len(raw) > 100000:
        raise RuntimeError("Playlist response exceeds size limit")
    return raw.decode("utf-8")


def private_json(path, data):
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    fd, temp = tempfile.mkstemp(prefix=".vibe-", dir=path.parent)
    try:
        with os.fdopen(fd, "w") as stream:
            json.dump(data, stream)
        os.replace(temp, path)
    finally:
        Path(temp).unlink(missing_ok=True)


def clean_text(text, limit=240):
    return " ".join("".join(c for c in str(text) if c.isprintable()).split())[:limit]
