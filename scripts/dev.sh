#!/bin/sh
# Runs Muse from source. The binary is signed with a stable identity so macOS
# keeps permission grants (files and folders, screen recording) across
# rebuilds.
set -eu
cd "$(dirname "$0")/.."
. scripts/signing.sh

identity="$(mygo_sign_identity)"
mkdir -p native/.mygo/dev
sh scripts/generate.sh
sh scripts/go.sh build -C native -o .mygo/dev/Muse ./cmd/muse-native

if [ -n "$identity" ] && command -v codesign >/dev/null 2>&1; then
  codesign --force --sign "$identity" --timestamp=none native/.mygo/dev/Muse
elif [ "$(uname -s)" = Darwin ]; then
  echo "warning: no Apple Development identity found; Muse is unsigned," >&2
  echo "         so macOS asks for permissions again after every rebuild." >&2
fi

# Muse resolves its resources relative to the working directory.
cd native
exec ./.mygo/dev/Muse "$@"
