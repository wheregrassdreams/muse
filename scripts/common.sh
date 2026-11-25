#!/bin/sh
# Shared paths and build helpers for Muse's local scripts.
# Source this file; it is not meant to run on its own:
#
#   . "$(dirname "$0")/common.sh"
set -eu

muse_scripts_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
muse_root=$(CDPATH= cd -- "$muse_scripts_dir/.." && pwd)
. "$muse_scripts_dir/signing.sh"

muse_generate() {
  sh "$muse_scripts_dir/generate.sh"
}

# muse_go_build <output>
# Builds cmd/muse-native without regenerating locale code. Outputs are absolute
# so callers cannot accidentally depend on their current directory. The _muse_
# prefix keeps helper variables from overwriting a caller's own globals.
muse_go_build() {
  _muse_out=$1
  case "$_muse_out" in
    /*) ;;
    *) _muse_out="$muse_root/$_muse_out" ;;
  esac
  mkdir -p "$(dirname -- "$_muse_out")"
  sh "$muse_scripts_dir/go.sh" build -C "$muse_root/native" -o "$_muse_out" ./cmd/muse-native
}

# muse_build <output>
# Regenerates MyGo artifacts, then builds the app.
muse_build() {
  muse_generate
  muse_go_build "$1"
}

# muse_sign <binary>
# Signs with the stable local identity so macOS keeps TCC grants across builds.
muse_sign() {
  _muse_sign_path=$1
  _muse_identity=$(mygo_sign_identity)
  if [ -n "$_muse_identity" ] && command -v codesign >/dev/null 2>&1; then
    codesign --force --sign "$_muse_identity" --timestamp=none "$_muse_sign_path"
  elif [ "$(uname -s)" = Darwin ]; then
    echo "warning: no Apple Development identity found; Muse is unsigned," >&2
    echo "         so macOS asks for permissions again after every rebuild." >&2
  fi
}

muse_build_signed() {
  muse_build "$1"
  muse_sign "$1"
}
