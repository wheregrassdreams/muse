package search

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	tantivy "github.com/anyproto/tantivy-go"
)

const literalProjectionBudget int64 = 32 << 20

const literalMapEntryBudget int64 = 128

const literalOrderBudget int64 = 8 << 20

const literalOrderEntryBytes int64 = 16

const literalOrderIncrementalChangeLimit = 1024

// literalDoc is the query-time projection of one indexed source. Keeping the
// folded bytes avoids reloading and normalizing the whole corpus on every
// literal query. Source text itself remains in Tantivy and is read only for
// documents that contribute returned hits.
type literalDoc struct {
	Path, Revision string
	folded         string
	starts, ends   []int
}

func (d literalDoc) bytes(id string) int64 {
	// Count text, offsets, metadata strings, and a conservative per-entry
	// allowance for the map bucket and struct headers. Benchmarks report this
	// deterministic retained-size estimate alongside transient B/op.
	return literalMapEntryBudget + int64(len(id)+len(d.Path)+len(d.folded)+(len(d.starts)+len(d.ends))*8)
}

func makeLiteralDoc(d Document) literalDoc {
	folded, starts, ends := lowercaseOffsets(d.Source)
	return literalDoc{Path: d.Path, Revision: d.Revision, folded: folded, starts: starts, ends: ends}
}

func buildLiteralProjection(ctx context.Context, docs []Document, previous map[string]literalDoc) (map[string]literalDoc, int64, []string, bool, error) {
	ordered := append([]Document(nil), docs...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Path != ordered[j].Path {
			return ordered[i].Path < ordered[j].Path
		}
		return ordered[i].ID < ordered[j].ID
	})
	next := make(map[string]literalDoc, min(len(ordered), 4096))
	orderBytes := literalOrderBytesForDocs(ordered)
	orderReady := orderBytes <= literalOrderBudget
	var order []string
	if orderReady {
		order = make([]string, 0, len(ordered))
	}
	var bytes int64
	for _, input := range ordered {
		if err := ctx.Err(); err != nil {
			return nil, 0, nil, false, err
		}
		if orderReady {
			order = append(order, input.ID)
		}
		d := input
		if d.Revision == "" {
			d.Revision = sourceDigest(d.Source)
		}
		if old, ok := previous[d.ID]; ok && old.Path == d.Path && old.Revision == d.Revision {
			if size := old.bytes(d.ID); bytes+size <= literalProjectionBudget {
				next[d.ID] = old
				bytes += size
				continue
			}
		}
		// Avoid building a transient projection for a single oversized note.
		if literalProjectionEstimate(d) > literalProjectionBudget-bytes {
			continue
		}
		projection := makeLiteralDoc(d)
		size := projection.bytes(d.ID)
		if bytes+size > literalProjectionBudget {
			continue
		}
		next[d.ID] = projection
		bytes += size
	}
	return next, bytes, order, orderReady, nil
}

// applyLiteralOrder runs under indexState.mu after the manifest has committed.
// It removes changed/deleted IDs from the old sorted sequence, sorts only the
// changed IDs, then merges the two sorted runs. If the optional sequence would
// exceed its fixed budget, queries fall back to sorting candidates per call.
func (s *indexState) applyLiteralOrder(upserts []Document, deletes []string) {
	if !s.literalOrderReady {
		return
	}
	if len(upserts) == 0 && len(deletes) == 0 {
		return
	}
	if len(upserts)+len(deletes) > literalOrderIncrementalChangeLimit {
		s.literalOrder, s.literalOrderBytes, s.literalOrderReady = buildLiteralOrderFromManifest(s.manifest.Docs)
		return
	}
	removed := make(map[string]struct{}, len(upserts)+len(deletes))
	for _, id := range deletes {
		removed[id] = struct{}{}
	}
	for _, doc := range upserts {
		removed[doc.ID] = struct{}{}
	}
	base := make([]string, 0, len(s.literalOrder))
	for _, id := range s.literalOrder {
		if _, changed := removed[id]; !changed {
			base = append(base, id)
		}
	}
	added := make([]string, 0, len(upserts))
	for _, doc := range upserts {
		if _, exists := s.manifest.Docs[doc.ID]; exists {
			added = append(added, doc.ID)
		}
	}
	if literalOrderSize(base)+literalOrderSize(added) > literalOrderBudget {
		s.literalOrder = nil
		s.literalOrderBytes = 0
		s.literalOrderReady = false
		return
	}
	sort.Slice(added, func(i, j int) bool {
		return literalOrderLess(s.manifest.Docs, added[i], added[j])
	})
	merged := make([]string, 0, len(base)+len(added))
	i, j := 0, 0
	for i < len(base) && j < len(added) {
		if literalOrderLess(s.manifest.Docs, base[i], added[j]) {
			merged = append(merged, base[i])
			i++
		} else {
			merged = append(merged, added[j])
			j++
		}
	}
	merged = append(merged, base[i:]...)
	merged = append(merged, added[j:]...)
	s.literalOrder = merged
	s.literalOrderBytes = literalOrderSize(merged)
}

