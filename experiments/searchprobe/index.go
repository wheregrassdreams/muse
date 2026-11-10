// Package searchprobe is an isolated caller-fed Bleve experiment.
package searchprobe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/lang/cjk"
	"github.com/blevesearch/bleve/v2/search/query"
)

type Document struct {
	ID, Path, Title string
	Tags, Aliases   []string
	Body            string
}
type Hit struct {
	ID, Path string
	Score    float64
}
type Match struct {
	ID, Path   string
	Start, End int
}
type Index struct {
	mu     sync.RWMutex
	closed bool
	disk   bleve.Index
	docs   map[string]Document
}

const snapshotKey = "searchprobe.snapshot.v1"

func newMemory() (bleve.Index, error) { return bleve.NewMemOnly(schema()) }

// Open creates an index only if the directory does not exist; other open errors
// never trigger destructive recreation. A path refers to index storage, not a vault.
func Open(path string) (*Index, error) {
	var b bleve.Index
	var err error
	if _, e := os.Stat(path); errors.Is(e, os.ErrNotExist) {
		b, err = bleve.New(path, schema())
	} else if e != nil {
		return nil, e
	} else {
		b, err = bleve.Open(path)
	}
	if err != nil {
		return nil, err
	}
	i := &Index{disk: b, docs: map[string]Document{}}
	raw, err := b.GetInternal([]byte(snapshotKey))
	var ids []string
	if err == nil && len(raw) > 0 {
		err = json.Unmarshal(raw, &ids)
	}
	if err == nil {
		for _, id := range ids {
			var d Document
			raw, err = b.GetInternal([]byte("doc:" + id))
			if err != nil {
				break
			}
			err = json.Unmarshal(raw, &d)
			if err != nil {
				break
			}
			i.docs[id] = d
		}
	}
	if err != nil {
		b.Close()
		return nil, err
	}
	return i, nil
}
func clone(d Document) Document {
	d.Tags = append([]string(nil), d.Tags...)
	d.Aliases = append([]string(nil), d.Aliases...)
	return d
}
func valid(d Document) error {
	if d.ID == "" || d.Path == "" {
		return errors.New("ID and Path required")
	}
	return nil
}

