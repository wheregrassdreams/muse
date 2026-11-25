#!/bin/sh
# Runs Muse from source. The binary is signed with a stable identity so macOS
# keeps permission grants (files and folders, screen recording) across
# rebuilds.
set -eu
cd "$(dirname "$0")/.."
. scripts/common.sh

out="$muse_root/native/.mygo/dev/Muse"
muse_build_signed "$out"

# Muse resolves its resources relative to the working directory.
cd "$muse_root/native"
exec "$out" "$@"
