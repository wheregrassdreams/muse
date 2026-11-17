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

if rg -n '^package nativeui$' native/nativeui --glob '*.go' | rg -v '^native/nativeui/[^/]+\.go:'; then
  fail "nativeui child directories contain files that masquerade as the parent package"
fi

if rg -n '\b(documentdialogs|commandpalette|filetree|findreplaceui|naviconpicker|outlineui|propertypanel|searchui|settingsui|agentpanel)\.State\b' native/nativeui/app.go; then
  fail "nativeui.App retains concrete panel/component state instead of handles"
fi

if rg -n 'func headingSlug|func codeLanguage|func treeProjectionSelectionLabel|func \(a \*App\) (selectedTreePaths|createTarget)\b' native/nativeui --glob '*.go'; then
  fail "nativeui retains filetree projection, document-type, or Markdown heading policy"
fi

if rg -n 'func \(a \*App\) (fileMenu|fileRootMenu)\b|func resolveWiki\b' native/nativeui --glob '*.go'; then
  fail "nativeui retains duplicate or obsolete filetree and wiki-link implementations"
fi

if rg -n '\b(isEditableFile|isImageFile|isVisibleWorkspaceFile)\b' native/nativeui --glob '*.go'; then
  fail "nativeui retains forwarding helpers for document-type policy"
fi

if rg -n '^package nativeui$' native/platform --glob '*.go'; then
  fail "platform adapters declare the presentation package instead of their adapter package"
fi

# The editor's nested directories are actual package boundaries, not filing
# locations that share the parent package. Host-coupled integration tests stay
# in components/editor; feature behavior belongs to these child packages.
for package in \
  native/nativeui/components/editor/inputadapter \
  native/nativeui/components/editor/ime \
  native/nativeui/components/editor/blocks/inline \
  native/nativeui/components/editor/blocks/paragraph \
  native/nativeui/components/editor/blocks/table \
  native/nativeui/components/editor/blocks/media \
  native/nativeui/components/editor/render \
  native/nativeui/components/editor/navigationrail \
  native/nativeui/components/editor/feedback \
  native/nativeui/components/editor/textlayout; do
  if rg -n '^package editor$' "$package" --glob '*.go'; then
    fail "editor host package files remain inside child package: $package"
  fi
  if [ ! -d "$package" ] || ! find "$package" -maxdepth 1 -type f -name '*.go' ! -name '*_test.go' -print -quit | grep -q .; then
    fail "editor child package has no production implementation: $package"
  fi
done

if rg -n 'ProductionContent|documentSession|^[[:space:]]*ed[[:space:]]+\*editorui\.Editor' \
  native/nativeui/app.go native/nativeui/components/editor --glob '*.go'; then
  fail "temporary production-editor bridge or duplicate App editor owner remains"
fi

if [ -d native/editor/internal/parser ] && find native/editor/internal/parser -type f -name '*.go' -print -quit | grep -q .; then
  fail "legacy editor parser-source implementation remains beside editor/source"
fi

if rg -n 'type[[:space:]]+parserSource|\*parserSource' native/editor native/nativeui/components/editor --glob '*.go'; then
  fail "editor component retains a parser-source owner instead of editor/source"
fi

if rg -n '"(muse/(nativeui|platform|cmd)/|github\.com/egoist/mygo)' \
  native/document native/editor native/markdown native/search native/vault \
  --glob '*.go' --glob '!**/*_test.go'; then
  fail "domain modules import presentation, MyGo, platform adapters, or executables"
fi

if rg -n '"muse/(nativeui|platform|cmd)/' native/application --glob '*.go' --glob '!**/*_test.go'; then
  fail "application use cases import presentation, platform adapters, or executables"
fi

if rg -n '"muse/(application|nativeui|platform|cmd)/' native/agent --glob '*.go' --glob '!**/*_test.go'; then
  fail "agent value model imports an outer layer"
fi

if rg -n '^type (Role|Message|PartKind|PartStatus|Part|ToolPartState|ToolRequest|ToolResult)\b' native/application/agent --glob '*.go' --glob '!**/*_test.go'; then
  fail "provider-neutral agent value types remain in application/agent"
fi

if rg -n '"(os|path/filepath)"' native/config --glob '*.go' --glob '!**/*_test.go'; then
  fail "config schema and JSON value policy perform concrete filesystem I/O"
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
  native/application/config \
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
  native/editor/input \
  native/editor/input/vim \
  native/editor/navigation \
  native/editor/source \
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
  native/application/agent \
  native/platform/agent \
  native/config \
  native/theme \
  native/ui/scrollbar \
  native/ui/iconlibrary \
  native/nativeui \
  native/nativeui/components/editor \
  native/nativeui/components/editor/textlayout \
  native/nativeui/components/markdown \
  native/nativeui/components/windowcontrols \
  native/nativeui/components/doctypes \
  native/nativeui/components/documentdialogs \
  native/nativeui/components/linkpreview \
  native/nativeui/window \
  native/nativeui/workspacepresenter \
  native/nativeui/surfaces/noteeditor \
  native/nativeui/surfaces/backlinks \
  native/nativeui/surfaces/imagepreview \
  native/nativeui/surfaces/imagedocument \
  native/nativeui/surfaces/mermaidpreview \
  native/nativeui/surfaces/quickopen \
  native/nativeui/interactions \
  native/nativeui/panels/filetree \
  native/nativeui/panels/outline \
  native/nativeui/panels/search \
  native/nativeui/panels/commandpalette \
  native/nativeui/panels/naviconpicker \
  native/nativeui/panels/vaulthome \
  native/nativeui/panels/properties \
  native/nativeui/panels/agent \
  native/nativeui/panels/settings \
  native/platform/filesystem/documentstore \
  native/platform/filesystem/configstore \
  native/platform/filesystem/agentworkspace \
  native/platform/filesystem/exportstore \
  native/platform/filesystem/filemetadata \
  native/platform/filesystem/imagecache \
  native/platform/filesystem/temporaryimages \
  native/platform/filesystem/imagefiles \
  native/platform/filesystem/naviconstore \
  native/platform/filesystem/recentstore \
  native/platform/filesystem/vaultfiles \
  native/platform/filesystem/vaultfilessource \
  native/platform/filesystem/vaultindexsource \
  native/platform/filesystem/vaultsearchstore \
  native/platform/filesystem/workspacestore \
  native/platform/filesystem/watcher \
  native/platform/externalopener \
  native/platform/process/vaultsearchindexer \
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
