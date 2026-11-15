package images

import (
	"errors"
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"
)

// LocalFiles is the filesystem capability consumed by image path resolution
// and loading. The platform adapter supplies the concrete implementation.
type LocalFiles interface {
	Stat(path string) (fs.FileInfo, error)
	ReadFile(path string) ([]byte, error)
	EvalSymlinks(path string) (string, error)
}

// ErrLocalFilesRequired is returned when a local image operation has no
// filesystem capability. Remote and data URL sources do not require one.
var ErrLocalFilesRequired = errors.New("markdown/images: local file capability is required")

// SourceOptions describes the filesystem boundary used to resolve a Markdown
// image reference. Lookup is consulted only for a bare attachment name.
type SourceOptions struct {
	BaseDir    string
	Root       string
	Lookup     func(name string) string
	LocalFiles LocalFiles
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
	if options.Root != "" {
		if options.LocalFiles == nil || !pathWithinRoot(local, options.Root, options.LocalFiles) {
			return ""
		}
	}
	if st, err := stat(options.LocalFiles, local); err == nil && st != nil && !st.IsDir() {
		return local
	}
	if options.Root == "" {
		return local
	}
	rel := resolvePath(src, options.Root)
	if !pathWithinRoot(rel, options.Root, options.LocalFiles) {
		return ""
	}
	if st, err := stat(options.LocalFiles, rel); err == nil && st != nil && !st.IsDir() {
		return rel
	}
	if strings.ContainsAny(src, "/\\") || options.Lookup == nil {
		return local
	}
	if p := options.Lookup(filepath.Base(local)); p != "" {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if pathWithinRoot(p, options.Root, options.LocalFiles) {
			if options.LocalFiles == nil {
				return p
			}
			if st, err := stat(options.LocalFiles, p); err == nil && st != nil && !st.IsDir() {
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
	if options.LocalFiles == nil {
		SubmitDecode(func() { complete(nil, ErrLocalFilesRequired) })
		return
	}
	path := ResolveSource(src, options)
	if path == "" {
		SubmitDecode(func() { complete(nil, fs.ErrNotExist) })
		return
	}
	SubmitDecode(func() { data, err := options.LocalFiles.ReadFile(path); complete(data, err) })
}

// ReadLocalSource synchronously resolves and reads a local source for APIs
// that require an immediate bitmap result. It rejects remote and data URLs;
// asynchronous document loading should use Loader.Load instead.
func ReadLocalSource(src string, options SourceOptions) ([]byte, error) {
	if strings.TrimSpace(src) == "" || isRemote(src) || strings.HasPrefix(src, "data:") {
		return nil, fs.ErrInvalid
	}
	if options.LocalFiles == nil {
		return nil, ErrLocalFilesRequired
	}
	path := ResolveSource(src, options)
	if path == "" {
		return nil, fs.ErrNotExist
	}
	return options.LocalFiles.ReadFile(path)
}

func stat(files LocalFiles, path string) (fs.FileInfo, error) {
	if files == nil {
		return nil, ErrLocalFilesRequired
	}
	return files.Stat(path)
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

func pathWithinRoot(path, root string, files LocalFiles) bool {
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
	if files == nil {
		return false
	}
	resolvedRoot, err := files.EvalSymlinks(absRoot)
	if err != nil {
		return false
	}
	resolvedPath, err := evalPathWithMissingSuffix(files, absPath)
	return err == nil && pathUnder(resolvedRoot, resolvedPath)
}

func evalPathWithMissingSuffix(files LocalFiles, path string) (string, error) {
	current := path
	var suffix []string
	for {
		resolved, err := files.EvalSymlinks(current)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func pathUnder(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
