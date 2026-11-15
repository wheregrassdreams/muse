package search

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	tantivy "github.com/anyproto/tantivy-go"
)

// Literal deliberately does not use token candidates: punctuation, short CJK
// queries and matches crossing token boundaries must never disappear.
func (i *Index) literal(ctx context.Context, query string, limit int, overlays ...Document) ([]Hit, error) {
	query = strings.TrimSpace(query)
	if limit <= 0 || query == "" {
		return nil, nil
	}
	if !utf8.ValidString(query) {
		return nil, errors.New("search: query is not valid UTF-8")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s, err := i.getState()
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, errors.New("search: index is closed")
	}
	if s.poisoned {
		return nil, errors.New("search: index transaction is incomplete; close and reopen to rebuild")
	}
	if s.literalReady {
		return literalCachedLocked(ctx, s, query, limit, overlays)
	}
	shadow := make(map[string]Document, len(overlays))
	for _, d := range overlays {
		if err := validateDocument(d); err != nil {
			return nil, err
		}
		if _, duplicate := shadow[d.ID]; duplicate {
			return nil, fmt.Errorf("search: duplicate overlay ID %q", d.ID)
		}
		shadow[d.ID] = d
	}
	type item struct{ id, path string }
	items := make([]item, 0, len(s.manifest.Docs)+len(shadow))
	for id, rec := range s.manifest.Docs {
		if _, replaced := shadow[id]; !replaced {
			items = append(items, item{id, rec.Path})
		}
	}
	for id, d := range shadow {
		items = append(items, item{id, d.Path})
	}
	sort.Slice(items, func(a, b int) bool {
		if items[a].path != items[b].path {
			return items[a].path < items[b].path
		}
		return items[a].id < items[b].id
	})
	needle := strings.ToLower(query)
	out := make([]Hit, 0, min(limit, 512))
	for at := 0; at < len(items) && len(out) < limit; at += literalBatchSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch := items[at:min(at+literalBatchSize, len(items))]
		qb := tantivy.NewQueryBuilder()
		ids := qb.NestedBuilder()
		nativeCount := 0
		for _, entry := range batch {
			if _, volatile := shadow[entry.id]; !volatile {
				ids.Query(tantivy.Should, fieldID, entry.id, tantivy.TermQuery, 1)
				nativeCount++
			}
		}
		stored := make(map[string]storedHit, nativeCount)
		if nativeCount > 0 {
			qb.BooleanQuery(tantivy.Must, ids, 1)
			result, err := s.engine.SearchJson(tantivy.NewSearchContextBuilder().
				SetQueryFromJson(ptrQuery(qb.Build())).SetDocsLimit(uintptr(nativeCount)).Build())
			if err != nil {
				return nil, fmt.Errorf("search: read literal snapshots: %w", err)
			}
			docs, readErr := readSearchHits(result, s.engine)
			result.Free()
			if readErr != nil {
				return nil, readErr
			}
			for _, d := range docs {
				stored[d.ID] = d
			}
		}
		for _, entry := range batch {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			d, volatile := shadow[entry.id]
			if !volatile {
				v, ok := stored[entry.id]
				if !ok {
					return nil, fmt.Errorf("search: missing indexed snapshot %q", entry.id)
				}
				d = Document{ID: v.ID, Path: v.Path, Source: v.Source, Revision: v.Revision}
			}
			folded, starts, ends := lowercaseOffsets(d.Source)
			for from := 0; from < len(folded) && len(out) < limit; {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				n := strings.Index(folded[from:], needle)
				if n < 0 {
					break
				}
				a, z := from+n, from+n+len(needle)
				start, end := a, z
				if starts != nil {
					start, end = starts[a], ends[z-1]
				}
				out = append(out, Hit{Path: d.Path, Revision: d.Revision, Start: start, End: end,
					Score: 1, Snippet: makeSnippet(d.Source, start, end)})
				from = z
			}
		}
	}
	return out, nil
}

// Go's Unicode lowercase is the pre-existing literal-search contract. EqualFold
// would change it (e.g. final sigma), and folded byte positions can differ from
// source positions (e.g. Turkish capital dotted I).
func lowercaseOffsets(source string) (string, []int, []int) {
	lower := strings.ToLower(source)
	lengthChanges := false
	for _, r := range source {
		if utf8.RuneLen(r) != utf8.RuneLen(unicode.ToLower(r)) {
			lengthChanges = true
			break
		}
	}
	if !lengthChanges {
		return lower, nil, nil
	}
	starts := make([]int, 0, len(lower))
	ends := make([]int, 0, len(lower))
	for at, r := range source {
		for n := 0; n < utf8.RuneLen(unicode.ToLower(r)); n++ {
			starts = append(starts, at)
			ends = append(ends, at+utf8.RuneLen(r))
		}
	}
	return lower, starts, ends
}

func findFold(source, query string) (int, int) {
	if query == "" {
		return -1, -1
	}
	folded, starts, ends := lowercaseOffsets(source)
	needle := strings.ToLower(query)
	a := strings.Index(folded, needle)
	if a < 0 {
		return -1, -1
	}
	z := a + len(needle)
	if starts != nil {
		return starts[a], ends[z-1]
	}
	return a, z
}
