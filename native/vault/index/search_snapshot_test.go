package index

import (
	"context"
	"path/filepath"
	"testing"

	"muse/search"
)

func TestSearchSnapshotIsNarrowStableProjection(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "b.md", "---\ntitle: legacy B\ntype: reference\naliases: [Bee]\ntags: [two]\ncreated_at: 2026-01-02T03:04:05Z\nupdated_at: 2026-02-03T04:05:06Z\n---\n# B\n\n[[a]]\n")
	writeNote(t, root, "a.md", "# A\n\nbody\n")
	idx := testIndex(t, root)
	snapshot := idx.SearchSnapshot()
	if len(snapshot) != 2 || snapshot[0].Path != "a.md" || snapshot[1].Path != "b.md" {
		t.Fatalf("search snapshot order/paths = %+v", snapshot)
	}
	meta := snapshot[1].Metadata
	if snapshot[1].Title != "B" || meta.Type != "reference" || meta.CreatedAt == "" || meta.UpdatedAt == "" || len(meta.Aliases) != 1 || meta.Aliases[0] != "Bee" || len(meta.Tags) != 1 || meta.Tags[0] != "two" {
		t.Fatalf("search metadata missing: %+v", snapshot[1])
	}
	// Search snapshots must not retain the unrelated semantic graph slices.
	full, ok := idx.Document("b.md")
	if !ok || len(full.Links) != 1 {
		t.Fatal("fixture did not contain a semantic link")
	}
	if snapshot[1].Metadata.Aliases[0] == "changed" {
		t.Fatal("unexpected snapshot alias mutation")
	}
	snapshot[1].Metadata.Aliases[0] = "changed"
	if got, _ := idx.Document("b.md"); got.Aliases[0] != "Bee" {
		t.Fatal("search snapshot leaked mutable aliases into index")
	}
}

func TestSearchDocumentsImplementsSearchOwnedCorpusPort(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "note.md", "---\ntags: [work]\n---\n# Corpus\n")
	idx := testIndex(t, root)
	docs := idx.SearchDocuments()
	if len(docs) != 1 || docs[0].Path != "note.md" || docs[0].Title != "Corpus" || len(docs[0].Metadata.Tags) != 1 || docs[0].Metadata.Tags[0] != "work" || docs[0].ID == "" {
		t.Fatalf("SearchDocuments() = %#v", docs)
	}
	searchIndex, err := search.Open(filepath.Join(t.TempDir(), "search"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = searchIndex.Close() })
	if err := searchIndex.ReconcileCorpus(context.Background(), idx); err != nil {
		t.Fatal(err)
	}
	hits, err := searchIndex.Search(context.Background(), "tag:work", 5)
	if err != nil || len(hits) != 1 || hits[0].Path != "note.md" {
		t.Fatalf("ReconcileCorpus/typed Search = %#v, %v", hits, err)
	}
}
