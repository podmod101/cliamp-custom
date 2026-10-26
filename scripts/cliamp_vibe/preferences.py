"""Optional private per-playlist taste hints; never part of source exports."""
import json
import os
from pathlib import Path


def playlist_hint(request):
    directory = os.environ.get("CLIAMP_CONFIG_DIR")
    if not directory:
        directory = str(Path(os.environ.get("XDG_CONFIG_HOME", str(Path.home() / ".config"))) / "cliamp")
    path = Path(directory) / "vibe-preferences.json"
    try:
        entries = json.loads(path.read_text())
        hint = entries.get(request.get("id", ""), {})
        if not isinstance(hint, dict):
            return {}
        return {key: hint[key] for key in ("description", "radio_tags", "youtube_queries") if key in hint}
    except FileNotFoundError:
        return {}
