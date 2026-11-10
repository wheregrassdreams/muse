#!/bin/sh
# Fetch the exact native archive paired with tantivy-go in native/go.mod.
# Checksums are pinned from anyproto/anytype-heart/tantivity_sha256.txt.
set -eu
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
version=v1.0.6
binding_version=$(awk '$1 == "github.com/anyproto/tantivy-go" {print $2}' "$repo_root/native/go.mod")
if [ "$binding_version" != "$version" ]; then
  echo "Tantivy Go binding ($binding_version) and native archive ($version) differ; update the pinned archive/checksums together." >&2
  exit 1
fi
target_os=${GOOS:-$(go env GOOS)}
target_arch=${GOARCH:-$(go env GOARCH)}
target="$target_os-$target_arch"
case "$target" in
  darwin-arm64) digest=f0e4fa9bdca31da3b115db4fbe6689c5d6cab771be8f9468e5309c818b3f8add ;;
  darwin-amd64) digest=10916b0f4f36c4d9fb115b6aa4f70f4ec0a36d3634fb755fe9b9addb9a33c325 ;;
  linux-amd64) target=linux-amd64-musl; digest=88b34e006707bfec2d29e8b26405f67cbb975d06fd4ea164da856d0084aaabbc ;;
  linux-arm64) target=linux-arm64-musl; digest=0a4a9143388977df41a174075ce05cd6aee2821da6425b5b9f9d591fe35d3756 ;;
  windows-amd64) digest=28b3a02cb3a3ad9d54675e7e2e65494c03183055e1b59f6866fd9dcf77b062e9 ;;
  *) echo "Tantivy native build is not configured for $target" >&2; exit 1 ;;
esac
destination="$repo_root/native/.mygo/tantivy/$version/$target"
if [ ! -s "$destination/libtantivy_go.a" ]; then
  mkdir -p "$destination"
  staging=$(mktemp -d "$destination/.download.XXXXXX")
  trap 'rm -rf "$staging"' EXIT HUP INT TERM
  echo "Preparing Tantivy $version ($target)..." >&2
  curl --fail --location --silent --show-error --retry 2 \
    "https://github.com/anyproto/tantivy-go/releases/download/$version/$target.tar.gz" \
    -o "$staging/archive.tar.gz"
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$staging/archive.tar.gz" | cut -d ' ' -f 1)
  else
    actual=$(shasum -a 256 "$staging/archive.tar.gz" | cut -d ' ' -f 1)
  fi
  [ "$actual" = "$digest" ] || { echo "Tantivy archive checksum mismatch" >&2; exit 1; }
  tar -xzf "$staging/archive.tar.gz" -C "$staging" ./libtantivy_go.a
  test -s "$staging/libtantivy_go.a"
  mv "$staging/libtantivy_go.a" "$destination/libtantivy_go.a"
fi
if [ "$target_os" = windows ] && command -v cygpath >/dev/null 2>&1; then
  cygpath -m "$destination"
else
  printf '%s\n' "$destination"
fi
