package nativeui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"muse/locales"
	"muse/nativeui/workbench/features/imagepreview"
	mdoc "muse/nativeui/workbench/features/markdown"
	"muse/platform/filesystem/imagefiles"
)

func writePreviewPNG(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "preview.png")
	img := image.NewRGBA(image.Rect(0, 0, 32, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: 40, G: 120, B: 220, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestImagePreviewShowsImageAndZoomControls(t *testing.T) {
	path := writePreviewPNG(t)
	a := New()
	a.imageLocalFiles = imagefiles.New()
	a.loadDoc("note\n" + strings.Repeat("entry\n", 100))
	a.noteEditor().FocusCommands().Focus()
	a.openImagePreview(path)
	tt := ui.NewTester(a.View, 900, 600)
	deadline := time.Now().Add(2 * time.Second)
	for (!tt.HasText("preview.png") || !tt.HasText("100%")) && time.Now().Before(deadline) {
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
	tt.Frame()
	if !a.imagePreviewOpen() {
		t.Fatalf("image preview did not open %q", path)
	}
	if !tt.HasText("preview.png") || !tt.HasText("100%") {
		t.Fatalf("preview overlay text = %q", tt.Texts())
	}
	writeImageUIScreenshot(t, tt, "image-preview")
	tt.Wheel(450, 300, 0, -40)
	tt.Frame()
	if a.noteEditor().NavigationCommands().ScrollY() != 0 {
		t.Fatalf("wheel up scrolled the note to %v", a.noteEditor().NavigationCommands().ScrollY())
	}
	tt.Wheel(450, 300, 0, 40)
	tt.Frame()
	if a.noteEditor().NavigationCommands().ScrollY() != 0 {
		t.Fatalf("wheel down scrolled the note to %v", a.noteEditor().NavigationCommands().ScrollY())
	}
	tt.Scroll(450, 300, 18, -24)
	tt.Frame()
	tt.Pinch(450, 300, 1.5)
	tt.Frame()
	tt.Type("accidental")
	if got := a.noteEditor().Source(); got != "note\n"+strings.Repeat("entry\n", 100) {
		t.Fatalf("typing with preview open changed note source: %q", got)
	}
	if err := tt.Click("放大图片"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Press(450, 300)
	tt.Move(490, 330)
	tt.Frame()
	tt.Release(490, 330)
	tt.Press(450, 300)
	tt.CancelPress()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.imagePreviewOpen() {
		t.Fatal("Esc left the image preview open")
	}
	tt.Frame()
	if !a.noteEditor().FocusCommands().Focused() {
		t.Fatal("closing the preview did not restore editor focus")
	}
}

func TestImagePreviewUsesSharedMissingImageMessage(t *testing.T) {
	dir := t.TempDir()
	a := New()
	setDocumentPathForTest(a, filepath.Join(dir, "note.md"))
	src := filepath.Join(dir, "附件", "missing.png")
	a.openImagePreview(src)
	tt := ui.NewTester(a.View, 900, 600)
	tt.Frame()
	want := mdoc.MissingImageMessage("附件/missing.png")
	if !tt.HasText(want) {
		t.Fatalf("missing-image preview = %q, want %q", tt.Texts(), want)
	}
	if dir := os.Getenv("MISSING_IMAGE_SHOTS"); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(dir, "image-preview.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, tt.Image()); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFullImagePreviewAdvancesAnimatedGIFFrames(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "testdata", "vault-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "Attachments", "animation.gif")
	a := New()
	a.imageLocalFiles = imagefiles.New()
	a.root = root
	setDocumentPathForTest(a, filepath.Join(root, "Home.md"))
	a.openImagePreview(src)
	tt := ui.NewTester(a.View, 900, 600)
	deadline := time.Now().Add(2 * time.Second)
	for !tt.HasText("animation.gif") && time.Now().Before(deadline) {
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
	tt.Frame()
	first := append([]byte(nil), tt.Image().Pix...)
	tt.Advance(750 * time.Millisecond)
	second := tt.Image().Pix
	if bytes.Equal(first, second) {
		t.Fatal("full-screen GIF preview did not repaint with a later frame")
	}
}

func TestImagePreviewZoomClamp(t *testing.T) {
	if got := imagepreview.ClampZoom(0); got != float32(0.1) {
		t.Errorf("lower clamp = %v, want %v", got, float32(0.1))
	}
	if got := imagepreview.ClampZoom(100); got != float32(8) {
		t.Errorf("upper clamp = %v, want %v", got, float32(8))
	}
}

func TestImageInitialScaleFitsOnlyOversizedImages(t *testing.T) {
	if got := imagepreview.InitialScale(300, 200, 600, 400); got != 1 {
		t.Fatalf("small image initial scale = %v, want native 1", got)
	}
	if got := imagepreview.InitialScale(1200, 600, 600, 400); got != .5 {
		t.Fatalf("wide image initial scale = %v, want .5", got)
	}
	if got := imagepreview.InitialScale(400, 1200, 600, 600); got != .5 {
		t.Fatalf("tall image initial scale = %v, want .5", got)
	}
}

func TestImageDocumentCanvasZoomPanAndFullscreen(t *testing.T) {
	path := writePreviewPNG(t)
	a := New()
	a.imageLocalFiles = imagefiles.New()
	a.openImageDocument(path, false)
	tt := ui.NewTester(a.View, 900, 600)
	tt.Frame()
	if !tt.HasText("100%") {
		t.Fatalf("native-size image page did not show 100%%: %q", tt.Texts())
	}
	writeImageUIScreenshot(t, tt, "image-document")
	tt.Wheel(450, 300, 0, -40)
	tt.Frame()
	tt.Scroll(450, 300, 20, -25)
	tt.Frame()
	tt.Pinch(450, 300, 1.5)
	tt.Frame()
	tt.Press(450, 300)
	tt.Move(480, 320)
	tt.Frame()
	tt.Release(480, 320)
	if err := tt.Click(locales.ImagePreviewTitle()); err != nil {
		t.Fatalf("click fullscreen preview: %v", err)
	}
	tt.Frame()
	if !a.imagePreviewOpen() {
		t.Fatal("fullscreen control did not open image preview")
	}
}

func TestImageDocumentBacklinksUseImageTarget(t *testing.T) {
	a, tt, _ := runtimeFixtureWithNotes(t, map[string]string{
		"Source.md": "A referenced attachment: ![[Attachments/diagram.png]]",
	})
	imagePath := filepath.Join(a.root, "Attachments", "diagram.png")
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(writePreviewPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imagePath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	a.openImageDocument(imagePath, false)
	tt.Frame()
	if got := a.documentBacklinkSources(); len(got) != 1 || got[0].Source != "Source.md" {
		t.Fatalf("image backlinks = %+v, want Source.md", got)
	}
	if !tt.HasText(locales.PropertyViewBacklinks()) || !tt.HasText(locales.PropertyBacklinkCount(1)) {
		t.Fatalf("image backlink control missing from page: %q", tt.Texts())
	}
	writeImageUIScreenshot(t, tt, "image-document-backlinks")
	tt.SetDark(true)
	tt.Frame()
	writeImageUIScreenshot(t, tt, "image-document-backlinks-dark")
	tt.SetDark(false)
	tt.Frame()
	if err := tt.Click(locales.PropertyViewBacklinks()); err != nil {
		t.Fatalf("click image backlinks control: %v", err)
	}
	tt.Frame()
	if !a.vaultSnapshot.LinksOpen {
		t.Fatal("image backlinks control did not open the backlinks view")
	}
}

func writeImageUIScreenshot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("MUSE_IMAGE_UI_SHOTS")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, tt.Image()); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
