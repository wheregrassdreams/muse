package mdoc

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type attachmentPaths struct {
	at    time.Time
	names map[string]string
}

var attachmentIndexes sync.Map

// ResolveImageSource handles document-relative, vault-relative and bare wiki
// attachment names. The fallback catalog holds paths only, never image pixels.
func ResolveImageSource(src, base, root string) string {
	if isRemoteImage(src) || strings.HasPrefix(src, "data:") {
		return src
	}
	local := resolveImagePath(src, base)
	if abs, err := filepath.Abs(local); err == nil {
		local = abs
	}
	if st, err := os.Stat(local); err == nil && !st.IsDir() {
		return local
	}
	if root == "" {
		return local
	}
	rel := resolveImagePath(src, root)
	if st, err := os.Stat(rel); err == nil && !st.IsDir() {
		return rel
	}
	if strings.ContainsAny(src, "/\\") {
		return local
	}
	name := strings.ToLower(filepath.Base(local))
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
	return local
}