func literalOrderBytesForDocs(docs []Document) int64 {
	var bytes int64
	for _, doc := range docs {
		bytes += literalOrderEntryBytes + int64(len(doc.ID))
		if bytes > literalOrderBudget {
			return bytes
		}
	}
	return bytes
}

func literalOrderSize(ids []string) int64 {
	var bytes int64
	for _, id := range ids {
		bytes += literalOrderEntryBytes + int64(len(id))
	}
	return bytes
}

func buildLiteralOrderFromManifest(docs map[string]manifestRecord) ([]string, int64, bool) {
	var bytes int64
	for id := range docs {
		bytes += literalOrderEntryBytes + int64(len(id))
		if bytes > literalOrderBudget {
			return nil, 0, false
		}
	}
	order := make([]string, 0, len(docs))
	for id := range docs {
		order = append(order, id)
	}
	sort.Slice(order, func(i, j int) bool {
		return literalOrderLess(docs, order[i], order[j])
	})
	return order, bytes, true
}

func literalOrderLess(docs map[string]manifestRecord, a, b string) bool {
	left, right := docs[a], docs[b]
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	return a < b
}

func (s *indexState) applyLiteralProjection(upserts []Document, deletes []string, prepared map[string]literalDoc) {
	for _, id := range deletes {
		if old, ok := s.literalDocs[id]; ok {
			s.literalBytes -= old.bytes(id)
			delete(s.literalDocs, id)
		}
	}
	for _, d := range upserts {
		if old, ok := s.literalDocs[d.ID]; ok {
			s.literalBytes -= old.bytes(d.ID)
			delete(s.literalDocs, d.ID)
		}
		projection, ok := prepared[d.ID]
		if !ok {
			continue
		}
		size := projection.bytes(d.ID)
		if s.literalBytes+size > literalProjectionBudget {
			continue
		}
		s.literalDocs[d.ID] = projection
		s.literalBytes += size
	}
}

func prepareLiteralProjections(upserts []Document, budget int64) map[string]literalDoc {
	projections := make(map[string]literalDoc)
	if budget <= 0 || len(upserts) == 0 {
		return projections
	}
	order := make([]int, len(upserts))
	for n := range order {
		order[n] = n
	}
	sort.SliceStable(order, func(i, j int) bool {
		return upserts[order[i]].ID < upserts[order[j]].ID
	})
	projections = make(map[string]literalDoc, min(len(upserts), int(budget/literalMapEntryBudget)))
	lastID := ""
	hasLastID := false
	for _, at := range order {
		d := upserts[at]
		if hasLastID && d.ID == lastID {
			continue
		}
		lastID, hasLastID = d.ID, true
		if literalProjectionEstimate(d) > budget {
			continue
		}
		projection := makeLiteralDoc(d)
		size := projection.bytes(d.ID)
		if size > budget {
			continue
		}
		projections[d.ID] = projection
		budget -= size
	}
	return projections
}

