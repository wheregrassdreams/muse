#!/bin/sh
# Builds the Muse binary signed with a stable identity, so macOS keeps
# permission grants (files and folders, screen recording) across rebuilds.
set -eu
cd "$(dirname "$0")/.."
. scripts/common.sh

muse_build_signed "$muse_root/out/Muse"
