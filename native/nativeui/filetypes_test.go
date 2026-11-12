package nativeui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestEditableFileTypes(t *testing.T) {
	for _, p := range []string{"a.md", "a.txt", "a.json", "a.yaml", "a.go", "README", "LICENSE"} {
		if !isEditableFile(p) {
			t.Errorf("%q should be editable", p)
		}
	}
	for _, p := range []string{"a.png", "a.jpg", "a.pdf", "a.zip", "a.exe", "a.bin", "a.docx"} {
		if isEditableFile(p) {
			t.Errorf("%q should not be editable", p)
		}
	}
}

func TestChildrenShowsImagesButHidesOtherBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.md", "b.png", "c.json", "d.pdf"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := New()
	got := a.children(dir)
	want := map[string]bool{
		filepath.Join(dir, "sub"):    true,
		filepath.Join(dir, "a.md"):   true,
		filepath.Join(dir, "b.png"):  true,
		filepath.Join(dir, "c.json"): true,
	}
	if len(got) != len(want) {
		t.Fatalf("children = %v", got)
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected child %q", p)
		}
	}
}

func TestCommonImagesAreVisibleButNotEditable(t *testing.T) {
	for _, ext := range []string{"png", "jpg", "jpeg", "gif", "bmp", "webp", "svg", "avif"} {
		path := "image." + ext
		if !isImageFile(path) || !isVisibleWorkspaceFile(path) {
			t.Errorf("%q should be shown in the file tree", path)
		}
		if isEditableFile(path) {
			t.Errorf("%q must not be opened as editor text", path)
		}
	}
	for _, ext := range []string{"pdf", "zip", "exe", "bin", "docx"} {
		if isVisibleWorkspaceFile("payload." + ext) {
			t.Errorf("non-image binary %q should remain hidden", ext)
		}
	}
}

func TestOpenRejectsBinaryData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake.txt")
	if err := os.WriteFile(path, []byte{'a', 0, 'b'}, 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.source = "keep"
	a.openPathNow(path)
	if a.source != "keep" {
		t.Fatalf("source = %q", a.source)
	}
}

func TestOpenRejectsBinaryExtension(t *testing.T) {
	if isEditableFile("image.png") {
		t.Fatal("image extensions must never be editable text")
	}
}

func TestFileTreeImageClickOpensImageDocumentAndKeepsDirtyNote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(path, []byte{0, 1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.setWorkspace(dir)
	a.path = "existing.md"
	a.loadDoc("preserve me")
	a.dirty = true
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !tt.HasText("photo.png") {
		t.Fatalf("image missing from tree: %q", tt.Texts())
	}
	if err := tt.Click("photo.png"); err != nil {
		t.Fatalf("click image: %v", err)
	}
	if !a.imageDocument.open || a.imageDocument.path != path {
		t.Fatalf("image document = %+v, want open %q", a.imageDocument, path)
	}
	if a.imagePreview.open {
		t.Fatal("opening image from file tree opened global preview")
	}
	if a.path != "existing.md" || a.ed.Source() != "preserve me" || !a.dirty {
		t.Fatalf("opening image changed dirty editor document: path=%q source=%q dirty=%v", a.path, a.ed.Source(), a.dirty)
	}
	if a.displayName() != "photo.png" || a.docPath() != "photo.png" {
		t.Fatalf("image title path=%q display=%q", a.docPath(), a.displayName())
	}
	if !tt.HasText("无法解码此图片格式") {
		t.Fatal("invalid image bytes were not handled by the image document")
	}
}
