#!/usr/bin/env bash
# Build this checkout, never download an upstream release over the custom build.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: bash scripts/install-omarchy.sh [--install-deps] [--help]

Builds and tests this checkout, then installs ~/.local/bin/cliamp,
~/.local/bin/cliamp-custom, and a user desktop launcher. Previous files are
backed up. Shell and Hyprland settings are left for the instructions in
docs/omarchy.md, so existing custom bindings can be preserved.

--install-deps  Install build/audio tools through `omarchy pkg add`.
               This can request sudo and requires network access.
EOF
}

install_deps=false
for arg in "$@"; do
  case "$arg" in
    --help|-h) usage; exit 0 ;;
    --install-deps) install_deps=true ;;
    *) usage >&2; exit 2 ;;
  esac
done

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
if "$install_deps"; then
  command -v omarchy >/dev/null || { echo 'Omarchy CLI is required.' >&2; exit 1; }
  omarchy pkg add go gcc make pkgconf alsa-lib ffmpeg yt-dlp songrec
fi

for tool in make gcc pkg-config ffmpeg ffprobe yt-dlp; do
  command -v "$tool" >/dev/null || { echo "Missing $tool; see docs/omarchy.md or use --install-deps." >&2; exit 1; }
done
pkg-config --exists alsa || { echo 'Missing ALSA development files (alsa-lib).' >&2; exit 1; }

# Use the exact pinned toolchain without changing the user's global selection.
required_go=$(awk '$1 == "go" {print $2; exit}' go.mod)
if command -v mise >/dev/null; then
  # This checkout's mise.toml contains only the Go version (review before install).
  mise trust "$root/mise.toml"
  if ! mise where "go@$required_go" >/dev/null 2>&1; then
    mise install "go@$required_go"
  fi
  go_runner=(mise exec "go@$required_go" --)
else
  command -v go >/dev/null || { echo 'Install Go or mise first; see docs/omarchy.md.' >&2; exit 1; }
  go_runner=(env "GOTOOLCHAIN=go$required_go")
fi

"${go_runner[@]}" make check
build_dir=$(mktemp -d)
trap 'rm -rf -- "$build_dir"; if [[ -n ${install_tmp:-} ]]; then rm -f -- "$install_tmp"; fi' EXIT
version=v2.3.0+search-history.youtube-cleanup
"${go_runner[@]}" go build -trimpath -ldflags="-s -w -X main.version=$version" -o "$build_dir/cliamp" .

bin_dir="$HOME/.local/bin"
data_dir="${XDG_DATA_HOME:-$HOME/.local/share}"
state_dir="${XDG_STATE_HOME:-$HOME/.local/state}"
mkdir -p "$bin_dir" "$data_dir/applications" "$state_dir"
backup=$(mktemp -d "$state_dir/cliamp-custom-install.XXXXXXXX")
backup_file() {
  local target=$1 name=$2
  if [[ -e "$target" || -L "$target" ]]; then
    cp -a -- "$target" "$backup/$name"
    printf 'Backed up %s as %s\n' "$target" "$backup/$name"
  else
    printf '%s\n' "$target" >> "$backup/new-files.txt"
  fi
}
atomic_install() {
  local source=$1 target=$2 mode=$3
  install_tmp=$(mktemp "$(dirname -- "$target")/.cliamp-install.XXXXXXXX")
  install -m "$mode" -- "$source" "$install_tmp"
  mv -f -- "$install_tmp" "$target"
  install_tmp=
}
backup_file "$bin_dir/cliamp" cliamp
backup_file "$bin_dir/cliamp-custom" cliamp-custom
backup_file "$data_dir/applications/cliamp.desktop" cliamp.desktop
atomic_install "$build_dir/cliamp" "$bin_dir/cliamp" 755

cat > "$build_dir/cliamp-custom" <<'EOF'
#!/usr/bin/env bash
exec "$HOME/.local/bin/cliamp" "$@"
EOF
atomic_install "$build_dir/cliamp-custom" "$bin_dir/cliamp-custom" 755

# sh expands the home directory at launch time; no username is baked in.
cat > "$build_dir/cliamp.desktop" <<'EOF'
[Desktop Entry]
Name=CLIamp Custom
GenericName=Music Player
Comment=Terminal music player with YouTube downloads and track organization
Exec=sh -c "exec ~/.local/bin/cliamp"
Icon=audio-x-generic
Terminal=true
Type=Application
Categories=Audio;Music;Player;AudioVideo;ConsoleOnly;
Keywords=music;audio;player;terminal;youtube;
StartupNotify=false
EOF
atomic_install "$build_dir/cliamp.desktop" "$data_dir/applications/cliamp.desktop" 644
if command -v update-desktop-database >/dev/null; then
  update-desktop-database "$data_dir/applications" || true
fi
printf '\nInstalled %s\nBackup: %s\n' "$version" "$backup"
printf 'Launch: %s/cliamp-custom\nNext: docs/omarchy.md (shell alias and Music TUI shortcut).\n' "$bin_dir"
if ! command -v songrec >/dev/null && [[ ! -x "$HOME/.local/libexec/cliamp/songrec" && ! -x ${CLIAMP_SONGREC:-/nonexistent} ]]; then
  echo 'SongRec is missing: downloads/splitting work; online artist/title recognition needs songrec.'
fi
