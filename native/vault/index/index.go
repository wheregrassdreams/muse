// Package index provides a small, in-memory Markdown workspace index.
// Disk access happens only in Rebuild and Update; queries never walk the tree.
package index

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"muse/document"
	"muse/document/types"
	"muse/vault/links"
)

// Document is an indexed Markdown file. Source and SearchText are snapshots;
// callers must treat their contents as immutable.
type Document struct {
	Path, Title, Source, SearchText string
	Revision                        document.Revision
	Type, CreatedAt, UpdatedAt      string
	Aliases, Tags                   []string
	Headings                        []links.Heading
	Blocks                          []links.BlockID
	Links                           []links.Link
}

// Metadata returns the stable document value projection used by search and
// other consumers. The index keeps these fields in its compact flat storage
// representation to avoid an extra object per indexed note.
func (d *Document) Metadata() types.Metadata {
	if d == nil {
		return types.Metadata{}
	}
	return types.Metadata{
		Type: types.NoteType(d.Type), CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
		Aliases: append([]string(nil), d.Aliases...), Tags: append([]string(nil), d.Tags...),
	}
}

// Match is a case-insensitive search hit with byte offsets into the source.
type Match struct {
	Path       string
	Start, End int
	Snippet    string
}

// Index stores a workspace snapshot. Its zero value is usable after Rebuild.
type Index struct {
	mu       sync.RWMutex
	pathsMu  sync.Mutex
	root     string
	docs     map[string]*Document
	paths    []string
	revision uint64
}

func (i *Index) invalidateDerivedLocked() {
	i.pathsMu.Lock()
	i.paths = nil
	i.pathsMu.Unlock()
}

func (i *Index) invalidatePathsLocked() {
	i.pathsMu.Lock()
	i.paths = nil
	i.pathsMu.Unlock()
}

func (i *Index) pathsLocked() []string {
	i.pathsMu.Lock()
	defer i.pathsMu.Unlock()
	if i.paths == nil {
		i.paths = sortedPaths(i.docs)
	}
	return i.paths
}

// New creates an empty index. Call Rebuild to attach a workspace root.
func New() *Index { return &Index{docs: make(map[string]*Document)} }

// Rebuild inventories Markdown recursively, independently of any UI tree.
// Hidden directories and symlinked directories are skipped.
func (i *Index) Rebuild(root string) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("vault/index: root is not a directory: %s", abs)
	}
	next := make(map[string]*Document)
	err = filepath.WalkDir(abs, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != abs && entry.IsDir() && strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !isMarkdown(path) {
			return nil
		}
		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return err
		}
		d, err := readDocument(path, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		next[d.Path] = d
		return nil
	})
	if err != nil {
		return err
	}
	i.mu.Lock()
	i.root, i.docs = abs, next
	i.invalidateDerivedLocked()
	i.revision++
	i.mu.Unlock()
	return nil
}

// Update reads and indexes one file below the workspace root. A missing file
// removes its former entry, which makes this suitable for watcher events.
func (i *Index) Update(path string) error {
	abs, rel, err := i.workspacePath(path)
	if err != nil {
		return err
	}
	st, err := os.Stat(abs)
	if errors.Is(err, os.ErrNotExist) {
		i.Remove(rel)
		return nil
	}
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || !isMarkdown(abs) {
		i.Remove(rel)
		return nil
	}
	source, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	d := parseDocument(rel, string(source))
	d.Revision = document.RevisionOf(d.Source)
	i.ApplySnapshot(*d)
	return nil
}

// ParseSnapshot parses source without reading or changing the Markdown file.
func (i *Index) ParseSnapshot(path, source string) (Document, error) {
	_, rel, err := i.workspacePath(path)
	if err != nil {
		return Document{}, err
	}
	if !isMarkdown(rel) {
		return Document{}, fmt.Errorf("vault/index: unsupported file type: %s", rel)
	}
	return *parseDocument(rel, source), nil
}

// ApplySnapshot replaces one document with a parsed source snapshot. Callers
// should pass confirmed disk content; editor buffers belong in query overlays.
func (i *Index) ApplySnapshot(d Document) {
	copy := cloneDocument(d)
	i.mu.Lock()
	if i.docs == nil {
		i.docs = make(map[string]*Document)
	}
	previous := i.docs[copy.Path]
	if copy.Revision == "" && previous != nil && previous.Source == copy.Source {
		copy.Revision = previous.Revision
	}
	if previous != nil && reflect.DeepEqual(*previous, copy) {
		i.mu.Unlock()
		return
	}
	i.docs[copy.Path] = &copy
	i.invalidatePathsLocked()
	i.revision++
	i.mu.Unlock()
}

// Revision returns the index-owned snapshot revision. It advances whenever
// Rebuild replaces the workspace snapshot, ApplySnapshot changes a document,
// or Remove deletes at least one document. Applying a byte-for-byte identical
// parsed Document is a no-op and does not advance the revision.
func (i *Index) Revision() uint64 {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.revision
}

// NotePaths returns vault-relative Markdown paths in lexical order. It copies
// only the path slice; callers that need document contents should use Document.
func (i *Index) NotePaths() []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return append([]string(nil), i.pathsLocked()...)
}

