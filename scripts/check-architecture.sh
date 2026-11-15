#!/bin/sh
# Enforce the package boundaries recorded in docs/architecture.md.
set -eu
cd "$(dirname "$0")/.."

failed=0
fail() {
  printf 'architecture check: %s\n' "$1" >&2
  failed=1
}

if rg -n '"muse/(mdoc|mdedit|vaultsearch|vaultindex|vaultfiles|vaultwatch|notestore|scrollui|appearance|services|notedate|frontmatter|cachepath|textnav)"' native --glob '*.go'; then
  fail "legacy import paths remain"
fi

if rg -n '^package (mdoc|mdedit|vaultsearch|vaultindex|vaultwatch|notestore|scrollui|appearance|services|textnav)$' native --glob '*.go'; then
  fail "legacy package declarations remain"
fi

if rg -n '^package nativeui$' native/nativeui/panels --glob '*.go'; then
  fail "nativeui/panels contains child packages that masquerade as the parent package"
fi

if rg -n '"muse/(nativeui|platform|cmd)/' \
  native/document native/editor native/markdown native/search native/vault \
  --glob '*.go' --glob '!**/*_test.go'; then
  fail "domain modules import presentation, platform adapters, or executables"
fi

if rg -n '"muse/(nativeui|platform|cmd)/' native/application --glob '*.go' --glob '!**/*_test.go'; then
  fail "application use cases import presentation, platform adapters, or executables"
fi

if rg -n '"muse/platform/' native/nativeui --glob '*.go' --glob '!**/*_test.go'; then
  fail "nativeui imports platform adapters directly instead of receiving capabilities"
fi

if rg -n '"(muse/nativeui|github\.com/egoist/mygo)' native/app --glob '*.go' --glob '!**/*_test.go'; then
  fail "app module assembly imports the desktop presentation runtime"
fi

if rg -n '"github\.com/egoist/mygo' native/application --glob '*.go' --glob '!**/*_test.go'; then
  fail "application use cases depend on MyGo instead of backend-neutral ports"
fi

if rg -n '"muse/editor/' native/markdown --glob '*.go' --glob '!**/*_test.go'; then
  fail "markdown imports the editor module"
fi

for obsolete in native/mdoc native/mdedit native/vaultsearch native/vaultindex native/vaultfiles native/vaultwatch native/notestore native/scrollui native/appearance native/services native/application/services native/notedate native/frontmatter native/cachepath native/textnav; do
  if [ -d "$obsolete" ] && find "$obsolete" -type f -name '*.go' -print -quit | grep -q .; then
    fail "obsolete implementation directory remains: $obsolete"
  fi
done

for package in \
  native/app \
  native/cmd/muse-native \
  native/application/document \
  native/application/vault \
  native/application/workspace \
  native/events \
  native/document \
  native/document/frontmatter \
  native/document/types \
  native/vault/files \
  native/vault/index \
  native/vault/links \
  native/vault/watch \
  native/search \
  native/search/query \
  native/search/fields \
  native/search/plan \
  native/search/rank \
  native/search/quickopen \
  native/workspace \
  native/editor \
  native/editor/internal/core \
  native/editor/internal/textlayout \
  native/editor/input \
  native/editor/navigation \
  native/editor/layout \
  native/editor/blocks/inline \
  native/editor/blocks/paragraph \
  native/editor/blocks/table \
  native/markdown \
  native/markdown/syntax \
  native/markdown/render \
  native/markdown/images \
  native/markdown/math \
  native/markdown/diagrams \
  native/agent \
  native/config \
  native/theme \
  native/ui/scrollbar \
  native/ui/iconlibrary \
  native/nativeui \
  native/nativeui/components \
  native/nativeui/components/doctypes \
  native/nativeui/interactions \
  native/nativeui/panels/filetree \
  native/nativeui/panels/outline \
  native/nativeui/panels/search \
  native/nativeui/panels/quickopen \
  native/nativeui/panels/naviconpicker \
  native/nativeui/panels/vaulthome \
  native/nativeui/panels/properties \
  native/nativeui/panels/agent \
  native/nativeui/panels/settings \
  native/platform/filesystem/documentstore \
  native/platform/filesystem/imagecache \
  native/platform/filesystem/naviconstore \
  native/platform/filesystem/recentstore \
  native/platform/filesystem/vaultfiles \
  native/platform/filesystem/vaultsearchstore \
  native/platform/filesystem/workspacestore \
  native/platform/filesystem/watcher \
  native/platform/cachepath \
  native/locales; do
  if [ ! -d "$package" ] || ! find "$package" -maxdepth 1 -type f -name '*.go' ! -name '*_test.go' -print -quit | grep -q .; then
    fail "target package is missing an implementation: $package"
  fi
done

if [ "$failed" -ne 0 ]; then
  exit 1
fi
printf 'architecture check: package tree and import boundaries pass\n'
