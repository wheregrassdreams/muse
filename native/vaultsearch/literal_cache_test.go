package vaultsearch

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode"
)

func TestLiteralProjectionBudgetAndPartialFallback(t *testing.T) {
	const sourceBytes = 1 << 20
	docs := make([]Document, 40)
	for n := range docs {
		docs[n] = Document{ID: fmt.Sprintf("doc-%02d", n), Path: fmt.Sprintf("%02d.md", n), Revision: "v1", Source: strings.Repeat("x", sourceBytes)}
	}
	projection, bytes, _, _, err := buildLiteralProjection(context.Background(), docs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes > literalProjectionBudget {
		t.Fatalf("projection payload %d exceeded budget %d", bytes, literalProjectionBudget)
	}
	if len(projection) >= len(docs) {
		t.Fatalf("budget should leave some docs uncached: cached %d of %d", len(projection), len(docs))
	}
	prepared := prepareLiteralProjections(docs, literalProjectionBudget)
	var preparedBytes int64
	for id, item := range prepared {
		preparedBytes += item.bytes(id)
	}
	if preparedBytes > literalProjectionBudget {
		t.Fatalf("temporary Apply projection payload %d exceeded budget %d", preparedBytes, literalProjectionBudget)
	}

	idx := openTestIndex(t)
	ctx := context.Background()
	indexed := []Document{
		{ID: "first", Path: "a.md", Source: "alpha phrase"},
		{ID: "middle", Path: "b.md", Source: "alpha phrase"},
		{ID: "last", Path: "c.md", Source: "alpha phrase"},
	}
	if err := idx.Reconcile(ctx, indexed); err != nil {
		t.Fatal(err)
	}
	// Exercise a partially populated bounded cache: uncached manifest entries
	// must still be read in batches and preserve global path ordering.
	idx.state.literalDocs = map[string]literalDoc{"first": makeLiteralDoc(indexed[0])}
	idx.state.literalBytes = idx.state.literalDocs["first"].bytes("first")
	hits, err := idx.Literal(ctx, "alpha", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 || hits[0].Path != "a.md" || hits[1].Path != "b.md" || hits[2].Path != "c.md" {
		t.Fatalf("partial cache lost hits or order: %+v", hits)
	}
}

func TestLiteralProjectionMatchesLegacyResults(t *testing.T) {
	ctx := context.Background()
	docs := []Document{
		{ID: "a", Path: "A.md", Revision: "r1", Source: "İstanbul! alpha+beta alpha+beta\n中文词语重复中文词语; A—B."},
		{ID: "b", Path: "B.md", Revision: "r2", Source: "alpha+beta, 中文词语。\n第二行跨token内容"},
		{ID: "c", Path: "C.md", Revision: "r3", Source: "shadowunique must disappear after overlay"},
	}
	cached, legacy := openTestIndex(t), openTestIndex(t)
	if err := cached.Reconcile(ctx, docs); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Reconcile(ctx, docs); err != nil {
		t.Fatal(err)
	}
	legacy.state.literalReady = false
	overlays := []Document{
		{ID: "b", Path: "B.md", Revision: "overlay-r2", Source: "overlay shadowunique + alpha+beta"},
		{ID: "z", Path: "Z.md", Revision: "overlay-z", Source: "new overlay: 中文词语 and İstanbul!"},
	}
	for _, query := range []string{"alpha+beta", "中文词语", "İstanbul", "A—B.", "shadowunique", "cross token"} {
		for _, limit := range []int{1, 2, 10} {
			got, err := cached.Literal(ctx, query, limit, overlays...)
			if err != nil {
				t.Fatalf("cached Literal(%q, %d): %v", query, limit, err)
			}
			want, err := legacy.Literal(ctx, query, limit, overlays...)
			if err != nil {
				t.Fatalf("legacy Literal(%q, %d): %v", query, limit, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Literal(%q, %d) differs:\n cached: %#v\n legacy: %#v", query, limit, got, want)
			}
		}
	}
	mutations := []Document{
		{ID: "b", Path: "A.md", Revision: "r4", Source: "alpha+beta moved alpha+beta"},
		{ID: "d", Path: "A.md", Revision: "r5", Source: "İstanbul! alpha+beta"},
	}
	for _, idx := range []*Index{cached, legacy} {
		if err := idx.Apply(ctx, mutations, []string{"c"}); err != nil {
			t.Fatal(err)
		}
	}
	pathChangeOverlays := []Document{
		{ID: "a", Path: "A.md", Revision: "overlay-a", Source: "alpha+beta overlay tie"},
		{ID: "z", Path: "A.md", Revision: "overlay-z", Source: "İstanbul! alpha+beta"},
	}
	for _, query := range []string{"alpha+beta", "İstanbul", "shadowunique"} {
		for _, limit := range []int{1, 3, 10} {
			got, err := cached.Literal(ctx, query, limit, pathChangeOverlays...)
			if err != nil {
				t.Fatalf("cached post-Apply Literal(%q, %d): %v", query, limit, err)
			}
			want, err := legacy.Literal(ctx, query, limit, pathChangeOverlays...)
			if err != nil {
				t.Fatalf("legacy post-Apply Literal(%q, %d): %v", query, limit, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("post-Apply Literal(%q, %d) differs:\n cached: %#v\n legacy: %#v", query, limit, got, want)
			}
		}
	}
	rename := Document{ID: "e", Path: "Renamed/Note.md", Revision: "r6", Source: "renamedunique"}
	for _, idx := range []*Index{cached, legacy} {
		if err := idx.Apply(ctx, []Document{rename}, []string{"b"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := cached.Literal(ctx, "renamedunique", 10)
	if err != nil {
		t.Fatal(err)
	}
	want, err := legacy.Literal(ctx, "renamedunique", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rename Literal differs: cached=%#v legacy=%#v", got, want)
	}
}

func TestLiteralOrderLargeBatchMerge(t *testing.T) {
	ctx := context.Background()
	const count = 256
	docs := make([]Document, count)
	for n := range docs {
		docs[n] = Document{ID: fmt.Sprintf("id-%04d", n), Path: fmt.Sprintf("Notes/%04d.md", n), Revision: "r1", Source: "commonphrase 中文内容"}
	}
	cached, legacy := openTestIndex(t), openTestIndex(t)
	for _, idx := range []*Index{cached, legacy} {
		if err := idx.Reconcile(ctx, docs); err != nil {
			t.Fatal(err)
		}
	}
	upserts := make([]Document, 128)
	for n := range upserts {
		upserts[n] = Document{ID: docs[n].ID, Path: fmt.Sprintf("Moved/%04d.md", 127-n), Revision: "r2", Source: "changedphrase 中文更新"}
	}
	deletes := make([]string, 64)
	for n := range deletes {
		deletes[n] = docs[count-1-n].ID
	}
	for _, idx := range []*Index{cached, legacy} {
		if err := idx.Apply(ctx, upserts, deletes); err != nil {
			t.Fatal(err)
		}
	}
	if !cached.state.literalOrderReady || len(cached.state.literalOrder) != len(cached.state.manifest.Docs) || cached.state.literalOrderBytes > literalOrderBudget {
		t.Fatalf("literal order invariant failed: ready=%v entries=%d manifest=%d bytes=%d", cached.state.literalOrderReady, len(cached.state.literalOrder), len(cached.state.manifest.Docs), cached.state.literalOrderBytes)
	}
	for n := 1; n < len(cached.state.literalOrder); n++ {
		if !literalOrderLess(cached.state.manifest.Docs, cached.state.literalOrder[n-1], cached.state.literalOrder[n]) {
			t.Fatalf("literal order is not strictly sorted at %d: %q then %q", n, cached.state.literalOrder[n-1], cached.state.literalOrder[n])
		}
	}
	for _, query := range []string{"commonphrase", "changedphrase", "中文更新"} {
		got, err := cached.Literal(ctx, query, 500)
		if err != nil {
			t.Fatal(err)
		}
		want, err := legacy.Literal(ctx, query, 500)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("large-batch Literal(%q) differs", query)
		}
	}
}

func TestLiteralOrderOversizedApplyRebuildsBoundedly(t *testing.T) {
	ctx := context.Background()
	const count = 1_100
	docs := make([]Document, count)
	for n := range docs {
		docs[n] = Document{ID: fmt.Sprintf("large-id-%04d", n), Path: fmt.Sprintf("Notes/%04d.md", n), Revision: "r1", Source: "stable-token"}
	}
	cached, legacy := openTestIndex(t), openTestIndex(t)
	for _, idx := range []*Index{cached, legacy} {
		if err := idx.Reconcile(ctx, docs); err != nil {
			t.Fatal(err)
		}
		if idx == legacy {
			idx.state.literalReady = false
		}
	}
	upserts := make([]Document, 1_025)
	for n := range upserts {
		upserts[n] = Document{ID: docs[n].ID, Path: fmt.Sprintf("Moved/%04d.md", len(upserts)-n), Revision: "r2", Source: "oversizedbatchtoken"}
	}
	deletes := make([]string, 10)
	for n := range deletes {
		deletes[n] = docs[count-1-n].ID
	}
	for _, idx := range []*Index{cached, legacy} {
		if err := idx.Apply(ctx, upserts, deletes); err != nil {
			t.Fatal(err)
		}
	}
	if !cached.state.literalOrderReady || len(cached.state.literalOrder) != len(cached.state.manifest.Docs) || cached.state.literalOrderBytes > literalOrderBudget {
		t.Fatalf("large Apply did not rebuild a bounded order: ready=%v entries=%d manifest=%d bytes=%d", cached.state.literalOrderReady, len(cached.state.literalOrder), len(cached.state.manifest.Docs), cached.state.literalOrderBytes)
	}
	got, err := cached.Literal(ctx, "oversizedbatchtoken", 500)
	if err != nil {
		t.Fatal(err)
	}
	want, err := legacy.Literal(ctx, "oversizedbatchtoken", 500)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("oversized Apply Literal differs: cached=%#v legacy=%#v", got, want)
	}
}

func TestLiteralOrderBudgetFallsBackAsAWhole(t *testing.T) {
	docs := make([]Document, int(literalOrderBudget/literalOrderEntryBytes)+1)
	for n := range docs {
		docs[n] = Document{ID: strings.Repeat("x", 64), Path: "same.md", Source: ""}
	}
	_, _, order, ready, err := buildLiteralProjection(context.Background(), docs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ready || order != nil {
		t.Fatalf("over-budget order must fall back wholesale: ready=%v len=%d", ready, len(order))
	}
}

func TestLiteralUncachedFallbackScansMultipleBatches(t *testing.T) {
	ctx := context.Background()
	docs := make([]Document, 300)
	for n := range docs {
		docs[n] = Document{ID: fmt.Sprintf("fallback-%03d", n), Path: fmt.Sprintf("Notes/%03d.md", n), Revision: "r1", Source: "fallbackbatchneedle"}
	}
	idx := openTestIndex(t)
	if err := idx.Reconcile(ctx, docs); err != nil {
		t.Fatal(err)
	}
	// Force all entries through the bounded Tantivy source fallback while
	// retaining the normal sorted manifest order.
	idx.state.literalDocs = map[string]literalDoc{}
	idx.state.literalBytes = 0
	hits, err := idx.Literal(ctx, "fallbackbatchneedle", len(docs))
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != len(docs) {
		t.Fatalf("fallback returned %d hits, want %d", len(hits), len(docs))
	}
	for n, hit := range hits {
		want := fmt.Sprintf("Notes/%03d.md", n)
		if hit.Path != want {
			t.Fatalf("hit %d path = %q, want %q", n, hit.Path, want)
		}
	}
}

func TestHanOnlyMatchesWhitespaceCollapsedReference(t *testing.T) {
	inputs := []string{
		"", "ascii only", "中文连续内容", "English 中文 mixed-日本語漢字", "中文---内容\n第二行",
		"😀中文 😀汉字😀", "  中文\t内容  ", "汉 A 汉 B汉 C",
	}
	for _, input := range inputs {
		var old strings.Builder
		for _, r := range input {
			if unicode.Is(unicode.Han, r) {
				old.WriteRune(r)
			} else if old.Len() > 0 {
				old.WriteByte(' ')
			}
		}
		want := strings.Join(strings.Fields(old.String()), " ")
		if got := hanOnly(input); got != want {
			t.Errorf("hanOnly(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLiteralProjectionFollowsApplyAndClose(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	docs := []Document{
		{ID: "a", Path: "a.md", Source: "oldphrase"},
		{ID: "b", Path: "b.md", Source: "deletephrase"},
	}
	if err := idx.Reconcile(ctx, docs); err != nil {
		t.Fatal(err)
	}
	updated := Document{ID: "a", Path: "a.md", Source: "newphrase", Revision: "v2"}
	if err := idx.Apply(ctx, []Document{updated}, []string{"b"}); err != nil {
		t.Fatal(err)
	}
	for query, want := range map[string]int{"oldphrase": 0, "deletephrase": 0, "newphrase": 1} {
		hits, err := idx.Literal(ctx, query, 10)
		if err != nil || len(hits) != want {
			t.Fatalf("Literal(%q) = %d hits, %v; want %d", query, len(hits), err, want)
		}
	}
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}
	if idx.state.literalDocs != nil || idx.state.literalBytes != 0 {
		t.Fatal("Close retained literal projection memory")
	}
	if idx.state.literalOrder != nil || idx.state.literalOrderBytes != 0 || idx.state.literalOrderReady {
		t.Fatal("Close retained literal order memory")
	}
}

func openTestIndex(t *testing.T) *Index {
	t.Helper()
	idx, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if idx.state != nil && !idx.state.closed {
			_ = idx.Close()
		}
	})
	return idx
}
