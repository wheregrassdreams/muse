package search

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const (
	searchBenchmarkDocBytes = 2 << 10
	searchBenchmarkLimit    = 100
)

var searchBenchmarkHits []Hit
var searchBenchmarkCandidates []literalCandidate

func BenchmarkSearchLatency(b *testing.B) {
	for _, count := range []int{1_000, 10_000} {
		idx, _ := benchmarkPersistentIndex(b, count)
		b.Run(fmt.Sprintf("files=%d/english-title", count), func(b *testing.B) {
			benchmarkSearch(b, idx, "Project Atlas", searchBenchmarkLimit)
		})
		b.Run(fmt.Sprintf("files=%d/english-body", count), func(b *testing.B) {
			benchmarkSearch(b, idx, "common searchable phrase", searchBenchmarkLimit)
		})
		b.Run(fmt.Sprintf("files=%d/cjk-body", count), func(b *testing.B) {
			benchmarkSearch(b, idx, "笔记正文检索片段", searchBenchmarkLimit)
		})
		b.Run(fmt.Sprintf("files=%d/rare-term", count), func(b *testing.B) {
			benchmarkSearch(b, idx, "rarebenchmarktoken", searchBenchmarkLimit)
		})
		_ = idx.Close()
	}
}

func BenchmarkLiteralLatency(b *testing.B) {
	for _, count := range []int{1_000, 10_000} {
		idx, _ := benchmarkPersistentIndex(b, count)
		b.Run(fmt.Sprintf("files=%d/common-en", count), func(b *testing.B) {
			benchmarkLiteral(b, idx, "common searchable phrase", searchBenchmarkLimit)
		})
		b.Run(fmt.Sprintf("files=%d/common-cjk", count), func(b *testing.B) {
			benchmarkLiteral(b, idx, "笔记正文检索片段", searchBenchmarkLimit)
		})
		b.Run(fmt.Sprintf("files=%d/rare-cjk", count), func(b *testing.B) {
			benchmarkLiteral(b, idx, "稀有词语基准", searchBenchmarkLimit)
		})
		b.Run(fmt.Sprintf("files=%d/no-match", count), func(b *testing.B) {
			benchmarkLiteral(b, idx, "当前库里完全不存在的查询词", searchBenchmarkLimit)
		})
		_ = idx.Close()
	}
}

// BenchmarkLiteralOverBudget exercises the bounded-cache fallback: 20k 2KiB
// documents exceed the 32MiB projection budget. Setup stays outside the timer.
func BenchmarkLiteralOverBudget(b *testing.B) {
	idx, _ := benchmarkPersistentIndex(b, 20_000)
	defer idx.Close()
	for _, query := range []struct{ name, value string }{
		{name: "no-match", value: "当前库里完全不存在的查询词"},
		{name: "common-en", value: "common searchable phrase"},
	} {
		b.Run(query.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				hits, err := idx.Literal(context.Background(), query.value, searchBenchmarkLimit)
				if err != nil {
					b.Fatal(err)
				}
				searchBenchmarkHits = hits
			}
			b.StopTimer()
			b.ReportMetric(float64(idx.state.literalBytes), "proj-B")
			b.ReportMetric(float64(idx.state.literalOrderBytes), "order-B")
		})
	}
}

