package nativeui

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/platform/filesystem/imagefiles"
)

func writeWorkflowJPEG(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "photo.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 48, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 48; x++ {
			img.Set(x, y, color.RGBA{R: 30, G: 140, B: 210, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, img, nil); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpenImagePathShowsJPEGInEditorAndPreservesDirtyDraft(t *testing.T) {
	path := writeWorkflowJPEG(t)
	a := New()
	a.imageLocalFiles = imagefiles.New()
	setDocumentPathForTest(a, filepath.Join(t.TempDir(), "unsaved.md"))
	a.loadDoc("# Unsaved note\n\nKeep this draft.\n")
	a.noteEditor().MarkDirty()
	a.propertiesPanel.Open()
	source := a.noteEditor().Source()

	a.openPathNow(path)
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	tt.Frame()
	if !a.imageDocumentOpen() || a.imageDocumentPath() != path {
		t.Fatalf("JPEG image document did not load: open=%v path=%q", a.imageDocumentOpen(), a.imageDocumentPath())
	}
	if a.imagePreviewOpen() {
		t.Fatal("opening an image path opened global preview")
	}
	if a.propertiesPanel.IsOpen() {
		t.Fatal("opening an image left the Markdown properties sidebar open")
	}
	if !tt.HasText("photo.jpg") {
		t.Fatalf("image document title missing: %q", tt.Texts())
	}
	if a.displayName() != "photo.jpg" || a.docPath() != path {
		t.Fatalf("image title path=%q display=%q", a.docPath(), a.displayName())
	}
	if a.documentPath() != filepath.Join(filepath.Dir(a.documentPath()), "unsaved.md") || a.noteEditor().Source() != source || !a.documentDirty() {
		t.Fatalf("image open lost dirty note: path=%q source=%q dirty=%v", a.documentPath(), a.noteEditor().Source(), a.documentDirty())
	}
	paths := a.navigationHistory.Paths()
	if len(paths) == 0 || paths[len(paths)-1] != path {
		t.Fatalf("image path missing from navigation history: %v", paths)
	}
	if err := tt.Click("全局预览"); err != nil {
		t.Fatalf("explicit preview action: %v", err)
	}
	tt.Frame()
	if !a.imagePreviewOpen() {
		t.Fatal("explicit image preview action did not open global preview")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if !a.imageDocumentOpen() || a.noteEditor().Source() != source || !a.documentDirty() {
		t.Fatalf("closing global preview state: imageOpen=%v source=%q dirty=%v", a.imageDocumentOpen(), a.noteEditor().Source(), a.documentDirty())
	}
}

func TestImageNavigationPreservesDirtyNoteAndRestoresImage(t *testing.T) {
	path := writeWorkflowJPEG(t)
	note := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(note, []byte("on disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	withDocumentPorts(t, a)
	a.imageLocalFiles = imagefiles.New()
	a.openPathNow(note)
	a.loadDoc("unsaved draft")
	a.noteEditor().MarkDirty()
	a.requestOpen(path)
	if !a.imageDocumentOpen() || a.noteEditor().Source() != "unsaved draft" || !a.documentDirty() {
		t.Fatalf("image open lost draft: open=%v source=%q dirty=%v", a.imageDocumentOpen(), a.noteEditor().Source(), a.documentDirty())
	}
	a.nav(-1)
	if a.imageDocumentOpen() || a.documentPath() != note || a.noteEditor().Source() != "unsaved draft" || !a.documentDirty() {
		t.Fatalf("back navigation did not restore dirty note: imageOpen=%v path=%q source=%q dirty=%v", a.imageDocumentOpen(), a.documentPath(), a.noteEditor().Source(), a.documentDirty())
	}
	a.nav(1)
	if !a.imageDocumentOpen() || a.imageDocumentPath() != path || a.noteEditor().Source() != "unsaved draft" || !a.documentDirty() {
		t.Fatalf("forward navigation did not restore image and draft: imageOpen=%v source=%q dirty=%v", a.imageDocumentOpen(), a.noteEditor().Source(), a.documentDirty())
	}
}
