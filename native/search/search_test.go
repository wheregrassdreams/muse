package search_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"muse/document/types"
	"muse/search"
)

func TestSearchChineseEnglishPrefixPathAndLiteralInput(t *testing.T) {
	i := acceptanceIndex(t, filepath.Join(t.TempDir(), "cache"))
	docs := []search.Document{
		{ID: "quantum", Path: "physics/quantum-mechanics.md", Title: "Quantum Mechanics", Source: "A compact note about particles.", Revision: "1"},
		{ID: "chinese", Path: "notes/法术.md", Title: "Archive", Source: "法术与修行。", Revision: "1"},
		{ID: "path-only", Path: "reference/oddfilename.md", Title: "Index Card", Source: "The body is unremarkable.", Revision: "1"},
		{ID: "typed", Path: "projects/architecture.md", Source: "A body with no type token.", Metadata: types.Metadata{Type: "research", CreatedAt: "2026-11-13T00:00:00Z"}, Revision: "1"},
	}
	if err := i.Reconcile(context.Background(), docs); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ query, path string }{
		{"quant", "physics/quantum-mechanics.md"},  // English title prefix.
		{"mechan", "physics/quantum-mechanics.md"}, // Token prefix in file path.
		{"法", "notes/法术.md"},                       // Single Han character via the Han-only ngram field.
		{"oddfile", "reference/oddfilename.md"},    // Filename search independent of title.
		{"research", "projects/architecture.md"},   // Core type participates in metadata search.
	} {
		hits, err := i.Search(context.Background(), tc.query, 10)
		if err != nil {
			t.Fatalf("Search(%q): %v", tc.query, err)
		}
		found := false
		for _, h := range hits {
			if h.Path == tc.path {
				found = true
			}
		}
		if !found {
			t.Errorf("Search(%q) = %#v, missing %q", tc.query, hits, tc.path)
		}
	}
	if _, err := i.Search(context.Background(), `not-a-parser:(OR * "`, 10); err != nil {
		t.Fatalf("query syntax characters must remain safe text: %v", err)
	}
}

func TestSearchChineseNgramCoversTitleBodyAndMetadata(t *testing.T) {
	i := acceptanceIndex(t, filepath.Join(t.TempDir(), "cache"))
	defer i.Close()
	docs := []search.Document{{
		ID: "han", Path: "notes/中文路径.md", Title: "中文标题检索",
		Source: "正文检索内容，包含跨分词片段。", Metadata: types.Metadata{Aliases: []string{"中文别名"},
			Tags: []string{"元数据标签"}, Type: "研究"}, Revision: "1",
	}}
	if err := i.Reconcile(context.Background(), docs); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"中文标题", "A中文标题检索B", "正文检索", "中文别名", "元数据标签", "研究"} {
		hits, err := i.Search(context.Background(), query, 10)
		if err != nil {
			t.Fatalf("Search(%q): %v", query, err)
		}
		if len(hits) != 1 || hits[0].Path != docs[0].Path {
			t.Errorf("Search(%q) = %#v, want %q", query, hits, docs[0].Path)
		}
	}
}

