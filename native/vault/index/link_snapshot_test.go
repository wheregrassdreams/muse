package index

import "testing"

func TestLinkSnapshotIsDetachedStructureProjection(t *testing.T) {
	idx := testIndex(t, t.TempDir())
	if err := idx.UpdateText("notes/source.md", "---\naliases: [Alias]\n---\n# Heading\n[[Target]]\n"); err != nil {
		t.Fatal(err)
	}
	first := idx.LinkSnapshot()
	if len(first) != 1 || first[0].Path != "notes/source.md" || first[0].Title != "Heading" || len(first[0].Headings) != 1 || len(first[0].Links) != 1 {
		t.Fatalf("projection = %+v", first)
	}
	first[0].Aliases[0] = "mutated"
	first[0].Headings[0].Text = "mutated"
	first[0].Links[0].Target = "mutated"
	next, ok := idx.LinkDocument("notes/source.md")
	if !ok || next.Aliases[0] != "Alias" || next.Headings[0].Text != "Heading" || next.Links[0].Target != "Target" {
		t.Fatalf("projection exposed mutable storage: %+v", next)
	}
}