// Batch atomically persists upserts and deletes. Deletes win on collisions.
// Repeated upserts use the last value. Successful changes clear affected buffers.
func (i *Index) Batch(upserts []Document, deletes []string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, d := range upserts {
		if err := valid(d); err != nil {
			return err
		}
	}
	for _, id := range deletes {
		if id == "" {
			return errors.New("empty delete ID")
		}
	}
	next := make(map[string]Document, len(i.docs))
	for id, d := range i.docs {
		next[id] = d
	}
	batch := i.disk.NewBatch()
	for _, d := range upserts {
		d = clone(d)
		next[d.ID] = d
		raw, err := json.Marshal(d)
		if err != nil {
			return err
		}
		batch.SetInternal([]byte("doc:"+d.ID), raw)
		if err := batch.Index(d.ID, d); err != nil {
			return err
		}
	}
	for _, id := range deletes {
		delete(next, id)
		batch.Delete(id)
		batch.DeleteInternal([]byte("doc:" + id))
	}
	ids := make([]string, 0, len(next))
	for id := range next {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	batch.SetInternal([]byte(snapshotKey), raw)
	if err = i.disk.Batch(batch); err != nil {
		return err
	}
	i.docs = next
	return nil
}

// UpdateText creates a caller-owned buffer snapshot; it never changes the index.
func (i *Index) UpdateText(id, body string) (Document, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	d, ok := i.docs[id]
	if !ok {
		return Document{}, fmt.Errorf("unknown ID %q", id)
	}
	d = clone(d)
	d.Body = body
	return d, nil
}

// Search returns one hit per document. Plain query text is ANDed within a
// field and ORed across fields, without interpreting query-string syntax.
func (i *Index) Search(ctx context.Context, text string, limit int, buffers ...Document) ([]Hit, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	text = strings.TrimSpace(text)
	if text == "" || limit <= 0 {
		return nil, nil
	}
	q := bleve.NewDisjunctionQuery()
	for _, field := range []string{"Path", "Title", "Tags", "Aliases", "Body"} {
		m := bleve.NewMatchQuery(text)
		m.SetField(field)
		m.Analyzer = cjk.AnalyzerName
		m.SetOperator(query.MatchQueryOperatorAnd)
		q.AddQuery(m)
	}
	overlay, err := newMemory()
	if err != nil {
		return nil, err
	}
	defer overlay.Close()
	overrides, err := i.validateBuffers(buffers)
	if err != nil {
		return nil, err
	}
	batch := overlay.NewBatch()
	for _, d := range buffers {
		if err := valid(d); err != nil {
			return nil, err
		}
		overrides[d.ID] = d
		if err := batch.Index(d.ID, d); err != nil {
			return nil, err
		}
	}
	if err := overlay.Batch(batch); err != nil {
		return nil, err
	}
	hits := map[string]Hit{}
	for n, b := range []bleve.Index{i.disk, overlay} {
		// Fetch all candidates so stale persisted buffer hits cannot consume limit.
		req := bleve.NewSearchRequestOptions(q, len(i.docs)+len(overrides), 0, false)
		res, err := b.SearchInContext(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, h := range res.Hits {
			if _, ok := overrides[h.ID]; n == 0 && ok {
				continue
			}
			d := i.docs[h.ID]
			if n == 1 {
				d = overrides[h.ID]
			}
			hits[h.ID] = Hit{h.ID, d.Path, h.Score}
		}
	}
	out := make([]Hit, 0, len(hits))
	for _, h := range hits {
		out = append(out, h)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Path != out[b].Path {
			return out[a].Path < out[b].Path
		}
		return out[a].ID < out[b].ID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Literal scans effective Body snapshots: Unicode lowercase, trimmed query,
// non-overlapping occurrences, original UTF-8 offsets, path then byte order.
// A nonpositive limit returns no matches. Metadata is not searched here.
func (i *Index) Literal(ctx context.Context, query string, limit int, buffers ...Document) ([]Match, error) {
	i.mu.RLock()
	query = strings.TrimSpace(query)
	if query == "" || limit <= 0 {
		i.mu.RUnlock()
		return nil, nil
	}
	overrides, err := i.validateBuffers(buffers)
	if err != nil {
		i.mu.RUnlock()
		return nil, err
	}
	needle := strings.ToLower(query)
	docs := make([]Document, 0, len(i.docs))
	for id, d := range i.docs {
		if v, ok := overrides[id]; ok {
			d = v
		}
		docs = append(docs, d)
	}
	for id, d := range overrides {
		if _, ok := i.docs[id]; !ok {
			docs = append(docs, d)
		}
	}
	i.mu.RUnlock()
	sort.Slice(docs, func(a, b int) bool {
		if docs[a].Path != docs[b].Path {
			return docs[a].Path < docs[b].Path
		}
		return docs[a].ID < docs[b].ID
	})
	var out []Match
	for _, d := range docs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ascii := true
		for n := 0; n < len(d.Body); n++ {
			if d.Body[n] >= 128 {
				ascii = false
				break
			}
		}
		if ascii {
			hay := strings.ToLower(d.Body)
			for from := 0; from < len(hay); {
				n := strings.Index(hay[from:], needle)
				if n < 0 {
					break
				}
				a := from + n
				z := a + len(needle)
				out = append(out, Match{d.ID, d.Path, a, z})
				if len(out) >= limit {
					return out, nil
				}
				from = z
			}
			continue
		}
		// Most Unicode lowercase operations preserve byte lengths, including CJK.
		// Allocate byte offset maps only for the uncommon length-changing case.
		lower := strings.ToLower(d.Body)
		same := true
		for _, r := range d.Body {
			if len(strings.ToLower(string(r))) != len(string(r)) {
				same = false
				break
			}
		}
		if same {
			for from := 0; from < len(lower); {
				n := strings.Index(lower[from:], needle)
				if n < 0 {
					break
				}
				a := from + n
				z := a + len(needle)
				out = append(out, Match{d.ID, d.Path, a, z})
				if len(out) >= limit {
					return out, nil
				}
				from = z
			}
			continue
		}
		var folded strings.Builder
		var starts, ends []int
		for at, r := range d.Body {
			if at%4096 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			piece := strings.ToLower(string(r))
			folded.WriteString(piece)
			for range []byte(piece) {
				starts = append(starts, at)
				ends = append(ends, at+len(string(r)))
			}
		}

		hay := folded.String()
		for from := 0; from < len(hay); {
			n := strings.Index(hay[from:], needle)
			if n < 0 {
				break
			}
			a := from + n
			z := a + len(needle)
			out = append(out, Match{d.ID, d.Path, starts[a], ends[z-1]})
			if len(out) >= limit {
				return out, nil
			}
			from = z
		}
	}
	return out, nil
}
func (i *Index) Close() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return nil
	}
	i.closed = true
	return i.disk.Close()
}

func (i *Index) validateBuffers(buffers []Document) (map[string]Document, error) {
	out := map[string]Document{}
	for _, d := range buffers {
		if err := valid(d); err != nil {
			return nil, err
		}
		if old, ok := i.docs[d.ID]; ok && old.Path != d.Path {
			return nil, errors.New("overlay path differs from persisted ID")
		}
		if _, ok := out[d.ID]; ok {
			return nil, errors.New("duplicate overlay ID")
		}
		out[d.ID] = d
	}
	return out, nil
}