func literalProjectionEstimate(d Document) int64 {
	foldedBytes := 0
	lengthChanged := false
	for _, r := range d.Source {
		originalBytes := utf8.RuneLen(r)
		lowerBytes := utf8.RuneLen(unicode.ToLower(r))
		foldedBytes += lowerBytes
		lengthChanged = lengthChanged || originalBytes != lowerBytes
	}
	estimate := literalMapEntryBudget + int64(len(d.ID)+len(d.Path)+foldedBytes)
	if lengthChanged {
		estimate += int64(foldedBytes) * 2 * 8
	}
	return estimate
}

type literalCandidate struct {
	id, path, revision string
	source             string
	folded             string
	starts, ends       []int
	cached, overlay    bool
}

type literalMatch struct {
	id, path, revision string
	source             string
	start, end         int
	overlay            bool
}

// literalCachedLocked runs with indexState.mu held for reading. Results remain
// ordered by path and source position, exactly like the storage scan.
func literalCachedLocked(ctx context.Context, s *indexState, query string, limit int, overlays []Document) ([]Hit, error) {
	if limit <= 0 || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
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

	overlayCandidates := make([]literalCandidate, 0, len(shadow))
	for id, d := range shadow {
		cached := makeLiteralDoc(d)
		overlayCandidates = append(overlayCandidates, literalCandidate{id: id, path: d.Path, revision: d.Revision, source: d.Source, folded: cached.folded, starts: cached.starts, ends: cached.ends, cached: true, overlay: true})
	}
	sort.Slice(overlayCandidates, func(i, j int) bool {
		return literalCandidateLess(overlayCandidates[i], overlayCandidates[j])
	})
	candidates := make([]literalCandidate, 0, len(s.manifest.Docs)+len(shadow))
	if s.literalOrderReady && len(s.literalOrder) == len(s.manifest.Docs) {
		storedAt, overlayAt := 0, 0
		var stored literalCandidate
		storedOK := false
		for storedAt < len(s.literalOrder) || storedOK || overlayAt < len(overlayCandidates) {
			if !storedOK {
				for storedAt < len(s.literalOrder) {
					id := s.literalOrder[storedAt]
					storedAt++
					if _, replaced := shadow[id]; replaced {
						continue
					}
					rec, ok := s.manifest.Docs[id]
					if !ok {
						return nil, fmt.Errorf("search: stale literal order ID %q", id)
					}
					stored = literalCandidate{id: id, path: rec.Path, revision: rec.Revision}
					if projection, ok := s.literalDocs[id]; ok && projection.Path == rec.Path && projection.Revision == rec.Revision {
						stored.folded, stored.starts, stored.ends, stored.cached = projection.folded, projection.starts, projection.ends, true
					}
					storedOK = true
					break
				}
			}
			if overlayAt < len(overlayCandidates) && (!storedOK || literalCandidateLess(overlayCandidates[overlayAt], stored)) {
				candidates = append(candidates, overlayCandidates[overlayAt])
				overlayAt++
			} else if storedOK {
				candidates = append(candidates, stored)
				storedOK = false
			}
		}
	} else {
		for id, rec := range s.manifest.Docs {
			if _, replaced := shadow[id]; replaced {
				continue
			}
			d := literalCandidate{id: id, path: rec.Path, revision: rec.Revision}
			if projection, ok := s.literalDocs[id]; ok && projection.Path == rec.Path && projection.Revision == rec.Revision {
				d.folded, d.starts, d.ends, d.cached = projection.folded, projection.starts, projection.ends, true
			}
			candidates = append(candidates, d)
		}
		candidates = append(candidates, overlayCandidates...)
		sort.Slice(candidates, func(i, j int) bool {
			return literalCandidateLess(candidates[i], candidates[j])
		})
	}

	needle := strings.ToLower(strings.TrimSpace(query))
	matches := make([]literalMatch, 0, min(limit, 64))
	ids := make([]string, 0, min(limit, 64))
	seenIDs := make(map[string]struct{}, min(limit, 64))
	scannedSources := make(map[string]storedHit, min(limit, 64))
	var pending []literalCandidate
	flushPending := func() error {
		if len(pending) == 0 || len(matches) >= limit {
			pending = pending[:0]
			return nil
		}
		batchIDs := make([]string, len(pending))
		for i := range pending {
			batchIDs[i] = pending[i].id
		}
		sources, err := literalSourcesByID(ctx, s, batchIDs)
		if err != nil {
			return err
		}
		for _, d := range pending {
			stored, ok := sources[d.id]
			if !ok {
				return fmt.Errorf("search: missing indexed source %q", d.id)
			}
			folded, starts, ends := lowercaseOffsets(stored.Source)
			d.folded, d.starts, d.ends = folded, starts, ends
			before := len(matches)
			if err := scanLiteralCandidate(ctx, d, needle, limit, &matches, &ids, seenIDs); err != nil {
				return err
			}
			if len(matches) > before {
				scannedSources[d.id] = stored
			}
			if len(matches) >= limit {
				break
			}
		}
		pending = pending[:0]
		return nil
	}
	for _, d := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !d.cached {
			pending = append(pending, d)
			if len(pending) >= literalBatchSize {
				if err := flushPending(); err != nil {
					return nil, err
				}
			}
		} else {
			if err := flushPending(); err != nil {
				return nil, err
			}
			if err := scanLiteralCandidate(ctx, d, needle, limit, &matches, &ids, seenIDs); err != nil {
				return nil, err
			}
		}
		if len(matches) >= limit {
			break
		}
	}
	if err := flushPending(); err != nil {
		return nil, err
	}

	missingSources := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := scannedSources[id]; !ok {
			missingSources = append(missingSources, id)
		}
	}
	sources, err := literalSourcesByID(ctx, s, missingSources)
	if err != nil {
		return nil, err
	}
	if sources == nil && len(scannedSources) > 0 {
		sources = make(map[string]storedHit, len(scannedSources))
	}
	for id, source := range scannedSources {
		sources[id] = source
	}
	hits := make([]Hit, 0, len(matches))
	for _, m := range matches {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		source := m.source
		if !m.overlay {
			stored, ok := sources[m.id]
			if !ok {
				return nil, fmt.Errorf("search: missing indexed source %q", m.id)
			}
			source = stored.Source
		}
		hits = append(hits, Hit{Path: m.path, Revision: m.revision, Start: m.start, End: m.end, Score: 1, Snippet: makeSnippet(source, m.start, m.end)})
	}
	return hits, nil
}

