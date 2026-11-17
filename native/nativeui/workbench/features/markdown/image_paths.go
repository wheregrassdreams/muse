package markdown

import (
	"strings"

	"muse/locales"
	"muse/markdown/images"
)

// MissingImageMessage is the localized renderer label for a missing source.
func MissingImageMessage(src string) string {
	src = strings.TrimSpace(src)
	if len(src) >= len("data:") && strings.EqualFold(src[:len("data:")], "data:") {
		return locales.DocumentMissingImage()
	}
	src = strings.ReplaceAll(src, "\\", "/")
	if src == "" {
		return locales.DocumentMissingImage()
	}
	return locales.DocumentMissingImageNamed(src)
}

// ResolveImageSource resolves a path relative to a document or vault root.
// Vault-rooted resolution requires the LocalFiles capability for confinement.
func ResolveImageSource(src, base, root string) string {
	return ResolveImageSourceWithLocalFilesAndLookup(src, base, root, nil, nil)
}

// ResolveImageSourceWithLookup adapts the native UI's vault lookup callback to
// the Markdown feature's renderer-neutral source resolver.
func ResolveImageSourceWithLookup(src, base, root string, lookup func(name string) string) string {
	return ResolveImageSourceWithLocalFilesAndLookup(src, base, root, nil, lookup)
}

// ResolveImageSourceWithLocalFilesAndLookup resolves a source using the
// platform-provided file capability and an optional catalog attachment lookup.
func ResolveImageSourceWithLocalFilesAndLookup(src, base, root string, files images.LocalFiles, lookup func(name string) string) string {
	return images.ResolveSource(src, images.SourceOptions{BaseDir: base, Root: root, Lookup: lookup, LocalFiles: files})
}