// NoteContent returns the title and source snapshot for one vault-relative
// note path. Strings are immutable, so this avoids copying unrelated document
// metadata such as headings, aliases, blocks, and links.
func (i *Index) NoteContent(path string) (title, source string, ok bool) {
	_, rel, err := i.workspacePath(path)
	if err != nil {
		return "", "", false
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	doc := i.docs[rel]
	if doc == nil {
		return "", "", false
	}
	return doc.Title, doc.Source, true
}

// Snapshot returns copies of all indexed documents in path order.
func (i *Index) Snapshot() []Document {
	i.mu.RLock()
	defer i.mu.RUnlock()
	paths := i.pathsLocked()
	out := make([]Document, 0, len(paths))
	for _, path := range paths {
		out = append(out, cloneDocument(*i.docs[path]))
	}
	return out
}

// Document returns a copy of one indexed document.
func (i *Index) Document(path string) (Document, bool) {
	_, rel, err := i.workspacePath(path)
	if err != nil {
		return Document{}, false
	}
	i.mu.RLock()
	d, ok := i.docs[rel]
	if !ok {
		i.mu.RUnlock()
		return Document{}, false
	}
	copy := cloneDocument(*d)
	i.mu.RUnlock()
	return copy, true
}

func cloneDocument(d Document) Document {
	d.Aliases = append([]string(nil), d.Aliases...)
	d.Tags = append([]string(nil), d.Tags...)
	d.Headings = append([]links.Heading(nil), d.Headings...)
	d.Blocks = append([]links.BlockID(nil), d.Blocks...)
	d.Links = append([]links.Link(nil), d.Links...)
	return d
}

// UpdateText overlays source text without touching disk. It is intended for
// the active editor buffer; a later watcher Update replaces it with disk text.
func (i *Index) UpdateText(path, source string) error {
	_, rel, err := i.workspacePath(path)
	if err != nil {
		return err
	}
	if !isMarkdown(rel) {
		return fmt.Errorf("vault/index: unsupported file type: %s", rel)
	}
	d := parseDocument(rel, source)
	i.ApplySnapshot(*d)
	return nil
}

// Remove drops a document or directory subtree from the snapshot. path may be
// absolute or root-relative; it need not still exist on disk.
func (i *Index) Remove(path string) {
	_, rel, err := i.workspacePath(path)
	if err != nil {
		return
	}
	i.mu.Lock()
	changed := false
	for indexed := range i.docs {
		if indexed == rel || rel == "" || strings.HasPrefix(indexed, rel+"/") {
			changed = true
			delete(i.docs, indexed)
		}
	}
	i.invalidatePathsLocked()
	if changed {
		i.revision++
	}
	i.mu.Unlock()
}

// Query returns all case-insensitive substring matches in path order. Empty
// queries return no results. Offsets refer to the original UTF-8 byte source.
func (i *Index) Query(query string) []Match {
	return i.query(query, 0)
}

// QueryLimit returns at most limit case-insensitive substring matches in path
// order. It stops scanning as soon as the limit is reached. A nonpositive
// limit returns no matches; use Query for an unbounded result set.
func (i *Index) QueryLimit(query string, limit int) []Match {
	if limit <= 0 {
		return nil
	}
	return i.query(query, limit)
}

func (i *Index) query(query string, limit int) []Match {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	paths := i.pathsLocked()
	needle, _, _ := foldWithOffsets(query)
	var out []Match
	for _, path := range paths {
		d := i.docs[path]
		hay, starts, ends := foldWithOffsets(d.SearchText)
		for from := 0; from < len(hay); {
			n := strings.Index(hay[from:], needle)
			if n < 0 {
				break
			}
			a := from + n
			z := a + len(needle)
			if z == 0 {
				break
			}
			start, end := a, z
			if starts != nil {
				if a >= len(starts) || z-1 >= len(ends) {
					break
				}
				start, end = starts[a], ends[z-1]
			}
			out = append(out, Match{Path: path, Start: start, End: end, Snippet: snippet(d.Source, start, end)})
			if limit > 0 && len(out) >= limit {
				return out
			}
			from = z
		}
	}
	return out
}

// foldWithOffsets applies Unicode lowercase while retaining a source byte
// interval for each byte in the folded string (lowercase can expand a rune).
func foldWithOffsets(s string) (string, []int, []int) {
	ascii := true
	for n := 0; n < len(s); n++ {
		if s[n] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return strings.ToLower(s), nil, nil
	}
	var folded strings.Builder
	var starts, ends []int
	for at, r := range s {
		piece := strings.ToLower(string(r))
		folded.WriteString(piece)
		for n := 0; n < len(piece); n++ {
			starts = append(starts, at)
			ends = append(ends, at+len(string(r)))
		}
	}
	return folded.String(), starts, ends
}

// Resolve finds a wiki or Markdown target by relative path, title, alias,
// heading, or block ID. Relative paths are tried from the current document
// directory before the workspace root. Unknown targets return false.
func (i *Index) workspacePath(path string) (string, string, error) {
	i.mu.RLock()
	root := i.root
	i.mu.RUnlock()
	if root == "" {
		return "", "", errors.New("vault/index: call Rebuild before indexing paths")
	}
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, abs)
	}
	abs, err := filepath.Abs(abs)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("vault/index: path escapes workspace: %s", path)
	}
	return abs, filepath.ToSlash(rel), nil
}
func sortedPaths(m map[string]*Document) []string {
	p := make([]string, 0, len(m))
	for k := range m {
		p = append(p, k)
	}
	sort.Strings(p)
	return p
}
func isMarkdown(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".md", ".markdown", ".mdx":
		return true
	}
	return false
}
func cleanPath(p string) string {
	p = filepath.ToSlash(filepath.Clean(filepath.FromSlash(p)))
	if p == "." {
		return ""
	}
	return strings.TrimPrefix(p, "./")
}
