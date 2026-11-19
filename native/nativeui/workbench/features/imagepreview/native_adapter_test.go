package imagepreview

import (
	"path/filepath"
	"testing"
)

func TestDisplayLabelKeepsURLsAndRelativizesLocalPaths(t *testing.T) {
	if got := DisplayLabel("https://example.test/image.png", "/vault/notes"); got != "https://example.test/image.png" {
		t.Fatalf("URL label=%q", got)
	}
	base := t.TempDir()
	path := filepath.Join(base, "assets", "image.png")
	if got := DisplayLabel(path, base); got != "assets/image.png" {
		t.Fatalf("local label=%q, want assets/image.png", got)
	}
}
