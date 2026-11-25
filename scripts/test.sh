#!/bin/sh
# The standard local test entry point.
#
#   sh scripts/test.sh                          full suite; gofmt warning + build first
#   sh scripts/test.sh ./nativeui/... -run Foo  focused tests; args go to go test
#
# A full run also builds every package, so command packages are compiled too.
# Every run writes a log under native/.mygo/test/<timestamp>-<worktree>/.
# Useful environment:
#   MUSE_TEST_JOBS=2        package parallelism (default 4; perf runs use 1)
#   MUSE_TEST_SHOTS=1       write UI screenshots under the run directory
#   MUSE_PERF=1             timing-sensitive tests; also forces -p 1
#   MUSE_TEST_STRICT_FMT=1  fail a full run on gofmt drift
set -eu

cd "$(dirname "$0")/.."
. scripts/common.sh

usage() {
  sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}
case "${1:-}" in
  -h|--help) usage 0 ;;
esac

worktree=$(basename -- "$muse_root")
branch=$(git -C "$muse_root" branch --show-current 2>/dev/null || true)
[ -n "$branch" ] || branch=detached
commit=$(git -C "$muse_root" rev-parse --short HEAD 2>/dev/null || echo unknown)
dirty=$(git -C "$muse_root" status --porcelain 2>/dev/null | wc -l | tr -d ' ')
run_id="$(date '+%Y%m%d-%H%M%S')-$worktree"
run_dir="$muse_root/native/.mygo/test/$run_id"
n=2
while [ -e "$run_dir" ]; do
  run_dir="$muse_root/native/.mygo/test/$run_id-$n"
  n=$((n + 1))
done
mkdir -p "$run_dir"
log="$run_dir/test.log"
: > "$log"

say() { printf '%s\n' "$*"; }

# Runs a command, tees its output to the run log, and returns the command's
# own status rather than tee's.
run() {
  status_file=$(mktemp)
  { "$@" 2>&1; printf '%s\n' "$?" >"$status_file"; } | tee -a "$log"
  status=$(cat "$status_file")
  rm -f "$status_file"
  return "$status"
}

banner() {
  say "────────────────────────────────────────"
  say "worktree   $worktree"
  say "branch     $branch"
  say "commit     $commit (+$dirty dirty)"
  say "run        $(basename -- "$run_dir")"
  say "artifacts  $run_dir"
  say "────────────────────────────────────────"
}
footer() {
  say "────────────────────────────────────────"
  say "run        $(basename -- "$run_dir")"
  say "artifacts  $run_dir"
  say "────────────────────────────────────────"
}

# Warns about gofmt drift in files this worktree changed or added. The
# repository already has older drift, so a full-tree check would be noise.
check_format() {
  changed=$(mktemp)
  {
    git -C "$muse_root" diff --name-only --diff-filter=ACMR HEAD -- '*.go'
    git -C "$muse_root" ls-files --others --exclude-standard -- '*.go'
  } | sort -u > "$changed"
  needs_format=0
  while IFS= read -r file; do
    [ -n "$file" ] || continue
    [ -f "$muse_root/$file" ] || continue
    if [ -n "$(gofmt -l "$muse_root/$file")" ]; then
      say "$file"
      needs_format=1
    fi
  done < "$changed"
  rm -f "$changed"
  if [ "$needs_format" -eq 1 ]; then
    if [ "${MUSE_TEST_STRICT_FMT:-0}" = 1 ]; then
      say "gofmt: files need formatting"
      exit 1
    fi
    say "gofmt: warning — format the files you touched"
  fi
}

banner

jobs=${MUSE_TEST_JOBS:-4}
if [ "${MUSE_PERF:-0}" = 1 ]; then
  jobs=1
fi

if [ "$#" -eq 0 ]; then
  say "== gofmt =="
  check_format
  say "== build =="
  status=0
  run sh scripts/go.sh build -C "$muse_root/native" ./... || status=$?
  if [ "$status" -ne 0 ]; then
    footer
    say "FAILED in build (exit $status)"
    exit "$status"
  fi
  set -- test ./... -count=1 -p "$jobs"
else
  set -- test "$@"
  if [ -n "${MUSE_TEST_JOBS:-}" ] || [ "${MUSE_PERF:-0}" = 1 ]; then
    set -- "$@" -p "$jobs"
  fi
fi

if [ "${MUSE_TEST_SHOTS:-0}" = 1 ]; then
  shots="$run_dir/shots"
  mkdir -p "$shots"
  export MDEDIT_SHOTS="$shots"
  export MUSE_SHOTS="$shots"
  export MUSE_UI_SHOTS="$shots"
  export MISSING_IMAGE_SHOTS="$shots"
  export MUSE_SIDEBAR_CAPTURE_DIR="$shots/sidebar"
  export MUSE_IMAGE_HOVER_CAPTURE="$shots/image-hover.png"
fi

say "command    $*"
status=0
run sh scripts/go.sh -C "$muse_root/native" "$@" || status=$?
footer
if [ "$status" -ne 0 ]; then
  say "FAILED (exit $status)"
  exit "$status"
fi
say "PASSED"
