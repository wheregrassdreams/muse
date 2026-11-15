// Package links owns vault path/title/alias resolution and the derived outgoing
// and backlink graph. It consumes narrow immutable document projections and
// has no dependency on vault/index storage or presentation packages.
package links

import (
	"errors"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"
)

const backlinksCacheLimit = 256

type Heading struct {
	Text, Slug string
	Offset     int
}
type BlockID struct {
	ID     string
	Offset int
}
type Link struct {
	Source, Target, Label, Kind string
	Start, End                  int
}
type Location struct {
	Path   string
	Offset int
}
type BacklinkSource struct {
	Source       string
	Start, Count int
}

// Document is the minimum immutable projection needed to resolve note links.
type Document struct {
	Path, Title string
	Aliases     []string
	Headings    []Heading
	Blocks      []BlockID
	Links       []Link
}

type linkRef struct {
	source string
	index  int
}

type Index struct {
	mu                   sync.RWMutex
	root                 string
	docs                 map[string]*Document
	backlinksByName      map[string][]linkRef
	backlinksByPath      map[string][]linkRef
	docPathsByName       map[string][]string
	backlinksCache       map[string][]Link
	backlinkSourcesCache map[string][]BacklinkSource
	backlinksOrder       []string
	revision             uint64
}

func New() *Index { return &Index{docs: make(map[string]*Document)} }

// Rebuild replaces the graph from an immutable vault/index projection.
func (i *Index) Rebuild(root string, docs []Document) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if i == nil {
		return errors.New("vault/links: nil index")
	}
	next := make(map[string]*Document, len(docs))
	for _, input := range docs {
		path, err := i.cleanVaultPath(abs, input.Path)
		if err != nil {
			return err
		}
		if _, exists := next[path]; exists {
			return errors.New("vault/links: duplicate document path")
		}
		doc := cloneDocument(input)
		doc.Path = path
		next[path] = &doc
	}
	i.mu.Lock()
	i.root, i.docs = abs, next
	i.revision++
	i.rebuildGraphLocked()
	i.mu.Unlock()
	return nil
}