// BenchmarkLiteralBudgetScale reports retained projection payload and Go heap
// delta for representative vault sizes, then measures common and no-match
// literal queries with corpus construction excluded from the query timers.
func BenchmarkLiteralBudgetScale(b *testing.B) {
	for _, count := range []int{20_000, 50_000} {
		docs := searchBenchmarkDocumentsUnique(count)
		idx, err := openLocalTestIndex(b.TempDir())
		if err != nil {
			b.Fatal(err)
		}
		if err := idx.Reconcile(context.Background(), docs); err != nil {
			b.Fatal(err)
		}
		docs = nil
		runtime.GC()
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		heapBytes := after.HeapAlloc
		b.Run(fmt.Sprintf("files=%d", count), func(b *testing.B) {
			for _, query := range []struct{ name, value string }{
				{name: "no-match", value: "当前库里完全不存在的查询词"},
				{name: "common-en", value: "common searchable phrase"},
			} {
				b.Run(query.name, func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					for n := 0; n < b.N; n++ {
						hits, err := idx.Literal(context.Background(), query.value, searchBenchmarkLimit)
						if err != nil {
							b.Fatal(err)
						}
						searchBenchmarkHits = hits
					}
					b.StopTimer()
					b.ReportMetric(float64(idx.state.literalBytes), "proj-B")
					b.ReportMetric(float64(idx.state.literalOrderBytes), "order-B")
					b.ReportMetric(float64(heapBytes), "heap-B")
				})
			}
		})
		if err := idx.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLiteralUniqueLatency(b *testing.B) {
	for _, count := range []int{10_000, 20_000, 50_000} {
		docs := searchBenchmarkDocumentsUnique(count)
		idx, err := openLocalTestIndex(b.TempDir())
		if err != nil {
			b.Fatal(err)
		}
		if err := idx.Reconcile(context.Background(), docs); err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("files=%d", count), func(b *testing.B) {
			for _, query := range []struct{ name, value string }{
				{name: "no-match", value: "当前库里完全不存在的查询词"},
				{name: "common-en", value: "common searchable phrase"},
			} {
				b.Run(query.name, func(b *testing.B) {
					benchmarkLiteral(b, idx, query.value, searchBenchmarkLimit)
				})
			}
		})
		_ = idx.Close()
	}
}

// BenchmarkLiteralCandidateOrdering isolates the per-query candidate ordering
// cost against a hypothetical path-sorted ID sequence maintained with the
// manifest. It does not alter product behavior or include source scanning.
func BenchmarkLiteralCandidateOrdering(b *testing.B) {
	for _, count := range []int{10_000, 20_000} {
		docs := searchBenchmarkDocuments(count)
		manifest := make(map[string]manifestRecord, len(docs))
		orderedIDs := make([]string, 0, len(docs))
		for _, doc := range docs {
			manifest[doc.ID] = manifestRecord{Path: doc.Path, Revision: doc.Revision}
			orderedIDs = append(orderedIDs, doc.ID)
		}
		b.Run(fmt.Sprintf("files=%d/sort-each-query", count), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				candidates := make([]literalCandidate, 0, len(manifest))
				for id, rec := range manifest {
					candidates = append(candidates, literalCandidate{id: id, path: rec.Path, revision: rec.Revision})
				}
				sort.Slice(candidates, func(i, j int) bool {
					if candidates[i].path != candidates[j].path {
						return candidates[i].path < candidates[j].path
					}
					return candidates[i].id < candidates[j].id
				})
				searchBenchmarkCandidates = candidates
			}
		})
		b.Run(fmt.Sprintf("files=%d/preordered-ids", count), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				candidates := make([]literalCandidate, 0, len(manifest))
				for _, id := range orderedIDs {
					rec := manifest[id]
					candidates = append(candidates, literalCandidate{id: id, path: rec.Path, revision: rec.Revision})
				}
				searchBenchmarkCandidates = candidates
			}
		})
	}
}

// Cold includes creation of a fresh persistent Tantivy index and the initial
// full reconcile. Use -benchtime=1x -count=5 so each sample measures a fresh
// index without retaining many large indexes in one benchmark process.
func BenchmarkColdReconcile(b *testing.B) {
	for _, count := range []int{1_000, 10_000, 20_000} {
		docs := searchBenchmarkDocuments(count)
		b.Run(fmt.Sprintf("files=%d", count), func(b *testing.B) {
			root := b.TempDir()
			b.ReportAllocs()
			b.SetBytes(int64(count * searchBenchmarkDocBytes))
			b.ResetTimer()
			var projectionBytes, orderBytes int64
			for n := 0; n < b.N; n++ {
				dir := filepath.Join(root, fmt.Sprintf("index-%d", n))
				idx, err := openLocalTestIndex(dir)
				if err != nil {
					b.Fatal(err)
				}
				if err := idx.Reconcile(context.Background(), docs); err != nil {
					b.Fatal(err)
				}
				projectionBytes = idx.state.literalBytes
				orderBytes = idx.state.literalOrderBytes
				if err := idx.Close(); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(projectionBytes), "proj-B")
			b.ReportMetric(float64(orderBytes), "order-B")
		})
	}
}

