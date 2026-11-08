#!/bin/sh
# Runs Muse on a playground note: every kind of content in one document, and
# the document starts over on every launch, so nothing edited there is kept
# by accident.
#
#   sh scripts/play.sh              build, open it, rebuild and reopen on save
#   sh scripts/play.sh --build-only build it and leave the note ready
#   sh scripts/play.sh --once       build, open it once, stop with the window
#
# A rebuild is a new binary, so the window closes and opens again; the window
# keeps its place (window-state.json), so the note comes back where it was.
# The options pass through: MUSE_THEME=dark sh scripts/play.sh.
set -eu
cd "$(dirname "$0")/.."
. scripts/signing.sh

out=native/.mygo/play
fixture=native/testdata/playground.md
note_abs=$(pwd)/$out/playground.md
mode=${1:-watch}

# A fresh copy for every launch: the playground is a fixed input, not a note
# to keep. Edits made in it are gone when the window opens again.
reset() {
  mkdir -p "$out"
  cp "$fixture" "$out/playground.md.tmp"
  mv "$out/playground.md.tmp" "$out/playground.md"
}

build() {
  go build -C native -o .mygo/play/Muse .
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
  ( cd native && MUSE_OPEN="$note_abs" exec ./.mygo/play/Muse ) &
  pid=$!
}

# The files whose change means a rebuild; the fixture is one of them, so
# editing it brings the note back with what was written.
sources() {
  find native/mdedit native/nativeui native/mdoc native/services native/doc \
    native/cmd native/main.go "$fixture" \
    -newer "$1" \( -name '*.go' -o -name '*.md' \) -print -quit 2>/dev/null
}

pid=
stop() {
  [ -n "$pid" ] || return 0
  kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
  pid=
}

reset
build
if [ "$mode" = --build-only ]; then
  echo "playground ready: $note_abs"
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
