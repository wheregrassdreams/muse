package nativeui

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
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
	a.path = filepath.Join(t.TempDir(), "unsaved.md")
	a.loadDoc("# Unsaved note\n\nKeep this draft.\n")
	a.dirty = true
	a.propertiesOpen = true
	source := a.ed.Source()

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
	if a.propertiesOpen {
		t.Fatal("opening an image left the Markdown properties sidebar open")
	}
	if !tt.HasText("photo.jpg") {
		t.Fatalf("image document title missing: %q", tt.Texts())
	}
	if a.displayName() != "photo.jpg" || a.docPath() != path {
		t.Fatalf("image title path=%q display=%q", a.docPath(), a.displayName())
	}
	if a.path != filepath.Join(filepath.Dir(a.path), "unsaved.md") || a.ed.Source() != source || !a.dirty {
		t.Fatalf("image open lost dirty note: path=%q source=%q dirty=%v", a.path, a.ed.Source(), a.dirty)
	}
	if len(a.navHist) == 0 || a.navHist[len(a.navHist)-1] != path {
		t.Fatalf("image path missing from navigation history: %v", a.navHist)
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
	if !a.imageDocumentOpen() || a.ed.Source() != source || !a.dirty {
		t.Fatalf("closing global preview state: imageOpen=%v source=%q dirty=%v", a.imageDocumentOpen(), a.ed.Source(), a.dirty)
	}
}

func TestImageNavigationPreservesDirtyNoteAndRestoresImage(t *testing.T) {
	path := writeWorkflowJPEG(t)
	note := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(note, []byte("on disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.openPathNow(note)
	a.ed.SetSource("unsaved draft")
	a.dirty = true
	a.requestOpen(path)
	if !a.imageDocumentOpen() || a.ed.Source() != "unsaved draft" || !a.dirty {
		t.Fatalf("image open lost draft: open=%v source=%q dirty=%v", a.imageDocumentOpen(), a.ed.Source(), a.dirty)
	}
	a.nav(-1)
	if a.imageDocumentOpen() || a.path != note || a.ed.Source() != "unsaved draft" || !a.dirty {
		t.Fatalf("back navigation did not restore dirty note: imageOpen=%v path=%q source=%q dirty=%v", a.imageDocumentOpen(), a.path, a.ed.Source(), a.dirty)
	}
	a.nav(1)
	if !a.imageDocumentOpen() || a.imageDocumentPath() != path || a.ed.Source() != "unsaved draft" || !a.dirty {
		t.Fatalf("forward navigation did not restore image and draft: imageOpen=%v source=%q dirty=%v", a.imageDocumentOpen(), a.ed.Source(), a.dirty)
	}
}
