package images

import (
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SourceOptions describes the filesystem boundary used to resolve a Markdown
// image reference. Lookup is consulted only for a bare attachment name.
type SourceOptions struct {
	BaseDir string
	Root    string
	Lookup  func(name string) string
}

// ResolveSource resolves a Markdown image reference without decoding it. A
// non-empty Root confines local paths (including existing symlinks) to that
// directory. Remote URLs and data URLs pass through unchanged.
func ResolveSource(src string, options SourceOptions) string {
	if isRemote(src) || strings.HasPrefix(src, "data:") {
		return src
	}
	local := resolvePath(src, options.BaseDir)
	if abs, err := filepath.Abs(local); err == nil {
		local = abs
	}
	if options.Root != "" && !pathWithinRoot(local, options.Root) {
		return ""
	}
	if st, err := os.Stat(local); err == nil && !st.IsDir() {
		return local
	}
	if options.Root == "" {
		return local
	}
	rel := resolvePath(src, options.Root)
	if !pathWithinRoot(rel, options.Root) {
		return ""
	}
	if st, err := os.Stat(rel); err == nil && !st.IsDir() {
		return rel
	}
	if strings.ContainsAny(src, "/\\") || options.Lookup == nil {
		return local
	}
	if p := options.Lookup(filepath.Base(local)); p != "" {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if pathWithinRoot(p, options.Root) {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	return local
}

// LoadSourceBytes resolves a local reference or fetches a remote/data source,
// then invokes complete with encoded bytes. Encoded bytes are only guaranteed
// to remain valid until complete returns; callers should decode or copy them
// there rather than retaining source buffers alongside pixels.
func LoadSourceBytes(src string, options SourceOptions, store CacheStore, complete func([]byte, error)) {
	if complete == nil {
		return
	}
	src = strings.TrimSpace(src)
	if isRemote(src) {
		SubmitRemote(func() { data, err := NewRemoteCache(store, ValidateImageData).Fetch(src); complete(data, err) })
		return
	}
	if strings.HasPrefix(src, "data:") {
		SubmitDecode(func() { data, err := dataURLBytes(src); complete(data, err) })
		return
	}
	path := ResolveSource(src, options)
	if path == "" {
		SubmitDecode(func() { complete(nil, os.ErrNotExist) })
		return
	}
	SubmitDecode(func() { data, err := os.ReadFile(path); complete(data, err) })
}

// ReadLocalSource synchronously resolves and reads a local source for APIs
// that require an immediate bitmap result. It rejects remote and data URLs;
// asynchronous document loading should use Loader.Load instead.
func ReadLocalSource(src string, options SourceOptions) ([]byte, error) {
	if strings.TrimSpace(src) == "" || isRemote(src) || strings.HasPrefix(src, "data:") {
		return nil, os.ErrInvalid
	}
	path := ResolveSource(src, options)
	if path == "" {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(path)
}

func resolvePath(src, base string) string {
	src = strings.TrimSpace(src)
	src = strings.TrimPrefix(src, "file://")
	if unescaped, err := url.PathUnescape(src); err == nil {
		src = unescaped
	}
	src = strings.ReplaceAll(src, "\\", string(filepath.Separator))
	src = filepath.Clean(src)
	if filepath.IsAbs(src) || base == "" {
		return src
	}
	return filepath.Join(base, src)
}

func isRemote(src string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(src)), "http://") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(src)), "https://")
}

type attachmentPaths struct {
	at    time.Time
	names map[string]string
}

var attachmentIndexes sync.Map

// FindAttachment is a bounded-time path catalog fallback for callers without
// a maintained vault attachment index. It stores paths only, never pixels.
func FindAttachment(root, name string) string {
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
	return idx.names[name]
}

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