func TestSearchOverlayShadowsDiskAndCanceledAndClosedLifecycle(t *testing.T) {
	i := acceptanceIndex(t, filepath.Join(t.TempDir(), "cache"))
	ctx := context.Background()
	disk := search.Document{ID: "one", Path: "one.md", Title: "Old title", Source: "oldneedle", Revision: "1"}
	if err := i.Reconcile(ctx, []search.Document{disk}); err != nil {
		t.Fatal(err)
	}
	overlay := search.Document{ID: disk.ID, Path: disk.Path, Title: "New title", Source: "newneedle", Revision: "2"}
	if hits, err := i.Search(ctx, "oldneedle", 10, overlay); err != nil || len(hits) != 0 {
		t.Fatalf("overlay did not shadow stale disk document: %#v, %v", hits, err)
	}
	if hits, err := i.Search(ctx, "newneedle", 10, overlay); err != nil || len(hits) != 1 || hits[0].Revision != "2" {
		t.Fatalf("overlay was not searchable: %#v, %v", hits, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := i.Search(canceled, "newneedle", 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Search error = %v, want context.Canceled", err)
	}
	if err := i.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Search(ctx, "newneedle", 10); err == nil {
		t.Fatal("Search after Close succeeded")
	}
}

func TestNeedsReconcileMatchesManifestWithoutMutatingIt(t *testing.T) {
	i := acceptanceIndex(t, filepath.Join(t.TempDir(), "cache"))
	defer i.Close()
	docs := []search.Document{
		{ID: "one", Path: "one.md", Title: "One", Source: "one body", Revision: "rev-one"},
		{ID: "two", Path: "two.md", Title: "Two", Source: "two body", Revision: "rev-two"},
	}
	if needed, err := i.NeedsReconcile(docs); err != nil || !needed {
		t.Fatalf("empty index NeedsReconcile = %v, %v; want true", needed, err)
	}
	if err := i.Reconcile(context.Background(), docs); err != nil {
		t.Fatal(err)
	}
	if needed, err := i.NeedsReconcile(docs); err != nil || needed {
		t.Fatalf("unchanged index NeedsReconcile = %v, %v; want false", needed, err)
	}
	changed := append([]search.Document(nil), docs...)
	changed[1].Revision = "rev-two-new"
	if needed, err := i.NeedsReconcile(changed); err != nil || !needed {
		t.Fatalf("changed document NeedsReconcile = %v, %v; want true", needed, err)
	}
	if needed, err := i.NeedsReconcile(docs[:1]); err != nil || !needed {
		t.Fatalf("deleted document NeedsReconcile = %v, %v; want true", needed, err)
	}
	if _, err := i.NeedsReconcile([]search.Document{docs[0], docs[0]}); err == nil {
		t.Fatal("duplicate document IDs were accepted")
	}
}

func TestSearchSnippetIsSingleLineAndBounded(t *testing.T) {
	i := acceptanceIndex(t, filepath.Join(t.TempDir(), "cache"))
	source := strings.Repeat("prefix words\r\n", 20) + "needle " + strings.Repeat("tail words\n", 20)
	if err := i.Reconcile(context.Background(), []search.Document{{ID: "long", Path: "long.md", Source: source, Revision: "1"}}); err != nil {
		t.Fatal(err)
	}
	hits, err := i.Search(context.Background(), "needle", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("Search: %#v, %v", hits, err)
	}
	if strings.ContainsAny(hits[0].Snippet, "\r\n") {
		t.Fatalf("snippet contains a line break: %q", hits[0].Snippet)
	}
	if n := utf8.RuneCountInString(hits[0].Snippet); n > 160 {
		t.Fatalf("snippet has %d runes, want <= 160", n)
	}
}

func TestApplyUpsertWinsDeleteAndDoesNotMutateInput(t *testing.T) {
	i := acceptanceIndex(t, filepath.Join(t.TempDir(), "cache"))
	ctx := context.Background()
	doc := search.Document{ID: "same", Path: "same.md", Title: "Title", Source: "applyneedle"}
	if err := i.Apply(ctx, []search.Document{doc}, nil); err != nil {
		t.Fatal(err)
	}
	if doc.Revision != "" {
		t.Fatalf("Apply mutated caller's upsert Revision: %q", doc.Revision)
	}
	if err := i.Apply(ctx, []search.Document{doc}, []string{"same"}); err != nil {
		t.Fatal(err)
	}
	hits, err := i.Search(ctx, "applyneedle", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("upsert should win when ID is also deleted: %#v, %v", hits, err)
	}
}

func TestSameDirectoryOpenCannotRecoverActiveIndex(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	i := acceptanceIndex(t, dir)
	ctx := context.Background()
	if err := i.Reconcile(ctx, []search.Document{{ID: "live", Path: "live.md", Source: "activeindexneedle", Revision: "1"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := openSearchTestIndex(dir); err == nil || !strings.Contains(err.Error(), "index in use") {
		t.Fatalf("second Open error = %v, want index in use", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dirty.marker"), []byte("interrupted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openSearchTestIndex(dir); err == nil || !strings.Contains(err.Error(), "index in use") {
		t.Fatalf("Open with active dirty marker error = %v, want index in use", err)
	}
	hits, err := i.Search(ctx, "activeindexneedle", 10)
	if err != nil || len(hits) != 1 || hits[0].Path != "live.md" {
		t.Fatalf("active index changed during rejected recovery: %#v, %v", hits, err)
	}
	quarantined, err := filepath.Glob(filepath.Join(dir, "tantivy.corrupt-*"))
	if err != nil || len(quarantined) != 0 {
		t.Fatalf("active index was quarantined: %v, %v", quarantined, err)
	}
}

func TestUnknownManifestVersionIsQuarantinedBeforeRebuild(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	i := acceptanceIndex(t, dir)
	ctx := context.Background()
	if err := i.Reconcile(ctx, []search.Document{{ID: "old", Path: "old.md", Source: "manifestoldneedle", Revision: "1"}}); err != nil {
		t.Fatal(err)
	}
	if err := i.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"version":999,"docs":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	i = acceptanceIndex(t, dir)
	if err := i.Reconcile(ctx, []search.Document{{ID: "new", Path: "new.md", Source: "manifestnewneedle", Revision: "1"}}); err != nil {
		t.Fatal(err)
	}
	if hits, err := i.Search(ctx, "manifestoldneedle", 10); err != nil || len(hits) != 0 {
		t.Fatalf("old schema data survived recovery: %#v, %v", hits, err)
	}
	if hits, err := i.Search(ctx, "manifestnewneedle", 10); err != nil || len(hits) != 1 || hits[0].Path != "new.md" {
		t.Fatalf("new document missing after recovery: %#v, %v", hits, err)
	}
	preserved, err := filepath.Glob(filepath.Join(dir, "manifest.json.corrupt-*"))
	if err != nil || len(preserved) != 1 {
		t.Fatalf("invalid manifest not preserved in quarantine: %v, %v", preserved, err)
	}
}

type cancelAfterChecks struct {
	context.Context
	checks   int
	cancelAt int
}

func (c *cancelAfterChecks) Err() error {
	c.checks++
	if c.checks >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

func TestPartialBatchFailurePoisonsAndRebuildsOnOpen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	i := acceptanceIndex(t, dir)
	// The upsert batch cap is 1024, so this exercises cancellation after the
	// first batch has committed and before the second batch starts.
	docs := make([]search.Document, 1025)
	for n := range docs {
		docs[n] = search.Document{ID: fmt.Sprintf("doc-%03d", n), Path: fmt.Sprintf("%03d.md", n), Source: fmt.Sprintf("batchtoken-%03d", n), Revision: "r1"}
	}
	// One initial check, one per input document, one before dirty-marker
	// creation, and one at the start of each native batch.
	ctx := &cancelAfterChecks{Context: context.Background(), cancelAt: len(docs) + 4}
	err := i.Apply(ctx, docs, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Apply error = %v, want context.Canceled", err)
	}
	if ctx.checks != len(docs)+4 {
		t.Fatalf("canceled after %d context checks, want partial second-batch failure", ctx.checks)
	}
	if _, err := os.Stat(filepath.Join(dir, "dirty.marker")); err != nil {
		t.Fatalf("partial commit marker missing: %v", err)
	}
	if _, err := i.Search(context.Background(), "batchtoken", 10); err == nil {
		t.Fatal("poisoned index allowed a query after partial commit")
	}
	if err := i.Close(); err != nil {
		t.Fatal(err)
	}
	i = acceptanceIndex(t, dir)
	if err := i.Reconcile(context.Background(), docs); err != nil {
		t.Fatal(err)
	}
	hits, err := i.Search(context.Background(), "batchtoken-1024", 10)
	if err != nil || len(hits) != 1 || hits[0].Path != "1024.md" {
		t.Fatalf("reconciled post-recovery index: %#v, %v", hits, err)
	}
}
