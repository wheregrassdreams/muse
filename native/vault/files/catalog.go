// Package files maintains path-only metadata for files in a vault.
// It deliberately does not read or retain file contents.
package files

import (
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Catalog is a deterministic, incrementally maintained inventory of regular
// files below one vault root. Hidden directories and symlinks are excluded.
type Catalog struct {
	mu       sync.RWMutex
	source   Source
	root     string
	files    map[string]struct{} // slash-separated vault-relative paths
	images   map[string]map[string]struct{}
	revision uint64
}

func New(source Source) *Catalog {
	root := ""
	if source != nil {
		root = source.Root()
	}
	c := &Catalog{source: source, root: cleanRoot(root), files: make(map[string]struct{}), images: make(map[string]map[string]struct{})}
	return c
}

func cleanRoot(root string) string {
	abs, err := filepath.Abs(root)
	if err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(root)
}

// Rebuild performs the initial inventory or a recovery scan after watcher
// overflow. Normal filesystem events should use Update and Remove.
func (c *Catalog) Rebuild() error {
	c.mu.RLock()
	source := c.source
	c.mu.RUnlock()
	if err := validateSource(source); err != nil {
		return err
	}
	next := make(map[string]struct{})
	err := source.Walk(func(entry Entry) error {
		rel, err := validateEntryPath(entry.Path)
		if err != nil {
			return err
		}
		if entry.Kind == EntryDirectory && strings.HasPrefix(path.Base(rel), ".") {
			return ErrSkipDir
		}
		if entry.Kind != EntryRegular || hasHiddenComponent(rel) {
			return nil
		}
		next[rel] = struct{}{}
		return nil
	})
	if err != nil {
		return err
	}
	c.mu.Lock()
	if !samePaths(c.files, next) {
		c.files = next
		c.revision++
	}
	c.images = imageIndex(next)
	c.mu.Unlock()
	return nil
}

func samePaths(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for p := range a {
		if _, ok := b[p]; !ok {
			return false
		}
	}
	return true
}

func imageIndex(files map[string]struct{}) map[string]map[string]struct{} {
	idx := make(map[string]map[string]struct{})
	for p := range files {
		if !IsImage(p) {
			continue
		}
		key := strings.ToLower(filepath.Base(filepath.FromSlash(p)))
		if idx[key] == nil {
			idx[key] = make(map[string]struct{})
		}
		idx[key][p] = struct{}{}
	}
	return idx
}

func (c *Catalog) addImageLocked(path string) {
	if !IsImage(path) {
		return
	}
	key := strings.ToLower(filepath.Base(filepath.FromSlash(path)))
	if c.images[key] == nil {
		c.images[key] = make(map[string]struct{})
	}
	c.images[key][path] = struct{}{}
}

func (c *Catalog) removeImageLocked(path string) {
	if !IsImage(path) {
		return
	}
	key := strings.ToLower(filepath.Base(filepath.FromSlash(path)))
	delete(c.images[key], path)
	if len(c.images[key]) == 0 {
		delete(c.images, key)
	}
}

// Update refreshes one changed path. It ignores directories, hidden paths,
// symlinks, and paths outside the vault.
func (c *Catalog) Update(path string) {
	c.mu.RLock()
	root, source := c.root, c.source
	c.mu.RUnlock()
	if validateSource(source) != nil {
		return
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return
	}
	key := filepath.ToSlash(rel)
	valid := !hasHiddenComponent(rel)
	entry, statErr := source.Stat(key)
	if statErr != nil || entry.Kind != EntryRegular {
		valid = false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, had := c.files[key]
	if valid && !had {
		c.files[key] = struct{}{}
		c.addImageLocked(key)
		c.revision++
	}
	if !valid && had {
		delete(c.files, key)
		c.removeImageLocked(key)
		c.revision++
	}
}

// Remove removes a path and any descendants, allowing a single directory
// removal event to update the inventory without rescanning the vault.
func (c *Catalog) Remove(path string) {
	c.mu.RLock()
	root := c.root
	c.mu.RUnlock()
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return
	}
	key := filepath.ToSlash(rel)
	c.mu.Lock()
	defer c.mu.Unlock()
	changed := false
	for p := range c.files {
		if p == key || strings.HasPrefix(p, key+"/") {
			delete(c.files, p)
			c.removeImageLocked(p)
			changed = true
		}
	}
	if changed {
		c.revision++
	}
}

func hasHiddenComponent(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

// Revision changes whenever the catalog contents change.
func (c *Catalog) Revision() uint64 { c.mu.RLock(); defer c.mu.RUnlock(); return c.revision }

// Paths returns all catalogued files in stable vault-relative order.
func (c *Catalog) Paths() []string {
	c.mu.RLock()
	out := make([]string, 0, len(c.files))
	for p := range c.files {
		out = append(out, p)
	}
	c.mu.RUnlock()
	sort.Strings(out)
	return out
}

// ImagePaths returns supported image paths in stable vault-relative order.
func (c *Catalog) ImagePaths() []string {
	paths := c.Paths()
	out := paths[:0]
	for _, p := range paths {
		if IsImage(p) {
			out = append(out, p)
		}
	}
	return out
}

// IsImage reports whether path has an extension supported by image rendering.
func IsImage(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg", ".avif":
		return true
	default:
		return false
	}
}

// FindImage returns the deterministic best match for a bare attachment name:
// shortest vault-relative path, then lexical order.
func (c *Catalog) FindImage(name string) string {
	name = strings.ToLower(filepath.Base(filepath.FromSlash(name)))
	if name == "" || name == "." {
		return ""
	}
	c.mu.RLock()
	paths := make([]string, 0, len(c.images[name]))
	for p := range c.images[name] {
		paths = append(paths, p)
	}
	root := c.root
	c.mu.RUnlock()
	sort.Strings(paths)
	best := ""
	for _, p := range paths {
		if best == "" || len(p) < len(best) || len(p) == len(best) && p < best {
			best = p
		}
	}
	if best == "" {
		return ""
	}
	return filepath.Join(root, filepath.FromSlash(best))
}