// ApplySnapshot updates one note projection and invalidates derived queries.
func (i *Index) ApplySnapshot(input Document) error {
	if i == nil {
		return errors.New("vault/links: nil index")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	path, err := i.cleanVaultPath(i.root, input.Path)
	if err != nil {
		return err
	}
	if i.root == "" {
		return errors.New("vault/links: rebuild must set vault root before snapshots")
	}
	if previous := i.docs[path]; previous != nil {
		i.unindexDocumentLocked(previous)
	}
	copy := cloneDocument(input)
	copy.Path = path
	i.docs[path] = &copy
	i.indexDocumentLocked(&copy)
	i.revision++
	i.invalidateCachesLocked()
	return nil
}

// Remove deletes a note projection or directory subtree.
func (i *Index) Remove(path string) {
	if i == nil {
		return
	}
	_, rel, err := i.workspacePath(path)
	if err != nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	removed := false
	for candidate := range i.docs {
		if candidate == rel || strings.HasPrefix(candidate, rel+"/") {
			i.unindexDocumentLocked(i.docs[candidate])
			delete(i.docs, candidate)
			removed = true
		}
	}
	if removed {
		i.revision++
		i.invalidateCachesLocked()
	}
}

func (i *Index) Resolve(current, target string) (Location, bool) {
	target = strings.TrimSpace(strings.Trim(target, "<>"))
	if target == "" {
		return Location{}, false
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.resolveLocked(current, target)
}

func (i *Index) Outgoing(source string) []Link {
	_, rel, err := i.workspacePath(source)
	if err != nil {
		return nil
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	doc := i.docs[rel]
	if doc == nil {
		return nil
	}
	return append([]Link(nil), doc.Links...)
}

func (i *Index) Backlinks(target string) []Link {
	if i == nil {
		return nil
	}
	if !isMarkdownTarget(target) {
		return i.resourceBacklinks(target)
	}
	i.mu.RLock()
	if cached, ok := i.backlinksCache[target]; ok {
		out := append([]Link(nil), cached...)
		i.mu.RUnlock()
		return out
	}
	i.mu.RUnlock()
	i.mu.Lock()
	defer i.mu.Unlock()
	if cached, ok := i.backlinksCache[target]; ok {
		return append([]Link(nil), cached...)
	}
	file, _, _ := splitTarget(target)
	lookup := noteName(file)
	keys := map[string]bool{foldKey(lookup): true}
	for _, path := range i.docPathsByName[foldKey(lookup)] {
		if doc := i.docs[path]; doc != nil {
			keys[foldKey(noteName(doc.Path))] = true
			keys[foldKey(doc.Title)] = true
			for _, alias := range doc.Aliases {
				keys[foldKey(alias)] = true
			}
		}
	}
	var refs []linkRef
	if file == "" {
		for _, candidates := range i.backlinksByPath {
			refs = append(refs, candidates...)
		}
	} else {
		for key := range keys {
			refs = append(refs, i.backlinksByName[key]...)
		}
	}
	var out []Link
	for _, ref := range refs {
		doc := i.docs[ref.source]
		if doc != nil && ref.index >= 0 && ref.index < len(doc.Links) {
			link := doc.Links[ref.index]
			if i.resolvesLocked(ref.source, link.Target, target) {
				out = append(out, link)
			}
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Source != out[b].Source {
			return out[a].Source < out[b].Source
		}
		return out[a].Start < out[b].Start
	})
	if len(i.backlinksOrder) >= backlinksCacheLimit {
		oldest := i.backlinksOrder[0]
		delete(i.backlinksCache, oldest)
		delete(i.backlinkSourcesCache, oldest)
		copy(i.backlinksOrder, i.backlinksOrder[1:])
		i.backlinksOrder = i.backlinksOrder[:len(i.backlinksOrder)-1]
	}
	i.backlinksCache[target] = append([]Link(nil), out...)
	i.backlinksOrder = append(i.backlinksOrder, target)
	return out
}

func (i *Index) BacklinkSources(target string) []BacklinkSource {
	if i == nil {
		return nil
	}
	for {
		i.mu.RLock()
		if cached, ok := i.backlinkSourcesCache[target]; ok {
			out := append([]BacklinkSource(nil), cached...)
			i.mu.RUnlock()
			return out
		}
		revision := i.revision
		i.mu.RUnlock()
		links := i.Backlinks(target)
		out := make([]BacklinkSource, 0, len(links))
		for _, link := range links {
			if n := len(out); n > 0 && out[n-1].Source == link.Source {
				out[n-1].Count++
				continue
			}
			out = append(out, BacklinkSource{Source: link.Source, Start: link.Start, Count: 1})
		}
		i.mu.Lock()
		if i.revision != revision {
			i.mu.Unlock()
			continue
		}
		if cached, ok := i.backlinkSourcesCache[target]; ok {
			out = cached
		} else if i.backlinkSourcesCache != nil {
			i.backlinkSourcesCache[target] = out
		}
		i.mu.Unlock()
		return append([]BacklinkSource(nil), out...)
	}
}

func (i *Index) rebuildGraphLocked() {
	i.backlinksByName = make(map[string][]linkRef)
	i.backlinksByPath = make(map[string][]linkRef)
	i.docPathsByName = make(map[string][]string)
	i.invalidateCachesLocked()
	for _, doc := range i.docs {
		i.indexDocumentLocked(doc)
	}
}

func (i *Index) invalidateCachesLocked() {
	i.backlinksCache = make(map[string][]Link)
	i.backlinkSourcesCache = make(map[string][]BacklinkSource)
	i.backlinksOrder = nil
}

func (i *Index) indexDocumentLocked(doc *Document) {
	if doc == nil {
		return
	}
	path := doc.Path
	names := make(map[string]struct{})
	addName := func(name string) {
		key := foldKey(name)
		if key == "" {
			return
		}
		if _, exists := names[key]; exists {
			return
		}
		names[key] = struct{}{}
		paths := i.docPathsByName[key]
		at := sort.SearchStrings(paths, path)
		if at == len(paths) || paths[at] != path {
			paths = append(paths, "")
			copy(paths[at+1:], paths[at:])
			paths[at] = path
			i.docPathsByName[key] = paths
		}
	}
	addName(noteName(doc.Path))
	addName(doc.Title)
	for _, alias := range doc.Aliases {
		addName(alias)
	}
	for at, link := range doc.Links {
		file, _, _ := splitTarget(link.Target)
		ref := linkRef{source: path, index: at}
		if file == "" {
			i.backlinksByPath[path] = append(i.backlinksByPath[path], ref)
		} else if key := foldKey(noteName(file)); key != "" {
			i.backlinksByName[key] = append(i.backlinksByName[key], ref)
		}
	}
}

func (i *Index) unindexDocumentLocked(doc *Document) {
	if doc == nil {
		return
	}
	path := doc.Path
	removeRef := func(refs []linkRef, wanted linkRef) []linkRef {
		for n, ref := range refs {
			if ref == wanted {
				return append(refs[:n], refs[n+1:]...)
			}
		}
		return refs
	}
	for at, link := range doc.Links {
		file, _, _ := splitTarget(link.Target)
		ref := linkRef{source: path, index: at}
		if file == "" {
			i.backlinksByPath[path] = removeRef(i.backlinksByPath[path], ref)
			if len(i.backlinksByPath[path]) == 0 {
				delete(i.backlinksByPath, path)
			}
		} else if key := foldKey(noteName(file)); key != "" {
			i.backlinksByName[key] = removeRef(i.backlinksByName[key], ref)
			if len(i.backlinksByName[key]) == 0 {
				delete(i.backlinksByName, key)
			}
		}
	}
	names := map[string]struct{}{}
	removeName := func(name string) {
		key := foldKey(name)
		if key == "" {
			return
		}
		if _, exists := names[key]; exists {
			return
		}
		names[key] = struct{}{}
		paths := i.docPathsByName[key]
		for n, candidate := range paths {
			if candidate == path {
				i.docPathsByName[key] = append(paths[:n], paths[n+1:]...)
				break
			}
		}
		if len(i.docPathsByName[key]) == 0 {
			delete(i.docPathsByName, key)
		}
	}
	removeName(noteName(doc.Path))
	removeName(doc.Title)
	for _, alias := range doc.Aliases {
		removeName(alias)
	}
}

func (i *Index) resolveLocked(current, target string) (Location, bool) {
	file, heading, block := splitTarget(target)
	var candidates []string
	if file == "" {
		candidates = append(candidates, cleanPath(current))
	} else {
		name := filepath.FromSlash(file)
		if filepath.Ext(name) == "" {
			name += ".md"
		}
		for _, base := range []string{filepath.Dir(cleanPath(current)), "."} {
			path := cleanPath(filepath.Join(base, name))
			if i.docs[path] != nil {
				candidates = appendUnique(candidates, path)
			}
		}
		want := strings.TrimSuffix(strings.ToLower(filepath.Base(name)), strings.ToLower(filepath.Ext(name)))
		candidates = i.appendResolveMatches(candidates, i.docPathsByName[foldKey(want)], want)
	}
	if len(candidates) == 0 && file != "" {
		for _, path := range i.docPathsByName[foldKey(file)] {
			doc := i.docs[path]
			if doc != nil && (equalAny(doc.Title, file) || hasAnyFold(doc.Aliases, file)) {
				candidates = append(candidates, path)
			}
		}
	}
	if len(candidates) == 0 {
		return Location{}, false
	}
	path := candidates[0]
	doc := i.docs[path]
	if block != "" {
		for _, b := range doc.Blocks {
			if strings.EqualFold(b.ID, block) {
				return Location{path, b.Offset}, true
			}
		}
		return Location{}, false
	}
	if heading != "" {
		slug := Slugify(heading)
		for _, h := range doc.Headings {
			if strings.EqualFold(h.Slug, slug) || strings.EqualFold(h.Text, heading) {
				return Location{path, h.Offset}, true
			}
		}
		return Location{}, false
	}
	return Location{Path: path}, true
}

func (i *Index) appendResolveMatches(dst, paths []string, want string) []string {
	for _, path := range paths {
		doc := i.docs[path]
		if doc != nil && (strings.EqualFold(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), want) || equalAny(doc.Title, want) || hasAnyFold(doc.Aliases, want)) {
			dst = appendUnique(dst, path)
		}
	}
	return dst
}
func (i *Index) resolvesLocked(current, linkTarget, wanted string) bool {
	loc, ok := i.resolveLocked(current, linkTarget)
	if !ok {
		return false
	}
	target, ok := i.resolveLocked(current, wanted)
	if !ok || target.Path != loc.Path {
		return false
	}
	_, heading, block := splitTarget(wanted)
	if block != "" {
		_, linkBlock, _ := splitTarget(linkTarget)
		return strings.EqualFold(linkBlock, block)
	}
	if heading != "" {
		_, linkHeading, _ := splitTarget(linkTarget)
		return strings.EqualFold(Slugify(linkHeading), Slugify(heading))
	}
	return true
}

func (i *Index) resourceBacklinks(target string) []Link {
	_, wanted, err := i.workspacePath(target)
	if err != nil {
		return nil
	}
	wanted = cleanPath(wanted)
	i.mu.RLock()
	defer i.mu.RUnlock()
	var out []Link
	for _, doc := range i.docs {
		for _, link := range doc.Links {
			if resourceLinkResolves(i.root, doc.Path, link.Target, wanted) {
				out = append(out, link)
			}
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Source != out[b].Source {
			return out[a].Source < out[b].Source
		}
		return out[a].Start < out[b].Start
	})
	return out
}
func (i *Index) workspacePath(path string) (string, string, error) {
	i.mu.RLock()
	root := i.root
	i.mu.RUnlock()
	return workspacePath(root, path)
}
func workspacePath(root, path string) (string, string, error) {
	abs, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(path)))
	if filepath.IsAbs(path) {
		abs, err = filepath.Abs(path)
	}
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", errors.New("vault/links: path outside vault")
	}
	return abs, cleanPath(rel), nil
}
func (i *Index) cleanVaultPath(root, path string) (string, error) {
	_, rel, err := workspacePath(root, path)
	return rel, err
}