// OpenAndReconcile measures the persisted-index startup path. The corpus and
// initial index are prepared outside the timed region; every iteration opens
// the same index and verifies that all revisions are unchanged.
func BenchmarkOpenAndReconcileUnchanged(b *testing.B) {
	for _, count := range []int{1_000, 10_000, 20_000} {
		docs := searchBenchmarkDocuments(count)
		root := b.TempDir()
		initial, err := openLocalTestIndex(root)
		if err != nil {
			b.Fatal(err)
		}
		if err := initial.Reconcile(context.Background(), docs); err != nil {
			b.Fatal(err)
		}
		if err := initial.Close(); err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("files=%d", count), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			var projectionBytes, orderBytes int64
			for n := 0; n < b.N; n++ {
				idx, err := openLocalTestIndex(root)
				if err != nil {
					b.Fatal(err)
				}
				if err := idx.Reconcile(context.Background(), docs); err != nil {
					b.Fatal(err)
				}
				projectionBytes = idx.state.literalBytes
				orderBytes = idx.state.literalOrderBytes
				if err := idx.Close(); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(projectionBytes), "proj-B")
			b.ReportMetric(float64(orderBytes), "order-B")
		})
	}
}

func BenchmarkApplyOneDocument(b *testing.B) {
	for _, count := range []int{1_000, 10_000} {
		idx, docs := benchmarkPersistentIndex(b, count)
		b.Run(fmt.Sprintf("files=%d", count), func(b *testing.B) {
			updated := docs[0]
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				updated.Revision = fmt.Sprintf("update-%d", n)
				if err := idx.Apply(context.Background(), []Document{updated}, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
		_ = idx.Close()
	}
}

// BenchmarkApplyBatch measures watcher bursts independently of
// vault creation: all upsert documents already exist in the persistent index.
func BenchmarkApplyBatch(b *testing.B) {
	const count = 10_000
	idx, docs := benchmarkPersistentIndex(b, count)
	defer idx.Close()
	for _, batchSize := range []int{1, 128, 512, 1024} {
		b.Run(fmt.Sprintf("upserts=%d", batchSize), func(b *testing.B) {
			batch := append([]Document(nil), docs[:batchSize]...)
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				for k := range batch {
					batch[k].Revision = fmt.Sprintf("batch-%d-%d", n, k)
				}
				if err := idx.Apply(context.Background(), batch, nil); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(idx.state.literalOrderBytes), "order-B")
		})
	}
}

// BenchmarkApplyOrderMaintenance compares the same Apply path with and without
// maintaining the optional sorted-ID sequence. The index and source changes
// are identical; disabling the sequence models the previous cache behavior.
func BenchmarkApplyOrderMaintenance(b *testing.B) {
	idx, docs := benchmarkPersistentIndex(b, 10_000)
	defer idx.Close()
	for _, batchSize := range []int{1, 128, 512, 1024} {
		batch := append([]Document(nil), docs[:batchSize]...)
		b.Run(fmt.Sprintf("upserts=%d/ordered", batchSize), func(b *testing.B) {
			idx.state.mu.Lock()
			idx.state.literalOrder, idx.state.literalOrderBytes, idx.state.literalOrderReady = buildLiteralOrderFromManifest(idx.state.manifest.Docs)
			idx.state.mu.Unlock()
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				for k := range batch {
					batch[k].Revision = fmt.Sprintf("ordered-%d-%d", n, k)
				}
				if err := idx.Apply(context.Background(), batch, nil); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(idx.state.literalOrderBytes), "order-B")
		})
		b.Run(fmt.Sprintf("upserts=%d/fallback", batchSize), func(b *testing.B) {
			idx.state.mu.Lock()
			idx.state.literalOrder, idx.state.literalOrderBytes, idx.state.literalOrderReady = nil, 0, false
			idx.state.mu.Unlock()
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				for k := range batch {
					batch[k].Revision = fmt.Sprintf("fallback-%d-%d", n, k)
				}
				if err := idx.Apply(context.Background(), batch, nil); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
		})
	}
}

// BenchmarkLiteralOrderMergeCost isolates the bounded manifest-order merge
// from the Tantivy transaction so any Apply-side sequence cost is visible.
func BenchmarkLiteralOrderMergeCost(b *testing.B) {
	docs := searchBenchmarkDocuments(10_000)
	manifestDocs := make(map[string]manifestRecord, len(docs))
	for _, doc := range docs {
		manifestDocs[doc.ID] = manifestRecord{Path: doc.Path, Revision: doc.Revision}
	}
	order, orderBytes, ready := buildLiteralOrderFromManifest(manifestDocs)
	if !ready {
		b.Fatal("benchmark order exceeded budget")
	}
	for _, batchSize := range []int{1, 128, 512, 1024} {
		upserts := append([]Document(nil), docs[:batchSize]...)
		b.Run(fmt.Sprintf("upserts=%d", batchSize), func(b *testing.B) {
			state := &indexState{manifest: manifest{Docs: manifestDocs}, literalOrder: append([]string(nil), order...), literalOrderBytes: orderBytes, literalOrderReady: true}
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				state.applyLiteralOrder(upserts, nil)
			}
			b.StopTimer()
			b.ReportMetric(float64(state.literalOrderBytes), "order-B")
		})
	}
}

