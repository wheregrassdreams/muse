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
	files    map[string]struct{}         // slash-separated vault-relative paths
	dirs     map[string]struct{}         // slash-separated vault-relative directory paths
	children map[string]map[string]Entry // immediate children keyed by vault-relative directory
	images   map[string]map[string]struct{}
	revision uint64
}

func New(source Source) *Catalog {
	root := ""
	if source != nil {
		root = source.Root()
	}
	c := &Catalog{
		source: source, root: cleanRoot(root), files: make(map[string]struct{}),
		dirs: make(map[string]struct{}), children: make(map[string]map[string]Entry),
		images: make(map[string]map[string]struct{}),
	}
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
	nextDirs := make(map[string]struct{})
	nextChildren := make(map[string]map[string]Entry)
	err := source.Walk(func(entry Entry) error {
		rel, err := validateEntryPath(entry.Path)
		if err != nil {
			return err
		}
		if entry.Kind == EntryDirectory && strings.HasPrefix(path.Base(rel), ".") {
			return ErrSkipDir
		}
		if entry.Kind == EntryDirectory && !hasHiddenComponent(rel) {
			nextDirs[rel] = struct{}{}
			addChildEntry(nextChildren, Entry{Path: rel, Kind: EntryDirectory})
			return nil
		}
		if entry.Kind != EntryRegular || hasHiddenComponent(rel) {
			return nil
		}
		next[rel] = struct{}{}
		addChildEntry(nextChildren, Entry{Path: rel, Kind: EntryRegular})
		return nil
	})
	if err != nil {
		return err
	}
	c.mu.Lock()
	if !samePaths(c.files, next) || !samePaths(c.dirs, nextDirs) {
		c.files = next
		c.dirs = nextDirs
		c.children = nextChildren
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
	if statErr != nil || entry.Kind != EntryRegular && entry.Kind != EntryDirectory {
		valid = false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	changed := false
	_, wasFile := c.files[key]
	_, wasDir := c.dirs[key]
	switch {
	case !valid:
		changed = c.removeTreeLocked(key)
	case entry.Kind == EntryRegular:
		if wasDir {
			changed = c.removeTreeLocked(key)
		}
		if !wasFile {
			c.files[key] = struct{}{}
			c.addImageLocked(key)
			changed = true
		}
	case entry.Kind == EntryDirectory:
		if wasFile {
			changed = c.removeTreeLocked(key)
		}
		if !wasDir {
			c.dirs[key] = struct{}{}
			changed = true
		}
	}
	if changed && valid {
		addChildEntry(c.children, Entry{Path: key, Kind: entry.Kind})
	}
	if changed {
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
	changed := c.removeTreeLocked(key)
	if changed {
		c.revision++
	}
}

// Children returns the immediate catalog entries under a vault-relative
// directory. The empty path addresses the vault root. Paths in the result
// remain slash-separated and include empty directories.
func (c *Catalog) Children(directory string) []Entry {
	if c == nil {
		return nil
	}
	directory, ok := normalizeDirectory(directory)
	if !ok {
		return nil
	}
	c.mu.RLock()
	entries := make([]Entry, 0)
	for _, entry := range c.children[directory] {
		entries = append(entries, entry)
	}
	c.mu.RUnlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries
}

// Entry returns the catalog kind for a tracked vault-relative path.
func (c *Catalog) Entry(pathValue string) (Entry, bool) {
	if c == nil {
		return Entry{}, false
	}
	rel, err := validateEntryPath(pathValue)
	if err != nil || hasHiddenComponent(rel) {
		return Entry{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	parent := path.Dir(rel)
	if parent == "." {
		parent = ""
	}
	entry, ok := c.children[parent][path.Base(rel)]
	return entry, ok
}

func normalizeDirectory(value string) (string, bool) {
	if value == "" || value == "." {
		return "", true
	}
	clean := path.Clean(value)
	if strings.Contains(value, "\\") || path.IsAbs(value) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

func addChildEntry(children map[string]map[string]Entry, entry Entry) {
	parent := path.Dir(entry.Path)
	if parent == "." {
		parent = ""
	}
	if children[parent] == nil {
		children[parent] = make(map[string]Entry)
	}
	children[parent][path.Base(entry.Path)] = entry
}

func (c *Catalog) removeTreeLocked(root string) bool {
	changed := false
	for file := range c.files {
		if file == root || strings.HasPrefix(file, root+"/") {
			delete(c.files, file)
			c.removeImageLocked(file)
			changed = true
		}
	}
	for dir := range c.dirs {
		if dir == root || strings.HasPrefix(dir, root+"/") {
			delete(c.dirs, dir)
			changed = true
		}
	}
	for parent := range c.children {
		if parent == root || strings.HasPrefix(parent, root+"/") {
			delete(c.children, parent)
		}
	}
	parent := path.Dir(root)
	if parent == "." {
		parent = ""
	}
	if siblings := c.children[parent]; siblings != nil {
		if _, exists := siblings[path.Base(root)]; exists {
			delete(siblings, path.Base(root))
			changed = true
		}
	}
	return changed
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
