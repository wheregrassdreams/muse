#!/bin/sh
# Runs Muse on a copied multi-note vault with nested folders and image assets.
# The vault starts over on every launch, so edits are disposable.
#
#   sh scripts/play.sh              build, open it, rebuild and reopen on save
#   sh scripts/play.sh --build-only build it and leave the note ready
#   sh scripts/play.sh --once       build, open it once, stop with the window
#
# A rebuild is a new binary, so the window closes and opens again; the window
# keeps its place (window-state.json), so the note comes back where it was.
# The options pass through: MUSE_THEME=dark sh scripts/play.sh.
# Memory diagnostics are enabled by default for play runs: five-second Go/image
# cache snapshots, GC traces, a startup heap profile, and a second profile after
# the Go heap crosses 50 MiB under .mygo/play/diagnostics.
# Set MUSE_MEMORY_DIAGNOSTICS=0 to disable snapshots or override
# MUSE_MEMORY_PROFILE_PATH to choose another heap profile location.
set -eu
cd "$(dirname "$0")/.."
. scripts/signing.sh

out=native/.mygo/play
diagnostics_abs=$(pwd)/$out/diagnostics
diagnostics_log=$diagnostics_abs/play.log
memory_profile=${MUSE_MEMORY_PROFILE_PATH:-$diagnostics_abs/heap.pprof}
fixture=native/testdata/vault-fixture
vault_abs=$(pwd)/$out/vault
cache_abs=$(pwd)/$out/cache
note_abs=$vault_abs/Home.md
mode=${1:-watch}

# A fresh copy for every launch: the fixture vault is fixed test input, not a
# note library to keep. Edits made in it are gone when the window opens again.
reset() {
  mkdir -p "$out"
  rm -rf "$vault_abs"
  mkdir -p "$vault_abs"
  cp -R "$fixture"/. "$vault_abs"/
}

build() {
  sh scripts/go.sh build -C native -o .mygo/play/Muse ./cmd/muse-native
  identity="$(mygo_sign_identity)"
  if [ -n "$identity" ] && command -v codesign >/dev/null 2>&1; then
    codesign --force --sign "$identity" --timestamp=none "$out/Muse"
  elif [ "$(uname -s)" = Darwin ]; then
    echo "warning: no Apple Development identity found; Muse is unsigned," >&2
    echo "         so macOS asks for permissions again after every rebuild." >&2
  fi
}

# The editor resolves its resources from its own directory, so it starts
# there; exec keeps the pid the one to stop.
launch() {
  mkdir -p "$diagnostics_abs"
  if [ "$diagnostics_log_started" = 0 ]; then
    : > "$diagnostics_log"
    rm -f "$memory_profile.high"
    diagnostics_log_started=1
  fi
  printf '\n--- Muse play launch %s ---\n' "$(date '+%Y-%m-%d %H:%M:%S %z')" >> "$diagnostics_log"
  if [ "${MUSE_MEMORY_DIAGNOSTICS:-1}" = 1 ]; then
    echo "memory diagnostics enabled; log: $diagnostics_log; heap profiles: $memory_profile and $memory_profile.high"
  else
    echo "memory diagnostics disabled (MUSE_MEMORY_DIAGNOSTICS=$MUSE_MEMORY_DIAGNOSTICS); log: $diagnostics_log"
  fi
  ( cd native && \
      MUSE_WORKSPACE="$vault_abs" \
      MUSE_OPEN="$note_abs" \
      MUSE_CACHE_DIR="${MUSE_CACHE_DIR:-$cache_abs}" \
      MUSE_MEMORY_DIAGNOSTICS="${MUSE_MEMORY_DIAGNOSTICS:-1}" \
      MUSE_MEMORY_PROFILE_PATH="$memory_profile" \
      GODEBUG="${GODEBUG:-gctrace=1}" \
      exec ./.mygo/play/Muse ) >> "$diagnostics_log" 2>&1 &
  pid=$!
}

# The files whose change means a rebuild; the fixture is one of them, so
# editing it brings the note back with what was written.
sources() {
  find native/app native/application native/agent native/config native/document native/editor native/events \
    native/locales native/markdown native/nativeui native/platform native/search native/theme native/ui \
    native/vault native/workspace native/cmd "$fixture" \
    -newer "$1" \( -name '*.go' -o -name '*.md' -o -name '*.png' -o -name '*.jpg' -o -name '*.jpeg' -o -name '*.gif' -o -name '*.webp' \) -print -quit 2>/dev/null
}

pid=
diagnostics_log_started=0
stop() {
  [ -n "$pid" ] || return 0
  kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
  pid=
}

reset
build
if [ "$mode" = --build-only ]; then
  echo "vault ready: $vault_abs"
  exit 0
fi

launch
if [ "$mode" = --once ]; then
  wait "$pid"
  exit 0
fi

stamp=$(mktemp)
trap 'stop; rm -f "$stamp"; exit 0' INT TERM
echo "watching native/**/*.go and $fixture — save a file to reopen the note"
while :; do
  sleep 0.4
  if [ -z "$(sources "$stamp")" ]; then
    # The window closing is the end of the session.
    kill -0 "$pid" 2>/dev/null || exit 0
    continue
  fi
  touch "$stamp"
  echo "change seen — rebuilding"
  stop
  if ! build; then
    echo "build failed — save again once it is fixed" >&2
    continue
  fi
  reset
  launch
done
