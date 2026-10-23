#!/bin/sh
# Custom source installer. Build this checkout instead of fetching upstream binaries.
set -eu
exec bash "$(dirname -- "$0")/scripts/install-omarchy.sh" "$@"
