package vaultsearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	tantivy "github.com/anyproto/tantivy-go"
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
		return nil, errors.New("vaultsearch: query is not valid UTF-8")
	}
	s, err := i.getState()
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, errors.New("vaultsearch: index is closed")
	}
	if s.poisoned {
		return nil, errors.New("vaultsearch: index transaction is incomplete; close and reopen to rebuild")
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
			return nil, fmt.Errorf("vaultsearch: duplicate overlay ID %q", d.ID)
		}
		shadowed[d.ID] = struct{}{}
	}
	qb := tantivy.NewQueryBuilder()
	terms := qb.NestedBuilder().
		Query(tantivy.Should, fieldTitleEN, query, tantivy.PhrasePrefixQuery, 14.0).
		Query(tantivy.Should, fieldTitleZH, query, tantivy.PhrasePrefixQuery, 14.0).
		Query(tantivy.Should, fieldTitleEdge, query, tantivy.EveryTermQuery, 10.0).
		Query(tantivy.Should, fieldTitleHanNG, query, tantivy.EveryTermQuery, 8.0).
		Query(tantivy.Should, fieldPathEN, query, tantivy.PhrasePrefixQuery, 10.0).
		Query(tantivy.Should, fieldPathZH, query, tantivy.PhrasePrefixQuery, 10.0).
		Query(tantivy.Should, fieldPathEdge, query, tantivy.EveryTermQuery, 8.0).
		Query(tantivy.Should, fieldPathHanNG, query, tantivy.EveryTermQuery, 6.0).
		Query(tantivy.Should, fieldBodyEN, query, tantivy.PhrasePrefixQuery, 1.0).
		Query(tantivy.Should, fieldBodyZH, query, tantivy.PhrasePrefixQuery, 1.0).
		Query(tantivy.Should, fieldBodyHanNG, query, tantivy.EveryTermQuery, 0.75).
		Query(tantivy.Should, fieldMetaEN, query, tantivy.PhrasePrefixQuery, 4.0).
		Query(tantivy.Should, fieldMetaZH, query, tantivy.PhrasePrefixQuery, 4.0)
	qb.BooleanQuery(tantivy.Must, terms, 1.0)
	result, err := s.engine.SearchJson(tantivy.NewSearchContextBuilder().SetQueryFromJson(ptrQuery(qb.Build())).SetDocsLimit(uintptr(limit + len(overlays))).Build())
	if err != nil {
		return nil, fmt.Errorf("vaultsearch: search Tantivy index: %w", err)
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
		if score, ok := overlayScore(d, query); ok {
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

// ptrQuery exists to keep QueryBuilder construction readable at the call site.
func ptrQuery(q tantivy.FinalQuery) *tantivy.FinalQuery { return &q }

func readSearchHits(result *tantivy.SearchResult, engine *tantivy.TantivyContext) ([]storedHit, error) {
	size, err := result.GetSize()
	if err != nil {
		return nil, fmt.Errorf("vaultsearch: read Tantivy result size: %w", err)
	}
	hits := make([]storedHit, 0, size)
	fields := []string{fieldID, fieldPath, fieldRevision, fieldSource}
	for n := uint64(0); n < size; n++ {
		doc, err := result.Get(n)
		if err != nil {
			return nil, fmt.Errorf("vaultsearch: load Tantivy result %d: %w", n, err)
		}
		var hit storedHit
		_, convertErr := tantivy.ToModel(doc, engine, fields, func(raw string) (struct{}, error) {
			return struct{}{}, json.Unmarshal([]byte(raw), &hit)
		})
		doc.Free()
		if convertErr != nil {
			return nil, fmt.Errorf("vaultsearch: decode Tantivy result %d: %w", n, convertErr)
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

// Overlays have no corpus statistics, so this score approximates field boosts
// with exact/prefix/substring matches; Tantivy and overlay scores are not on a
// calibrated common scale.
func overlayScore(d Document, query string) (float64, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return 0, false
	}
	score := 0.0
	title := strings.ToLower(d.Title)
	path := strings.ToLower(d.Path)
	if title == q {
		score += 24
	} else if strings.HasPrefix(title, q) {
		score += 18
	} else if strings.Contains(title, q) {
		score += 12
	}
	if strings.HasPrefix(path, q) {
		score += 10
	} else if strings.Contains(path, q) {
		score += 8
	}
	for _, value := range append(append([]string(nil), d.Aliases...), d.Tags...) {
		if strings.Contains(strings.ToLower(value), q) {
			score += 7
			break
		}
	}
	body := strings.ToLower(d.Source)
	if strings.Contains(body, q) {
		score += 2
	}
	if score == 0 {
		for _, token := range strings.Fields(q) {
			if strings.Contains(title, token) {
				score += 4
			} else if strings.Contains(body, token) {
				score += 0.5
			}
		}
	}
	return score, score > 0
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
