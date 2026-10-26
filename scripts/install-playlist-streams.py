#!/usr/bin/env python3
"""Opt-in installer for the independent local audio matcher. No sudo."""
import argparse
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--prefix", type=Path, default=Path.home() / ".local/libexec/cliamp-vibe")
    parser.add_argument("--model-cache", type=Path, default=Path.home() / ".local/share/cliamp-vibe/models")
    parser.add_argument("--copy-model-cache", type=Path, help="Reuse already-downloaded Hugging Face hub cache")
    args = parser.parse_args()
    prefix, cache = args.prefix.resolve(), args.model_cache.resolve()
    scripts = Path(__file__).resolve().parent
    for command in ("ffmpeg", "yt-dlp", "openclaw"):
        if not shutil.which(command):
            raise SystemExit(f"Required existing command missing: {command}")
    if prefix.exists():
        backup = prefix.with_name(prefix.name + ".backup-" + time.strftime("%Y%m%d-%H%M%S"))
        if backup.exists():
            raise SystemExit(f"Backup already exists: {backup}")
        prefix.rename(backup)
        print(f"Previous helper preserved at {backup}", flush=True)
    prefix.mkdir(parents=True)
    subprocess.run([sys.executable, "-m", "venv", str(prefix / "venv")], check=True)
    python = prefix / "venv/bin/python"
    subprocess.run([str(python), "-m", "pip", "install", "torch==2.14.1+cpu",
                    "--index-url", "https://download.pytorch.org/whl/cpu"], check=True)
    subprocess.run([str(python), "-m", "pip", "install", "-r", str(scripts / "vibe-requirements.txt")], check=True)
    cache.mkdir(parents=True, exist_ok=True)
    if args.copy_model_cache:
        model_folder = "models--laion--clap-htsat-unfused"
        shutil.copytree(args.copy_model_cache / model_folder, cache / model_folder,
                        dirs_exist_ok=True, symlinks=True)
    env = dict(os.environ, HF_HUB_CACHE=str(cache), HF_HUB_DISABLE_TELEMETRY="1")
    subprocess.run([str(python), "-c", "from transformers import ClapModel,ClapProcessor; "
                    "p='laion/clap-htsat-unfused'; ClapProcessor.from_pretrained(p); "
                    "ClapModel.from_pretrained(p); print('Local CLAP model ready')"], env=env, check=True)
    shutil.copytree(scripts / "cliamp_vibe", prefix / "cliamp_vibe",
                    ignore=shutil.ignore_patterns("__pycache__", "*.pyc"))
    shutil.copy2(scripts / "playlist_streams.py", prefix / "playlist_streams.py")
    runner = "#!/bin/sh\n"
    runner += "export HF_HUB_CACHE=" + shlex.quote(str(cache)) + "\n"
    runner += "export HF_HUB_DISABLE_TELEMETRY=1\nexport TOKENIZERS_PARALLELISM=false\n"
    runner += "exec " + shlex.quote(str(python)) + " " + shlex.quote(str(prefix / "playlist_streams.py")) + "\n"
    (prefix / "run").write_text(runner)
    (prefix / "run").chmod(0o755)
    with (prefix / "installed-requirements.txt").open("w") as stream:
        subprocess.run([str(python), "-m", "pip", "freeze"], stdout=stream, check=True)
    print(f"Audio helper installed: {prefix / 'run'}")
    print("Rebuild CLIamp with the playlist-streams feature, then press Alt+R.")


if __name__ == "__main__":
    main()
