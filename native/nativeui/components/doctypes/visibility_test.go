package doctypes

import "testing"

func TestEditableFileTypes(t *testing.T) {
	for _, path := range []string{"a.md", "a.txt", "a.json", "a.yaml", "a.go", "README", "LICENSE"} {
		if !IsEditablePath(path) {
			t.Errorf("%q should be editable", path)
		}
	}
	for _, path := range []string{"a.png", "a.jpg", "a.pdf", "a.zip", "a.exe", "a.bin", "a.docx"} {
		if IsEditablePath(path) {
			t.Errorf("%q should not be editable", path)
		}
	}
}

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

func TestMarkdownPathUsesRichNoteParser(t *testing.T) {
	for _, path := range []string{"", "README", "note.md", "note.markdown", "note.mdx", "note.mdown"} {
		if !IsMarkdownPath(path) {
			t.Errorf("IsMarkdownPath(%q) = false, want true", path)
		}
	}
	for _, path := range []string{"main.go", "notes.txt", "image.png"} {
		if IsMarkdownPath(path) {
			t.Errorf("IsMarkdownPath(%q) = true, want false", path)
		}
	}
}
