package vault

import (
	"path/filepath"
	"strings"
	"sync"

	"muse/search"
)

// QuickOpen builds the source-agnostic candidate projection from the vault's
// path catalog and metadata index, then delegates matching and bounded query
// caching to search. It is scoped to one vault runtime.
type QuickOpen struct {
	mu          sync.Mutex
	root        string
	files       CatalogPort
	index       IndexPort
	service     *search.QuickOpenService
	filesRev    uint64
	indexRev    uint64
	generation  uint64
	hasSnapshot bool
}

type QuickOpenMatch struct {
	Path, PathText, Title string
	Score                 int
	TitleMatches          []int
	PathMatches           []int
}

func NewQuickOpen(root string, files CatalogPort, index IndexPort) *QuickOpen {
	return &QuickOpen{
		root:    filepath.Clean(root),
		files:   files,
		index:   index,
		service: search.NewQuickOpenService(32),
	}
}

func (q *QuickOpen) Search(query string) []QuickOpenMatch {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	var filesRevision, indexRevision uint64
	if q.files != nil {
		filesRevision = q.files.Revision()
	}
	if q.index != nil {
		indexRevision = q.index.Revision()
	}
	if !q.hasSnapshot || filesRevision != q.filesRev || indexRevision != q.indexRev {
		q.generation++
		q.service.ReplaceSnapshot(q.generation, q.candidates())
		q.filesRev, q.indexRev, q.hasSnapshot = filesRevision, indexRevision, true
	}
	matches := q.service.Search(query)
	out := make([]QuickOpenMatch, 0, len(matches))
	for _, match := range matches {
		out = append(out, QuickOpenMatch{
			Path: match.Path, PathText: quickOpenPathText(q.root, match.Path), Title: match.Title,
			Score: match.Score, TitleMatches: match.TitleMatches, PathMatches: match.PathMatches,
		})
	}
	return out
}

func quickOpenPathText(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	return filepath.ToSlash(rel)
}

func (q *QuickOpen) candidates() []search.QuickOpenItem {
	paths := []string(nil)
	if q.files != nil {
		paths = q.files.Paths()
	} else if q.index != nil {
		for _, doc := range q.index.SearchDocuments() {
			paths = append(paths, doc.Path)
		}
	}
	metadata := make(map[string]search.Document)
	if q.index != nil {
		for _, doc := range q.index.SearchDocuments() {
			metadata[filepath.ToSlash(filepath.Clean(doc.Path))] = doc
		}
	}
	items := make([]search.QuickOpenItem, 0, len(paths))
	for _, path := range paths {
		path = filepath.ToSlash(filepath.Clean(path))
		doc := metadata[path]
		title := strings.TrimSpace(doc.Title)
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		items = append(items, search.QuickOpenItem{
			Path:     filepath.Join(q.root, filepath.FromSlash(path)),
			PathText: path,
			Title:    title,
			Aliases:  append([]string(nil), doc.Metadata.Aliases...),
		})
	}
	return items
}