func cloneDocument(d Document) Document {
	d.Aliases = append([]string(nil), d.Aliases...)
	d.Headings = append([]Heading(nil), d.Headings...)
	d.Blocks = append([]BlockID(nil), d.Blocks...)
	d.Links = append([]Link(nil), d.Links...)
	return d
}
func isMarkdownTarget(target string) bool {
	file, _, _ := splitTarget(target)
	return isMarkdown(file) || filepath.Ext(file) == ""
}
func isMarkdown(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".mdx":
		return true
	}
	return false
}
func resourceLinkResolves(root, current, target, wanted string) bool {
	file, _, _ := splitTarget(target)
	file = strings.TrimSpace(file)
	if file == "" || strings.Contains(file, "://") || strings.HasPrefix(file, "data:") {
		return false
	}
	if decoded, err := url.PathUnescape(file); err == nil {
		file = decoded
	}
	file = filepath.FromSlash(file)
	var candidates []string
	if filepath.IsAbs(file) {
		if rel, err := filepath.Rel(root, filepath.Clean(file)); err == nil {
			candidates = append(candidates, rel)
		}
	} else {
		candidates = append(candidates, filepath.Join(filepath.Dir(filepath.FromSlash(current)), file), file)
	}
	for _, candidate := range candidates {
		if cleanPath(candidate) == wanted {
			return true
		}
	}
	return false
}
func noteName(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	base := filepath.Base(filepath.FromSlash(path))
	return strings.TrimSuffix(base, filepath.Ext(base))
}
func foldKey(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		min := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < min {
				min = next
			}
		}
		b.WriteRune(min)
	}
	return b.String()
}
func cleanPath(path string) string {
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if path == "." {
		return ""
	}
	return strings.TrimPrefix(path, "./")
}
func appendUnique(paths []string, path string) []string {
	for _, candidate := range paths {
		if candidate == path {
			return paths
		}
	}
	return append(paths, path)
}
func equalAny(a, b string) bool { return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) }
func hasAnyFold(values []string, wanted string) bool {
	for _, value := range values {
		if equalAny(value, wanted) {
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
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			b.WriteRune(r)
			dash = false
		} else if unicode.IsSpace(r) {
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
