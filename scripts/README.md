# Scripts

## Day to day

- `dev.sh` — run Muse from source, signed with a stable identity so macOS keeps permission grants across rebuilds.
- `build.sh` — build `out/Muse`, signed with the same identity.
- `test.sh` — the standard test entry point. Logs every run under `native/.mygo/test/<timestamp>-<worktree>/`.
  - `sh scripts/test.sh` — full suite; warns about gofmt drift in changed files, builds every package, then tests with `-count=1`
  - `sh scripts/test.sh ./nativeui/... -run Foo` — focused tests, forwarded to `go test`
  - `MUSE_TEST_SHOTS=1` — write UI screenshots into the run directory
  - `MUSE_TEST_JOBS=2` — limit package parallelism when several worktrees run at once

## Playground

- `play.sh` — run Muse on a disposable copy of `native/testdata/vault-fixture`, rebuilding on save. `--build-only` and `--once` are described at the top of the script.

## Checks and tooling

These are occasional checks, not separate test entry points.

- `go.sh` — run `go` with the pinned Tantivy library and the macOS SDK setup. Use this instead of a bare `go` command. For example: `sh scripts/go.sh -C native vet ./...`.
- `check-architecture.sh` — enforce the package boundaries in `docs/architecture.md`: `sh scripts/check-architecture.sh`.

## Release

- `build-macos-app.sh` — package a native Muse binary into a `.app` bundle and DMG for macOS releases. Set `MACOS_SIGNING_IDENTITY` to sign with a real certificate; without it the bundle falls back to ad-hoc signing.

## Internal helpers

These are sourced or called by the scripts above; they are not separate entry points.

- `common.sh` — shared root paths plus `muse_generate`, `muse_go_build`, `muse_build`, and signing helpers.
- `generate.sh` — the single MyGo/locale generation step used by dev and release builds.
- `prepare-tantivy.sh` — download and verify the pinned Tantivy static library for `go.sh`.
- `signing.sh` — resolve `MACOS_SIGNING_IDENTITY` or the first `Apple Development` identity in the keychain.
- `strip_locale_messages.go` — normalizes generated locale accessors after `mygo generate`.
