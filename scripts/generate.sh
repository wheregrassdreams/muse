#!/bin/sh
# Regenerates MyGo artifacts and applies Muse-owned normalization to generated
# locale accessors. Keep this as the single generation entry point used by
# both development and release builds.
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
cd "$repo_root/native"

go tool mygo generate
go run ../scripts/strip_locale_messages.go -file locales/catalog_gen.go
go tool mygo i18n check
