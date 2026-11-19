package document

import "testing"

func TestInspectDocumentPropertiesReturnsDetachedFrontmatterAndBodyFacts(t *testing.T) {
	const source = "---\ntags: [work, notes]\n---\n# Heading\n\nbody words\n"
	snapshot := InspectDocumentProperties(source, "/vault/note.md", nil)
	if snapshot.Frontmatter.Err != nil || len(snapshot.Frontmatter.Fields) != 1 {
		t.Fatalf("frontmatter snapshot = %+v", snapshot.Frontmatter)
	}
	if snapshot.Facts.BodyWords != 4 {
		t.Fatalf("body words = %d, want 4", snapshot.Facts.BodyWords)
	}
	if snapshot.YAMLSource != "---\ntags: [work, notes]\n---\n" {
		t.Fatalf("YAML source = %q", snapshot.YAMLSource)
	}
}
