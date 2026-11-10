// Package vaultindex provides a small, in-memory Markdown workspace index.
// Disk access happens only in Rebuild and Update; queries never walk the tree.
package vaultindex

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Heading records an ATX heading and its GitHub-style anchor.
type Heading struct {
	Text, Slug string
	Offset     int
}

// Link is a source link. Target retains the authored destination; Kind is
// "wiki" or "markdown". Offsets are byte offsets in the source document.
type Link struct {
	Source, Target, Label, Kind string
	Start, End                  int
}

// Document is an indexed Markdown file. Source and SearchText are snapshots;
// callers must treat their contents as immutable.
type Document struct {
	Path, Title, Source, SearchText string
	Aliases, Tags                   []string
	Headings                        []Heading
	Blocks                          []BlockID
	Links                           []Link
}

// BlockID records an Obsidian-style trailing ^block identifier.
type BlockID struct {
	ID     string
	Offset int
}

// Match is a case-insensitive search hit with byte offsets into the source.
type Match struct {
	Path       string
	Start, End int
	Snippet    string
}

// Location resolves a link target to a file and byte offset. Offset is zero
// for a file-only target, and points to a matching heading/block otherwise.
type Location struct {
	Path   string
	Offset int
}

// Index stores a workspace snapshot. Its zero value is usable after Rebuild.
type Index struct {
	mu   sync.RWMutex
	root string
	docs map[string]*Document
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
		return fmt.Errorf("vaultindex: root is not a directory: %s", abs)
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
		return Document{}, fmt.Errorf("vaultindex: unsupported file type: %s", rel)
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
	i.docs[copy.Path] = &copy
	i.mu.Unlock()
}

// Snapshot returns copies of all indexed documents in path order.
func (i *Index) Snapshot() []Document {
	i.mu.RLock()
	defer i.mu.RUnlock()
	paths := sortedPaths(i.docs)
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
	d.Headings = append([]Heading(nil), d.Headings...)
	d.Blocks = append([]BlockID(nil), d.Blocks...)
	d.Links = append([]Link(nil), d.Links...)
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
		return fmt.Errorf("vaultindex: unsupported file type: %s", rel)
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
	for indexed := range i.docs {
		if indexed == rel || rel == "" || strings.HasPrefix(indexed, rel+"/") {
			delete(i.docs, indexed)
		}
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
	paths := sortedPaths(i.docs)
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
func (i *Index) Resolve(current, target string) (Location, bool) {
	target = strings.TrimSpace(strings.Trim(target, "<>"))
	if target == "" {
		return Location{}, false
	}
	filePart, anchor, block := splitTarget(target)
	i.mu.RLock()
	defer i.mu.RUnlock()
	paths := sortedPaths(i.docs)
	var candidates []string
	if filePart == "" {
		candidates = append(candidates, cleanPath(current))
	} else {
		name := filepath.FromSlash(filePart)
		if filepath.Ext(name) == "" {
			name += ".md"
		}
		cur := filepath.Dir(cleanPath(current))
		for _, base := range []string{cur, "."} {
			p := cleanPath(filepath.Join(base, name))
			if _, ok := i.docs[p]; ok {
				candidates = appendUnique(candidates, p)
			}
		}
		want := strings.TrimSuffix(strings.ToLower(filepath.Base(name)), strings.ToLower(filepath.Ext(name)))
		for _, p := range paths {
			d := i.docs[p]
			if strings.EqualFold(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)), want) || equalAny(d.Title, want) || hasAnyFold(d.Aliases, want) {
				candidates = appendUnique(candidates, p)
			}
		}
	}
	if len(candidates) == 0 && filePart != "" { // Also allow a bare title/alias with no matching filename.
		for _, p := range paths {
			d := i.docs[p]
			if equalAny(d.Title, filePart) || hasAnyFold(d.Aliases, filePart) {
				candidates = append(candidates, p)
			}
		}
	}
	if len(candidates) == 0 {
		return Location{}, false
	}
	chosen := candidates[0]
	d := i.docs[chosen]
	off := 0
	if block != "" {
		for _, b := range d.Blocks {
			if strings.EqualFold(b.ID, block) {
				off = b.Offset
				return Location{chosen, off}, true
			}
		}
		return Location{}, false
	}
	if anchor != "" {
		slug := slugify(anchor)
		for _, h := range d.Headings {
			if strings.EqualFold(h.Slug, slug) || strings.EqualFold(h.Text, anchor) {
				off = h.Offset
				return Location{chosen, off}, true
			}
		}
		return Location{}, false
	}
	return Location{Path: chosen, Offset: off}, true
}

