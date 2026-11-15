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

// ResolveImageSource is the compatibility adapter for callers that do not
// provide an indexed attachment lookup.
func ResolveImageSource(src, base, root string) string {
	return ResolveImageSourceWithLookup(src, base, root, func(name string) string {
		return images.FindAttachment(root, name)
	})
}

// ResolveImageSourceWithLookup adapts the native UI's vault lookup callback to
// the Markdown feature's renderer-neutral source resolver.
func ResolveImageSourceWithLookup(src, base, root string, lookup func(name string) string) string {
	return images.ResolveSource(src, images.SourceOptions{BaseDir: base, Root: root, Lookup: lookup})
}
