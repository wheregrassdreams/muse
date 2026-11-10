#!/bin/sh
# Go with Muse's pinned native search library; accepts the normal go arguments.
set -eu
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
if [ "${CGO_ENABLED:-1}" = 0 ]; then
  echo "Muse's experimental Tantivy search requires CGO_ENABLED=1." >&2
  exit 1
fi
native_search_lib=$(sh "$repo_root/scripts/prepare-tantivy.sh")
export CGO_ENABLED=1
export CGO_LDFLAGS="\"-L$native_search_lib\" ${CGO_LDFLAGS:-}"
# A Nix-provided clang on macOS may not find the system SDK libraries.
if [ "$(uname -s)" = Darwin ] && [ "${GOOS:-darwin}" = darwin ]; then
  export CC="${CC:-/usr/bin/clang}"
  export SDKROOT="${SDKROOT:-$(xcrun --show-sdk-path)}"
fi
exec go "$@"
