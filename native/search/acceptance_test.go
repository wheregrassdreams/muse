package search_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"muse/document/types"
	"muse/platform/filesystem/vaultsearchstore"
	"muse/search"
	"muse/search/rank"
)

func acceptanceIndex(t *testing.T, dir string) *search.Index {
	t.Helper()
	i, err := openSearchTestIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i.Close() })
	return i
}

func openSearchTestIndex(dir string) (*search.Index, error) {
	storage, err := vaultsearchstore.NewAt(dir)
	if err != nil {
		return nil, err
	}
	return search.OpenWithStorage(storage)
}

func requireSearchPaths(t *testing.T, i *search.Index, query string, want ...string) {
	t.Helper()
	hits, err := i.Search(context.Background(), query, 50)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool)
	for _, h := range hits {
		got[h.Path] = true
	}
	if len(got) != len(want) {
		t.Fatalf("query %q: paths %v, want %v", query, got, want)
	}
	for _, p := range want {
		if !got[p] {
			t.Fatalf("query %q: missing %q in %v", query, p, got)
		}
	}
}

func TestIndependentVaultsAndIncrementalReconcile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	one := acceptanceIndex(t, filepath.Join(dir, "one"))
	two := acceptanceIndex(t, filepath.Join(dir, "two"))
	docs := []search.Document{
		{ID: "a", Path: "a.md", Source: "firstneedle", Revision: "a1"},
		{ID: "b", Path: "b.md", Source: "secondneedle", Revision: "b1"},
	}
	if err := one.Reconcile(ctx, docs); err != nil {
		t.Fatal(err)
	}
	if err := two.Reconcile(ctx, []search.Document{{ID: "a", Path: "other.md", Source: "otherneedle", Revision: "a1"}}); err != nil {
		t.Fatal(err)
	}
	requireSearchPaths(t, one, "firstneedle", "a.md")
	requireSearchPaths(t, two, "firstneedle")
	if err := one.Reconcile(ctx, docs); err != nil {
		t.Fatal(err)
	}
	docs[0].Source, docs[0].Revision = "replacementneedle", "a2"
	if err := one.Reconcile(ctx, docs); err != nil {
		t.Fatal(err)
	}
	requireSearchPaths(t, one, "secondneedle", "b.md")
	requireSearchPaths(t, one, "replacementneedle", "a.md")
	if err := one.Close(); err != nil {
		t.Fatal(err)
	}
	one = acceptanceIndex(t, filepath.Join(dir, "one"))
	if err := one.Reconcile(ctx, docs[:1]); err != nil {
		t.Fatal(err)
	}
	requireSearchPaths(t, one, "secondneedle")
	requireSearchPaths(t, two, "otherneedle", "other.md")
}

type preferPathRanker struct{ path string }

func (r preferPathRanker) ID() string { return "prefer-path" }
func (r preferPathRanker) Score(candidate rank.Context) float64 {
	if candidate.Path == r.path {
		return 100
	}
	return 0
}

func TestStructuredSearchExecutesFiltersAndRegisteredRanker(t *testing.T) {
	ctx := context.Background()
	i := acceptanceIndex(t, filepath.Join(t.TempDir(), "cache"))
	docs := []search.Document{
		{ID: "first", Path: "a/first.md", Title: "Migration plan", Source: "migration details", Revision: "1", Metadata: types.Metadata{Type: "research", CreatedAt: "2026-11-13", Tags: []string{"deep work"}}},
		{ID: "second", Path: "z/second.md", Title: "Migration notes", Source: "migration details", Revision: "1", Metadata: types.Metadata{Type: "research", CreatedAt: "2026-11-14", Tags: []string{"deep work", "private"}}},
		{ID: "third", Path: "third.md", Title: "Migration archive", Source: "migration details", Revision: "1", Metadata: types.Metadata{Type: "reference", CreatedAt: "2026-11-14", Tags: []string{"deep work"}}},
	}
	if err := i.Reconcile(ctx, docs); err != nil {
		t.Fatal(err)
	}
	rankers := rank.NewRegistry()
	if err := rankers.Register(preferPathRanker{path: "z/second.md"}); err != nil {
		t.Fatal(err)
	}
	options := search.QueryOptions{Rankers: rankers}
	hits, err := i.SearchQuery(ctx, `migration tag:"deep work" type=research created>=2026-11-01 -tag:private`, 10, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Path != "a/first.md" {
		t.Fatalf("filters did not apply as an AND plan: %#v", hits)
	}
	hits, err = i.SearchQuery(ctx, `tag:"deep work" type=research created>=2026-11-01`, 10, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].Path != "z/second.md" {
		t.Fatalf("registered ranker did not affect candidate order: %#v", hits)
	}
	hits, err = i.Search(ctx, `tag:"deep work" type=research`, 10)
	if err != nil || len(hits) != 2 {
		t.Fatalf("public Search facade did not execute typed filters: %#v, %v", hits, err)
	}
}

func TestMissingManifestCannotLeaveGhostDocuments(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "cache")
	i := acceptanceIndex(t, dir)
	if err := i.Reconcile(ctx, []search.Document{{ID: "old", Path: "old.md", Source: "staleorphan", Revision: "1"}}); err != nil {
		t.Fatal(err)
	}
	if err := i.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	i = acceptanceIndex(t, dir)
	if err := i.Reconcile(ctx, []search.Document{{ID: "new", Path: "new.md", Source: "freshcurrent", Revision: "1"}}); err != nil {
		t.Fatal(err)
	}
	requireSearchPaths(t, i, "staleorphan")
	requireSearchPaths(t, i, "freshcurrent", "new.md")
}

func TestMissingNativeDataDoesNotReuseOldManifest(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "cache")
	i := acceptanceIndex(t, dir)
	docs := []search.Document{{ID: "one", Path: "one.md", Source: "recoverexistingnote", Revision: "same-revision"}}
	if err := i.Reconcile(ctx, docs); err != nil {
		t.Fatal(err)
	}
	if err := i.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "tantivy")); err != nil {
		t.Fatal(err)
	}
	i = acceptanceIndex(t, dir)
	if err := i.Reconcile(ctx, docs); err != nil {
		t.Fatal(err)
	}
	requireSearchPaths(t, i, "recoverexistingnote", "one.md")
}
