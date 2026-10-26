#!/usr/bin/env python3
"""JSON stdin/stdout worker used by CLIamp's Alt+R picker."""
import json
import sys
from cliamp_vibe.common import clean_text


def main():
    from cliamp_vibe.engine import match
    raw = sys.stdin.read(4_000_001)
    if len(raw) > 4_000_000:
        raise ValueError("Playlist input exceeds size limit")
    request = json.loads(raw)
    if not isinstance(request, dict) or not isinstance(request.get("tracks"), list):
        raise ValueError("Invalid playlist request")
    print(json.dumps(match(request)), flush=True)


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(json.dumps({"error": clean_text(error, 500)}), flush=True)
        sys.exit(1)