func literalCandidateLess(a, b literalCandidate) bool {
	if a.path != b.path {
		return a.path < b.path
	}
	return a.id < b.id
}

func scanLiteralCandidate(ctx context.Context, d literalCandidate, needle string, limit int, matches *[]literalMatch, ids *[]string, seenIDs map[string]struct{}) error {
	for from := 0; from < len(d.folded) && len(*matches) < limit; {
		if err := ctx.Err(); err != nil {
			return err
		}
		n := strings.Index(d.folded[from:], needle)
		if n < 0 {
			break
		}
		a, z := from+n, from+n+len(needle)
		start, end := a, z
		if d.starts != nil {
			start, end = d.starts[a], d.ends[z-1]
		}
		*matches = append(*matches, literalMatch{id: d.id, path: d.path, revision: d.revision, source: d.source, start: start, end: end, overlay: d.overlay})
		if !d.overlay {
			if _, exists := seenIDs[d.id]; !exists {
				seenIDs[d.id] = struct{}{}
				*ids = append(*ids, d.id)
			}
		}
		from = z
	}
	return nil
}

func literalSourcesByID(ctx context.Context, s *indexState, ids []string) (map[string]storedHit, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	stored := make(map[string]storedHit, len(ids))
	for at := 0; at < len(ids); at += literalBatchSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch := ids[at:min(at+literalBatchSize, len(ids))]
		qb := tantivy.NewQueryBuilder()
		terms := qb.NestedBuilder()
		for _, id := range batch {
			terms.Query(tantivy.Should, fieldID, id, tantivy.TermQuery, 1)
		}
		qb.BooleanQuery(tantivy.Must, terms, 1)
		result, err := s.engine.SearchJson(tantivy.NewSearchContextBuilder().
			SetQueryFromJson(ptrQuery(qb.Build())).SetDocsLimit(uintptr(len(batch))).Build())
		if err != nil {
			return nil, fmt.Errorf("search: read literal match sources: %w", err)
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
	return stored, nil
}
