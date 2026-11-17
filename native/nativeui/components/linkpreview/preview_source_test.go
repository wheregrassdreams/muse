package linkpreview

import (
	"strings"
	"testing"
)

func TestPreviewSourceRemovesFrontmatterAndSelectsHeadingSection(t *testing.T) {
	source := "---\naliases: [Preview]\n---\n# Atlas\n\n## Architecture\n\nBody with [[Projects/Target#Details|a link]] and ![[Assets/diagram.png]].\n\n## Next\nDo not include this section.\n"
	got := PreviewSource(source, strings.Index(source, "Architecture"))
	if strings.Contains(got, "aliases:") || strings.Contains(got, "## Next") {
		t.Fatalf("preview retained frontmatter or a following section: %q", got)
	}
	if !strings.HasPrefix(got, "## Architecture") {
		t.Fatalf("preview did not start at target heading: %q", got)
	}
	if !strings.Contains(got, "**a link**") || !strings.Contains(got, "diagram") || strings.Contains(got, "[[") {
		t.Fatalf("preview did not flatten wiki references: %q", got)
	}
}

func TestPreviewSourceWithoutHeadingKeepsDocumentBody(t *testing.T) {
	source := "---\ntitle: Example\n---\nParagraph with [[Folder/Target.md]] and ![[image.png]]."
	got := PreviewSource(source, 0)
	if strings.Contains(got, "title:") || got != "Paragraph with **Target** and image." {
		t.Fatalf("PreviewSource() = %q", got)
	}
}