// Outgoing returns links authored by source, in source order.
func (i *Index) Outgoing(source string) []Link {
	_, rel, err := i.workspacePath(source)
	if err != nil {
		return nil
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	d := i.docs[rel]
	if d == nil {
		return nil
	}
	return append([]Link(nil), d.Links...)
}

// Backlinks returns links resolving to target, in source path and source order.
func (i *Index) Backlinks(target string) []Link {
	i.mu.RLock()
	defer i.mu.RUnlock()
	var out []Link
	for _, p := range sortedPaths(i.docs) {
		for _, l := range i.docs[p].Links {
			if resolvesLocked(i, p, l.Target, target) {
				out = append(out, l)
			}
		}
	}
	return out
}

func (i *Index) workspacePath(path string) (string, string, error) {
	i.mu.RLock()
	root := i.root
	i.mu.RUnlock()
	if root == "" {
		return "", "", errors.New("vaultindex: call Rebuild before indexing paths")
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
		return "", "", fmt.Errorf("vaultindex: path escapes workspace: %s", path)
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
func appendUnique(a []string, s string) []string {
	for _, x := range a {
		if x == s {
			return a
		}
	}
	return append(a, s)
}
func equalAny(a, b string) bool { return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) }
func hasAnyFold(a []string, b string) bool {
	for _, s := range a {
		if equalAny(s, b) {
			return true
		}
	}
	return false
}
func splitTarget(s string) (file, heading, block string) {
	if p := strings.IndexByte(s, '#'); p >= 0 {
		file, heading = s[:p], strings.TrimSpace(s[p+1:])
		if strings.HasPrefix(heading, "^") {
			block, heading = strings.TrimSpace(strings.TrimPrefix(heading, "^")), ""
		}
		return
	}
	if p := strings.LastIndex(s, "^"); p >= 0 {
		file, block = s[:p], strings.TrimSpace(s[p+1:])
		return
	}
	return s, "", ""
}
func resolvesLocked(i *Index, current, linkTarget, wanted string) bool {
	loc, ok := resolveLocked(i, current, linkTarget)
	if !ok {
		return false
	}
	w, ok := resolveLocked(i, current, wanted)
	if !ok || w.Path != loc.Path {
		return false
	}
	_, wantHeading, wantBlock := splitTarget(wanted)
	if wantBlock != "" {
		_, linkBlock, _ := splitTarget(linkTarget)
		return strings.EqualFold(linkBlock, wantBlock)
	}
	if wantHeading != "" {
		_, linkHeading, _ := splitTarget(linkTarget)
		return strings.EqualFold(slugify(linkHeading), slugify(wantHeading))
	}
	return true
}
func resolveLocked(i *Index, current, target string) (Location, bool) { // lock is already held by Backlinks
	file, h, b := splitTarget(target)
	candidates := []string{}
	if file == "" {
		candidates = append(candidates, cleanPath(current))
	} else {
		n := filepath.FromSlash(file)
		if filepath.Ext(n) == "" {
			n += ".md"
		}
		for _, base := range []string{filepath.Dir(cleanPath(current)), "."} {
			p := cleanPath(filepath.Join(base, n))
			if i.docs[p] != nil {
				candidates = appendUnique(candidates, p)
			}
		}
		if len(candidates) == 0 {
			want := strings.TrimSuffix(filepath.Base(n), filepath.Ext(n))
			for _, p := range sortedPaths(i.docs) {
				d := i.docs[p]
				if equalAny(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)), want) || equalAny(d.Title, want) || hasAnyFold(d.Aliases, want) {
					candidates = appendUnique(candidates, p)
				}
			}
		}
	}
	if len(candidates) == 0 {
		return Location{}, false
	}
	d := i.docs[candidates[0]]
	off := 0
	if b != "" {
		for _, x := range d.Blocks {
			if strings.EqualFold(x.ID, b) {
				return Location{candidates[0], x.Offset}, true
			}
		}
		return Location{}, false
	}
	if h != "" {
		for _, x := range d.Headings {
			if strings.EqualFold(x.Slug, slugify(h)) || equalAny(x.Text, h) {
				return Location{candidates[0], x.Offset}, true
			}
		}
		return Location{}, false
	}
	return Location{candidates[0], off}, true
}
