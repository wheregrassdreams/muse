package vaultsearch_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"muse/vaultsearch"
)

func acceptanceIndex(t *testing.T, dir string) *vaultsearch.Index {
	t.Helper()
	i, err := vaultsearch.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i.Close() })
	return i
}

func requireSearchPaths(t *testing.T, i *vaultsearch.Index, query string, want ...string) {
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
	docs := []vaultsearch.Document{
		{ID: "a", Path: "a.md", Source: "firstneedle", Revision: "a1"},
		{ID: "b", Path: "b.md", Source: "secondneedle", Revision: "b1"},
	}
	if err := one.Reconcile(ctx, docs); err != nil {
		t.Fatal(err)
	}
	if err := two.Reconcile(ctx, []vaultsearch.Document{{ID: "a", Path: "other.md", Source: "otherneedle", Revision: "a1"}}); err != nil {
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

func TestMissingManifestCannotLeaveGhostDocuments(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "cache")
	i := acceptanceIndex(t, dir)
	if err := i.Reconcile(ctx, []vaultsearch.Document{{ID: "old", Path: "old.md", Source: "staleorphan", Revision: "1"}}); err != nil {
		t.Fatal(err)
	}
	if err := i.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	i = acceptanceIndex(t, dir)
	if err := i.Reconcile(ctx, []vaultsearch.Document{{ID: "new", Path: "new.md", Source: "freshcurrent", Revision: "1"}}); err != nil {
		t.Fatal(err)
	}
	requireSearchPaths(t, i, "staleorphan")
	requireSearchPaths(t, i, "freshcurrent", "new.md")
}

func TestMissingNativeDataDoesNotReuseOldManifest(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "cache")
	i := acceptanceIndex(t, dir)
	docs := []vaultsearch.Document{{ID: "one", Path: "one.md", Source: "recoverexistingnote", Revision: "same-revision"}}
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
