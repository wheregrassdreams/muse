package mdoc

import (
	"io/fs"
	"muse/locales"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MissingImageMessage is the shared, user-facing state for an image source
// that could not be loaded by a document renderer or the image preview.
func MissingImageMessage(src string) string {
	src = strings.TrimSpace(src)
	// An embedded data URL is an opaque payload rather than a useful file
	// name. Returning it in the error label can make text layout attempt to
	// shape tens of megabytes after a decode failure.
	if len(src) >= len("data:") && strings.EqualFold(src[:len("data:")], "data:") {
		return locales.DocumentMissingImage()
	}
	src = strings.ReplaceAll(src, "\\", "/")
	if src == "" {
		return locales.DocumentMissingImage()
	}
	return locales.DocumentMissingImageNamed(src)
}

type attachmentPaths struct {
	at    time.Time
	names map[string]string
}

var attachmentIndexes sync.Map

// ResolveImageSource handles document-relative, vault-relative and bare wiki
// attachment names. The fallback catalog holds paths only, never image pixels.
func ResolveImageSource(src, base, root string) string {
	return ResolveImageSourceWithLookup(src, base, root, func(name string) string {
		return legacyAttachmentPath(root, name)
	})
}

// ResolveImageSourceWithLookup resolves normal Markdown paths first and asks
// lookup only for a bare attachment name. Callers with a maintained vault
// catalog can avoid the renderer's standalone fallback scan entirely.
func ResolveImageSourceWithLookup(src, base, root string, lookup func(name string) string) string {
	if isRemoteImage(src) || strings.HasPrefix(src, "data:") {
		return src
	}
	local := resolveImagePath(src, base)
	if abs, err := filepath.Abs(local); err == nil {
		local = abs
	}
	if root != "" && !pathWithinRoot(local, root) {
		return ""
	}
	if st, err := os.Stat(local); err == nil && !st.IsDir() {
		return local
	}
	if root == "" {
		return local
	}
	rel := resolveImagePath(src, root)
	if !pathWithinRoot(rel, root) {
		return ""
	}
	if st, err := os.Stat(rel); err == nil && !st.IsDir() {
		return rel
	}
	if strings.ContainsAny(src, "/\\") {
		return local
	}
	if lookup == nil {
		return local
	}
	if p := lookup(filepath.Base(local)); p != "" {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if pathWithinRoot(p, root) {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	return local
}

func legacyAttachmentPath(root, name string) string {
	name = strings.ToLower(filepath.Base(name))
	var idx attachmentPaths
	if old, ok := attachmentIndexes.Load(root); ok {
		idx = old.(attachmentPaths)
	}
	if idx.names == nil || time.Since(idx.at) > 10*time.Second {
		idx = attachmentPaths{at: time.Now(), names: map[string]string{}}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if path != root && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			switch strings.ToLower(filepath.Ext(d.Name())) {
			case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg", ".avif", ".heic", ".heif":
			default:
				return nil
			}
			key := strings.ToLower(d.Name())
			old := idx.names[key]
			if old == "" || len(path) < len(old) || len(path) == len(old) && path < old {
				idx.names[key] = path
			}
			return nil
		})
		attachmentIndexes.Store(root, idx)
	}
	if p := idx.names[name]; p != "" {
		return p
	}
	return ""
}

// pathWithinRoot checks both lexical traversal and existing symlink targets.
// A note inside a vault must not be able to use an image path to read outside
// the vault. Missing paths are checked lexically so they still get the normal
// missing-image placeholder rather than being treated as a valid attachment.
func pathWithinRoot(path, root string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	if !pathUnder(absRoot, absPath) {
		return false
	}
	resolvedRoot, rootErr := filepath.EvalSymlinks(absRoot)
	resolvedPath, pathErr := filepath.EvalSymlinks(absPath)
	if rootErr == nil && pathErr == nil {
		return pathUnder(resolvedRoot, resolvedPath)
	}
	return true
}

func pathUnder(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