// BenchmarkPrepareLiteralProjectionsLargeBatch isolates the bounded temporary
// projection preparation for an oversized watcher batch. It intentionally
// exceeds the retained-cache budget several times over; setup is outside the
// timed region so B/op reflects preprocessing only.
func BenchmarkPrepareLiteralProjectionsLargeBatch(b *testing.B) {
	const (
		count      = 512
		sourceSize = 128 << 10
	)
	source := strings.Repeat("Searchable text with mixed case. 中文检索内容。 ", sourceSize/len("Searchable text with mixed case. 中文检索内容。 "))
	docs := make([]Document, count)
	for n := range docs {
		docs[n] = Document{ID: fmt.Sprintf("doc-%04d", n), Path: fmt.Sprintf("%04d.md", n), Revision: "v1", Source: source}
	}
	b.ReportAllocs()
	b.ResetTimer()
	var retained int64
	for n := 0; n < b.N; n++ {
		prepared := prepareLiteralProjections(docs, literalProjectionBudget)
		retained = 0
		for id, projection := range prepared {
			retained += projection.bytes(id)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(retained), "prepared-B")
	b.ReportMetric(float64(literalProjectionBudget), "budget-B")
}

func benchmarkSearch(b *testing.B, idx *Index, query string, limit int) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		hits, err := idx.Search(context.Background(), query, limit)
		if err != nil {
			b.Fatal(err)
		}
		searchBenchmarkHits = hits
	}
	b.StopTimer()
	b.ReportMetric(float64(idx.state.literalBytes), "proj-B")
	b.ReportMetric(float64(idx.state.literalOrderBytes), "order-B")
}

func benchmarkLiteral(b *testing.B, idx *Index, query string, limit int) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		hits, err := idx.Literal(context.Background(), query, limit)
		if err != nil {
			b.Fatal(err)
		}
		searchBenchmarkHits = hits
	}
	b.StopTimer()
	b.ReportMetric(float64(idx.state.literalBytes), "proj-B")
	b.ReportMetric(float64(idx.state.literalOrderBytes), "order-B")
}

func benchmarkPersistentIndex(b *testing.B, count int) (*Index, []Document) {
	b.Helper()
	docs := searchBenchmarkDocuments(count)
	idx, err := openLocalTestIndex(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	if err := idx.Reconcile(context.Background(), docs); err != nil {
		b.Fatal(err)
	}
	return idx, docs
}

func estimatedLiteralProjectionBytes(docs []Document) int64 {
	var bytes int64
	for _, doc := range docs {
		projection := makeLiteralDoc(doc)
		bytes += projection.bytes(doc.ID)
	}
	return bytes
}

func searchBenchmarkDocuments(count int) []Document {
	docs := make([]Document, count)
	var body strings.Builder
	for body.Len() < searchBenchmarkDocBytes-160 {
		body.WriteString("common searchable phrase with punctuation: alpha+beta. 笔记正文检索片段，提供中文搜索内容。 ")
	}
	for n := range docs {
		source := body.String()
		if n%97 == 0 {
			source += " 稀有词语基准 rarebenchmarktoken。"
		}
		docs[n] = Document{
			ID:       fmt.Sprintf("doc-%05d", n),
			Path:     fmt.Sprintf("Projects/Atlas/Note %05d.md", n),
			Title:    fmt.Sprintf("Project Atlas Note %05d", n),
			Source:   source,
			Revision: "v1",
		}
	}
	return docs
}

func searchBenchmarkDocumentsUnique(count int) []Document {
	docs := searchBenchmarkDocuments(count)
	for n := range docs {
		docs[n].ID = fmt.Sprintf("%064x", n)
		marker := fmt.Sprintf(" uniquecontent%05d", n)
		source := docs[n].Source
		docs[n].Source = source[:len(source)-len(marker)] + marker
	}
	return docs
}
