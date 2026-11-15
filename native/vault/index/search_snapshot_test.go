package vaultindex

import "testing"

func TestSearchSnapshotIsNarrowStableProjection(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "b.md", "---\ntitle: legacy B\ntype: reference\naliases: [Bee]\ntags: [two]\ncreated_at: 2026-01-02T03:04:05Z\nupdated_at: 2026-02-03T04:05:06Z\n---\n# B\n\n[[a]]\n")
	writeNote(t, root, "a.md", "# A\n\nbody\n")
	idx := New()
	if err := idx.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	snapshot := idx.SearchSnapshot()
	if len(snapshot) != 2 || snapshot[0].Path != "a.md" || snapshot[1].Path != "b.md" {
		t.Fatalf("search snapshot order/paths = %+v", snapshot)
	}
	if snapshot[1].Title != "B" || snapshot[1].Type != "reference" || snapshot[1].CreatedAt == "" || snapshot[1].UpdatedAt == "" || len(snapshot[1].Aliases) != 1 || snapshot[1].Aliases[0] != "Bee" || len(snapshot[1].Tags) != 1 || snapshot[1].Tags[0] != "two" {
		t.Fatalf("search metadata missing: %+v", snapshot[1])
	}
	// Search snapshots must not retain the unrelated semantic graph slices.
	full, ok := idx.Document("b.md")
	if !ok || len(full.Links) != 1 {
		t.Fatal("fixture did not contain a semantic link")
	}
	if snapshot[1].Aliases[0] == "changed" {
		t.Fatal("unexpected snapshot alias mutation")
	}
	snapshot[1].Aliases[0] = "changed"
	if got, _ := idx.Document("b.md"); got.Aliases[0] != "Bee" {
		t.Fatal("search snapshot leaked mutable aliases into index")
	}
}
