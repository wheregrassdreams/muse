package doctypes

import "testing"

func TestVisibleWorkspacePath(t *testing.T) {
	for _, path := range []string{
		"note.md",
		"src/main.go",
		"README",
		"image.png",
		"diagram.svg",
	} {
		if !IsVisibleWorkspacePath(path) {
			t.Errorf("IsVisibleWorkspacePath(%q) = false, want true", path)
		}
	}
	for _, path := range []string{
		"payload.pdf",
		"archive.zip",
		"payload.unknown",
	} {
		if IsVisibleWorkspacePath(path) {
			t.Errorf("IsVisibleWorkspacePath(%q) = true, want false", path)
		}
	}
}
