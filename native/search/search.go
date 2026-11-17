package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	tantivy "github.com/anyproto/tantivy-go"
	"muse/search/rank"
)

type storedHit struct {
	ID       string  `json:"id"`
	Path     string  `json:"path"`
	Revision string  `json:"revision"`
	Source   string  `json:"source"`
	Score    float64 `json:"score"`
}

func (i *Index) search(ctx context.Context, query string, limit int, overlays ...Document) ([]Hit, error) {
	if limit <= 0 || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if !utf8.ValidString(query) {
		return nil, errors.New("search: query is not valid UTF-8")
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	shadowed := make(map[string]struct{}, len(overlays))
	for _, d := range overlays {
		if err := validateDocument(d); err != nil {
			return nil, err
		}
		if _, duplicate := shadowed[d.ID]; duplicate {
			return nil, fmt.Errorf("search: duplicate overlay ID %q", d.ID)
		}
		shadowed[d.ID] = struct{}{}
	}
	finalQuery := buildTextQuery(query)
	result, err := s.engine.SearchJson(tantivy.NewSearchContextBuilder().SetQueryFromJson(ptrQuery(finalQuery)).SetDocsLimit(uintptr(limit + len(overlays))).Build())
	if err != nil {
		return nil, fmt.Errorf("search: search Tantivy index: %w", err)
	}
	defer result.Free()
	disk, err := readSearchHits(result, s.engine)
	if err != nil {
		return nil, err
	}
	hits := make([]Hit, 0, len(disk)+len(overlays))
	for _, d := range disk {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, replaced := shadowed[d.ID]; replaced {
			continue
		}
		hits = append(hits, hitFromStored(d, query))
	}
	for _, d := range overlays {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if score, ok := rank.ScoreOverlay(rank.OverlayInput{
			Query: query, Title: d.Title, Path: d.Path, Source: d.Source,
			Aliases: d.Metadata.Aliases, Tags: d.Metadata.Tags,
		}); ok {
			start, end := findFold(d.Source, query)
			hits = append(hits, Hit{Path: d.Path, Revision: d.Revision, Start: start, End: end, Score: score, Snippet: makeSnippet(d.Source, start, end)})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool {
		if hits[a].Score != hits[b].Score {
			return hits[a].Score > hits[b].Score
		}
		return hits[a].Path < hits[b].Path
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func buildTextQuery(query string) tantivy.FinalQuery {
	qb := tantivy.NewQueryBuilder()
	terms := qb.NestedBuilder().
		Query(tantivy.Should, fieldTitleEN, query, tantivy.PhrasePrefixQuery, 14.0).
		Query(tantivy.Should, fieldTitleEdge, query, tantivy.EveryTermQuery, 10.0).
		Query(tantivy.Should, fieldPathEN, query, tantivy.PhrasePrefixQuery, 10.0).
		Query(tantivy.Should, fieldPathEdge, query, tantivy.EveryTermQuery, 8.0).
		Query(tantivy.Should, fieldBodyEN, query, tantivy.PhrasePrefixQuery, 1.0).
		Query(tantivy.Should, fieldMetaEN, query, tantivy.PhrasePrefixQuery, 4.0)
	if hanQuery := hanOnly(query); hanQuery != "" {
		terms.Query(tantivy.Should, fieldTitleHanNG, hanQuery, tantivy.EveryTermQuery, 8.0).
			Query(tantivy.Should, fieldPathHanNG, hanQuery, tantivy.EveryTermQuery, 6.0).
			Query(tantivy.Should, fieldBodyHanNG, hanQuery, tantivy.EveryTermQuery, 0.75).
			Query(tantivy.Should, fieldMetaHanNG, hanQuery, tantivy.EveryTermQuery, 3.0)
	}
	qb.BooleanQuery(tantivy.Must, terms, 1.0)
	return qb.Build()
}

// ptrQuery exists to keep QueryBuilder construction readable at the call site.
func ptrQuery(q tantivy.FinalQuery) *tantivy.FinalQuery { return &q }

func readSearchHits(result *tantivy.SearchResult, engine *tantivy.TantivyContext) ([]storedHit, error) {
	size, err := result.GetSize()
	if err != nil {
		return nil, fmt.Errorf("search: read Tantivy result size: %w", err)
	}
	hits := make([]storedHit, 0, size)
	fields := []string{fieldID, fieldPath, fieldRevision, fieldSource}
	for n := uint64(0); n < size; n++ {
		doc, err := result.Get(n)
		if err != nil {
			return nil, fmt.Errorf("search: load Tantivy result %d: %w", n, err)
		}
		var hit storedHit
		_, convertErr := tantivy.ToModel(doc, engine, fields, func(raw string) (struct{}, error) {
			return struct{}{}, json.Unmarshal([]byte(raw), &hit)
		})
		doc.Free()
		if convertErr != nil {
			return nil, fmt.Errorf("search: decode Tantivy result %d: %w", n, convertErr)
		}
		hit.Score = safeScore(hit.Score)
		hits = append(hits, hit)
	}
	return hits, nil
}

func hitFromStored(d storedHit, query string) Hit {
	start, end := findFold(d.Source, query)
	return Hit{Path: d.Path, Revision: d.Revision, Start: start, End: end, Score: d.Score, Snippet: makeSnippet(d.Source, start, end)}
}

// findFold returns byte offsets in the original source while comparing Unicode
// simple-fold-equivalent rune windows.
func makeSnippet(source string, start, end int) string {
	if start < 0 || end < start || end > len(source) {
		start, end = 0, 0
	}
	left, right := start-72, end+112
	if left < 0 {
		left = 0
	}
	if right > len(source) {
		right = len(source)
	}
	for left > 0 && left < len(source) && !utf8.RuneStart(source[left]) {
		left--
	}
	for right < len(source) && !utf8.RuneStart(source[right]) {
		right++
	}
	text := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(source[left:right])
	text = strings.TrimSpace(text)
	if left > 0 {
		text = "…" + text
	}
	if right < len(source) {
		text += "…"
	}
	runes := []rune(text)
	if len(runes) > 160 {
		text = string(runes[:159]) + "…"
	}
	return text
}
