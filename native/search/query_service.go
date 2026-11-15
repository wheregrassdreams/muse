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
	"muse/document/types"
	"muse/search/fields"
	"muse/search/plan"
	"muse/search/query"
	"muse/search/rank"
)

type queryCandidate struct {
	ID, Path, Title, Revision string
	Metadata                  types.Metadata
	Score                     float64
	Source                    string
	Volatile                  bool
}

// SearchQuery parses and compiles the typed query language before searching.
// Plain text expressions retain Search's established literal-query behavior.
// Structured filters are applied to the persisted metadata projection, then
// only the final page of source text is loaded for snippets.
func (i *Index) SearchQuery(ctx context.Context, expression string, limit int, options QueryOptions, overlays ...Document) ([]Hit, error) {
	if limit <= 0 || strings.TrimSpace(expression) == "" {
		return nil, nil
	}
	if !utf8.ValidString(expression) {
		return nil, errors.New("search: query is not valid UTF-8")
	}
	ast, parseErr := query.Parse(expression)
	if parseErr != nil {
		// Preserve the existing literal search contract for arbitrary punctuation
		// and query-language-looking text. A malformed filter remains an error.
		if looksLikeFilter(expression) {
			return nil, parseErr
		}
		return i.search(ctx, expression, limit, overlays...)
	}
	if len(ast.Filters) == 0 {
		return i.search(ctx, expression, limit, overlays...)
	}
	fieldRegistry := options.Fields
	if fieldRegistry == nil {
		fieldRegistry = fields.NewDefaults()
	}
	compiled, err := plan.Compile(ast, fieldRegistry)
	if err != nil {
		return nil, err
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
	overlayByID := make(map[string]Document, len(overlays))
	for _, d := range overlays {
		if err := validateDocument(d); err != nil {
			return nil, err
		}
		if _, exists := overlayByID[d.ID]; exists {
			return nil, fmt.Errorf("search: duplicate overlay ID %q", d.ID)
		}
		overlayByID[d.ID] = d
	}
	candidates := make([]queryCandidate, 0, min(len(s.manifest.Docs), 2048)+len(overlays))
	if compiled.Text != "" {
		finalQuery := buildTextQuery(compiled.Text)
		result, err := s.engine.SearchJson(tantivy.NewSearchContextBuilder().SetQueryFromJson(ptrQuery(finalQuery)).SetDocsLimit(uintptr(len(s.manifest.Docs))).Build())
		if err != nil {
			return nil, fmt.Errorf("search: execute query candidates: %w", err)
		}
		nativeCandidates, readErr := readSearchCandidates(result, s.engine)
		result.Free()
		if readErr != nil {
			return nil, readErr
		}
		for _, candidate := range nativeCandidates {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if _, shadowed := overlayByID[candidate.ID]; shadowed {
				continue
			}
			record, ok := s.manifest.Docs[candidate.ID]
			if !ok {
				continue
			}
			candidate.Path, candidate.Title = record.Path, record.Title
			candidate.Metadata = record.Metadata
			candidates = append(candidates, candidate)
		}
	} else {
		for id, record := range s.manifest.Docs {
			if _, shadowed := overlayByID[id]; !shadowed {
				candidates = append(candidates, queryCandidate{ID: id, Path: record.Path, Title: record.Title, Revision: record.Revision, Metadata: record.Metadata})
			}
		}
	}
	for _, d := range overlays {
		score := 0.0
		if compiled.Text != "" {
			var ok bool
			score, ok = overlayScore(d, compiled.Text)
			if !ok {
				continue
			}
		}
		candidates = append(candidates, queryCandidate{ID: d.ID, Path: d.Path, Title: d.Title, Revision: d.Revision, Metadata: d.Metadata, Source: d.Source, Score: score, Volatile: true})
	}
	filtered := candidates[:0]
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		matched, err := compiled.Match(candidate.Metadata)
		if err != nil {
			return nil, err
		}
		if !matched {
			continue
		}
		if options.Rankers != nil {
			candidate.Score += options.Rankers.Score(rank.Context{Query: compiled.Text, ID: candidate.ID, Path: candidate.Path, Title: candidate.Title, Metadata: candidate.Metadata})
		}
		candidate.Score = safeScore(candidate.Score)
		filtered = append(filtered, candidate)
	}
	sort.SliceStable(filtered, func(a, b int) bool {
		if filtered[a].Score != filtered[b].Score {
			return filtered[a].Score > filtered[b].Score
		}
		if filtered[a].Path != filtered[b].Path {
			return filtered[a].Path < filtered[b].Path
		}
		return filtered[a].ID < filtered[b].ID
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	if err := hydrateCandidateSources(ctx, s.engine, filtered); err != nil {
		return nil, err
	}
	hits := make([]Hit, 0, len(filtered))
	for _, candidate := range filtered {
		start, end := 0, 0
		snippet := makeSnippet(candidate.Source, 0, 0)
		if compiled.Text != "" {
			start, end = findFold(candidate.Source, compiled.Text)
			snippet = makeSnippet(candidate.Source, start, end)
		}
		hits = append(hits, Hit{Path: candidate.Path, Revision: candidate.Revision, Start: start, End: end, Score: candidate.Score, Snippet: snippet})
	}
	return hits, nil
}

func looksLikeFilter(expression string) bool {
	for _, field := range []string{"tag:", "tag=", "type:", "type=", "created<", "created>", "updated<", "updated>"} {
		if strings.Contains(strings.ToLower(expression), field) {
			return true
		}
	}
	return false
}

func readSearchCandidates(result *tantivy.SearchResult, engine *tantivy.TantivyContext) ([]queryCandidate, error) {
	size, err := result.GetSize()
	if err != nil {
		return nil, fmt.Errorf("search: read query candidate count: %w", err)
	}
	candidates := make([]queryCandidate, 0, size)
	fields := []string{fieldID, fieldPath, fieldRevision}
	for n := uint64(0); n < size; n++ {
		doc, err := result.Get(n)
		if err != nil {
			return nil, fmt.Errorf("search: load query candidate %d: %w", n, err)
		}
		var candidate queryCandidate
		_, decodeErr := tantivy.ToModel(doc, engine, fields, func(raw string) (struct{}, error) {
			var stored struct {
				ID       string  `json:"id"`
				Path     string  `json:"path"`
				Revision string  `json:"revision"`
				Score    float64 `json:"score"`
			}
			if err := json.Unmarshal([]byte(raw), &stored); err != nil {
				return struct{}{}, err
			}
			candidate.ID, candidate.Path, candidate.Revision, candidate.Score = stored.ID, stored.Path, stored.Revision, safeScore(stored.Score)
			return struct{}{}, nil
		})
		doc.Free()
		if decodeErr != nil {
			return nil, fmt.Errorf("search: decode query candidate %d: %w", n, decodeErr)
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func hydrateCandidateSources(ctx context.Context, engine *tantivy.TantivyContext, candidates []queryCandidate) error {
	for at := 0; at < len(candidates); at += literalBatchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		batch := candidates[at:min(at+literalBatchSize, len(candidates))]
		qb := tantivy.NewQueryBuilder()
		ids := qb.NestedBuilder()
		for _, candidate := range batch {
			ids.Query(tantivy.Should, fieldID, candidate.ID, tantivy.TermQuery, 1)
		}
		qb.BooleanQuery(tantivy.Must, ids, 1)
		result, err := engine.SearchJson(tantivy.NewSearchContextBuilder().SetQueryFromJson(ptrQuery(qb.Build())).SetDocsLimit(uintptr(len(batch))).Build())
		if err != nil {
			return fmt.Errorf("search: hydrate result source: %w", err)
		}
		stored, readErr := readSearchHits(result, engine)
		result.Free()
		if readErr != nil {
			return readErr
		}
		byID := make(map[string]storedHit, len(stored))
		for _, doc := range stored {
			byID[doc.ID] = doc
		}
		for n := range batch {
			if batch[n].Volatile {
				continue
			}
			doc, ok := byID[batch[n].ID]
			if !ok {
				return fmt.Errorf("search: missing source for candidate %q", batch[n].ID)
			}
			batch[n].Source = doc.Source
			if batch[n].Revision == "" {
				batch[n].Revision = doc.Revision
			}
		}
	}
	return nil
}
