# Install the custom CLIamp on Omarchy

This repository contains the complete customized source based on upstream
[CLIamp v2.0.1](https://github.com/bjarneo/cliamp/tree/v2.0.1), commit
`c9cb29f85bcb5ed6d0af4f879eced8cbfd27a9a1`. No other custom CLIamp installation
is needed. Claude Code is optional: these commands also work in a terminal.

The **custom-v2.0.1** branch contains this playlist-stream build. The repository
default branch has a newer upstream base; use the branch below to get the
features and version described in this guide.

## Install

```bash
git clone --branch custom-v2.0.1 https://github.com/podmod101/cliamp-custom.git
cd cliamp-custom
bash scripts/install-omarchy.sh --install-deps
~/.local/bin/cliamp-custom
```

Read the script first. The dependency option uses Omarchy's package manager
and may request sudo. It installs Go, GCC, make, pkgconf, ALSA headers,
FFmpeg/ffprobe, yt-dlp, and SongRec. Without that option, install those tools
yourself first. Go 1.26.6 is selected through mise if available, otherwise
Go's toolchain mechanism. The installer trusts this checkout's `mise.toml`, which
only pins Go; review that file too. The toolchain and module downloads need internet.
Normal checks use local fixtures; they do not call YouTube or Shazam.

The script runs `make check`, builds this checkout, backs up any replaced
files under `${XDG_STATE_HOME:-~/.local/state}/cliamp-custom-install.*`, and
installs the executable, a `cliamp-custom` wrapper, and a user desktop entry.
It leaves the system CLIamp package intact. Choose **CLIamp Custom** in the
application launcher. The version is `v2.0.1+search-history.youtube-cleanup.playlist-streams.background`.

## Connect the terminal command and Music shortcut

Read and back up your existing settings before merging changes. On some Omarchy
systems `/usr/bin` precedes `~/.local/bin`, so `cliamp` can still select the
packaged player. For Bash, add this once to `~/.bashrc`, then open a new terminal:

```bash
alias cliamp='~/.local/bin/cliamp'
```

For Omarchy with Lua Hyprland configuration, inspect the existing Music TUI
binding first (`omarchy menu keybindings --print`). Its usual shortcut is
**Super+Shift+Alt+M**. Merge this override into `~/.config/hypr/bindings.lua`:

```lua
-- Music TUI: custom CLIamp (overrides the packaged Music TUI binding).
hl.unbind("SUPER + SHIFT + ALT + M")
o.bind("SUPER + SHIFT + ALT + M", "Music TUI", "omarchy-launch-or-focus-tui --app-id=org.omarchy.cliamp ~/.local/bin/cliamp")
```

Replace an existing custom Music TUI override rather than adding duplicates.
After editing, run `hyprctl reload` and `hyprctl configerrors`; resolve any
errors. Older Omarchy installations using `.conf` need the equivalent binding
in their existing format; do not paste Lua into a `.conf` file.

## Ask Claude to install it

Open Claude in the cloned repository and paste:

> Read CLAUDE.md, docs/omarchy.md, and LOCAL-CHANGES.md. Install this custom
> CLIamp on this Omarchy machine, including missing dependencies. First inspect
> any existing CLIamp customization record and current launch settings. Preserve
> other customizations and make backups. Run the installer, make the interactive
> shell's cliamp command and the existing Music TUI shortcut launch this custom
> executable, and validate Hyprland if you change it. Use this machine's paths
> and configuration format. Record the installation, verification, and rollback
> instructions in my local application customization log if one exists. Do not
> copy credentials, browser cookies, history, or music into this repository.

## Use

- Open a YouTube/YouTube Music item and press **Alt+S** on the player screen.
  A stream playlist is one continuous video, not every video in a YouTube playlist.
- Non-live audio is saved under `~/Music/<shortened title> [video ID]/`.
- Chapters split tracks first; otherwise FFmpeg tries silence detection.
  Crossfades may remain joined. SongRec checks samples and names confirmed
  tracks by artist/title. Unconfirmed tracks keep chapter or numbered names.
- The ordered `.m3u8` playlist opens the finished tracks. The continuous source
  is removed only after validating complete playable replacements and playlist
  coverage. If verification fails, the source remains.
- YouTube search remembers the last 20 queries; use Up/Down to recall them.
- `CLIAMP_IDENTIFY=0 ~/.local/bin/cliamp-custom` disables online recognition.
  Local downloads and splitting still work. Recognition sends fingerprints to
  Shazam through SongRec; there is no offline identification model bundled.

See [LOCAL-CHANGES.md](../LOCAL-CHANGES.md) and [yt-dlp.md](yt-dlp.md) for details.

## Updates and rollback

To update this custom build, review changes, `git pull --ff-only`, and rerun
the installer. Upstream `install.sh`, `cliamp upgrade`, upstream release
binaries, and standard package installs do not include these customizations.
The original upstream installation commands in README refer to upstream only.
Porting to another upstream release requires preserving and retesting the patch.

For rollback, close CLIamp and restore only the files changed by that installation
from its printed backup directory. If there was no previous file, `new-files.txt`
lists newly created paths that can be moved aside. Restore/remove only the Bash
alias and Music shortcut override you added, preserving later unrelated edits.
Validate Hyprland again. `/usr/bin/cliamp` launches the system package directly.
Keep your music, playlists, configuration, history, and recognition caches.

## Source and licenses

The upstream MIT license and attribution are preserved in [LICENSE](../LICENSE).
The custom source additions are also MIT licensed. Go dependencies retain their
own licenses and are downloaded from the versions in `go.mod`/`go.sum`; their
source is not vendored here. In particular, the inherited Spotify dependency
`go-librespot` is GPL-3.0, so do not assume an assembled binary is MIT-only.
This repository distributes source, not prebuilt combined binaries.

SongRec is a separate [GPL-3.0 open-source program](https://github.com/marin-m/SongRec).
Its executable and model weights are not bundled. Shazam is an external service,
not an open-source song database. No Shazam proprietary code is included.
No music, browser cookies, account configuration, or local history is included.
