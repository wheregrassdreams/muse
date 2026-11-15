package vaultsearchindexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	applicationvault "muse/application/vault"
	vaultindexsource "muse/platform/filesystem/vaultindexsource"
	vaultsearchstore "muse/platform/filesystem/vaultsearchstore"
	vaultindex "muse/vault/index"
)

func TestRebuildCreatesDurableIndexWithoutOpeningUI(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("# Worker note\n\nworker-only-search-token"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := vaultsearchstore.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	reconciler := New(store, newIndex)
	if err := reconciler.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	searchIndex, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = searchIndex.Close() })
	hits, err := searchIndex.Search(context.Background(), "worker-only-search-token", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Path != "note.md" {
		t.Fatalf("rebuilt index hits = %+v", hits)
	}
}

func TestOpenAndReconcileReturnsReopenedIndexAfterWorker(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("# Fresh note\n\nreconciler-fresh-token"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := vaultsearchstore.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	reconciler := New(store, newIndex)
	// This test seam runs the same child operation in process. Production New
	// always uses spawnWorker and does not fall back to this strategy.
	reconciler.runWorker = func(context.Context, string) error { return reconciler.Rebuild(root) }
	corpus, err := newIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := corpus.Rebuild(); err != nil {
		t.Fatal(err)
	}
	opened, err := reconciler.OpenAndReconcile(context.Background(), root, corpus)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	hits, err := opened.Search(context.Background(), "reconciler-fresh-token", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Path != "note.md" {
		t.Fatalf("reconciled hits = %+v", hits)
	}
}

func newIndex(root string) (applicationvault.IndexPort, error) {
	source, err := vaultindexsource.New(root)
	if err != nil {
		return nil, err
	}
	return vaultindex.New(source), nil
}

var _ applicationvault.IndexFactory = newIndex
