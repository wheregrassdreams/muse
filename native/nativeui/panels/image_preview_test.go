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
	"muse/mdoc"
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
	a.ed.SetSource("note\n" + strings.Repeat("entry\n", 100))
	a.ed.Focus()
	a.openImagePreview(path)
	tt := ui.NewTester(a.View, 900, 600)
	deadline := time.Now().Add(2 * time.Second)
	for a.imagePreview.bmp == nil && time.Now().Before(deadline) {
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
	tt.Frame()
	if !a.imagePreview.open || a.imagePreview.bmp == nil {
		t.Fatalf("image preview did not load %q: %+v", path, a.imagePreview)
	}
	if !tt.HasText("preview.png") || !tt.HasText("100%") {
		t.Fatalf("preview overlay text = %q", tt.Texts())
	}
	writeImageUIScreenshot(t, tt, "image-preview")
	before := a.imagePreview.zoom
	tt.Wheel(450, 300, 0, -40)
	tt.Frame()
	if a.imagePreview.zoom <= before {
		t.Fatalf("wheel up zoom = %v, want > %v", a.imagePreview.zoom, before)
	}
	zoomed := a.imagePreview.zoom
	if a.ed.ScrollY() != 0 {
		t.Fatalf("wheel up scrolled the note to %v", a.ed.ScrollY())
	}
	tt.Wheel(450, 300, 0, 40)
	tt.Frame()
	if a.imagePreview.zoom >= zoomed {
		t.Fatalf("wheel down zoom = %v, want < %v", a.imagePreview.zoom, zoomed)
	}
	if a.ed.ScrollY() != 0 {
		t.Fatalf("wheel down scrolled the note to %v", a.ed.ScrollY())
	}
	beforePanZoom := a.imagePreview.zoom
	tt.Scroll(450, 300, 18, -24)
	tt.Frame()
	if a.imagePreview.zoom != beforePanZoom || a.imagePreview.x == 0 || a.imagePreview.y == 0 {
		t.Fatalf("precise touchpad scroll = zoom %v, pan (%v,%v); want pan without zoom", a.imagePreview.zoom, a.imagePreview.x, a.imagePreview.y)
	}
	beforePinch := a.imagePreview.zoom
	tt.Pinch(450, 300, 1.5)
	tt.Frame()
	if a.imagePreview.zoom <= beforePinch {
		t.Fatalf("touchpad pinch zoom = %v, want > %v", a.imagePreview.zoom, beforePinch)
	}
	tt.Type("accidental")
	if got := a.ed.Source(); got != "note\n"+strings.Repeat("entry\n", 100) {
		t.Fatalf("typing with preview open changed note source: %q", got)
	}
	beforeButton := a.imagePreview.zoom
	if err := tt.Click("放大图片"); err != nil {
		t.Fatal(err)
	}
	if a.imagePreview.zoom <= beforeButton {
		t.Fatalf("zoom after + = %v, want > %v", a.imagePreview.zoom, beforeButton)
	}
	tt.Frame()
	tt.Press(450, 300)
	tt.Move(490, 330)
	tt.Frame()
	tt.Release(490, 330)
	if a.imagePreview.x == 0 || a.imagePreview.y == 0 {
		t.Fatalf("drag pan = (%v, %v), want nonzero movement", a.imagePreview.x, a.imagePreview.y)
	}
	tt.Press(450, 300)
	if !a.imagePreview.dragging {
		t.Fatal("pointer down did not begin a pan")
	}
	tt.CancelPress()
	if a.imagePreview.dragging {
		t.Fatal("canceled pointer up left pan active")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.imagePreview.open {
		t.Fatal("Esc left the image preview open")
	}
	tt.Frame()
	if !a.ed.Focused() {
		t.Fatal("closing the preview did not restore editor focus")
	}
}

func TestImagePreviewUsesSharedMissingImageMessage(t *testing.T) {
	dir := t.TempDir()
	a := New()
	a.path = filepath.Join(dir, "note.md")
	src := filepath.Join(dir, "附件", "missing.png")
	a.openImagePreview(src)
	tt := ui.NewTester(a.View, 900, 600)
	tt.Frame()
	want := mdoc.MissingImageMessage("附件/missing.png")
	if !tt.HasText(want) {
		t.Fatalf("missing-image preview = %q, want %q", tt.Texts(), want)
	}
	if a.imagePreview.bmp != nil || a.imagePreview.loading {
		t.Fatalf("missing image state = %+v, want a settled not-found state", a.imagePreview)
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
	a.root, a.path = root, filepath.Join(root, "Home.md")
	a.openImagePreview(src)
	tt := ui.NewTester(a.View, 900, 600)
	deadline := time.Now().Add(2 * time.Second)
	for a.imagePreview.bmp == nil && time.Now().Before(deadline) {
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
	tt.Frame()
	if a.imagePreview.asset == nil || a.imagePreview.bmp == nil {
		t.Fatalf("GIF full preview asset did not load: %+v", a.imagePreview)
	}
	if next, _ := a.imagePreview.asset.Advance(tt.Now()); next <= 0 {
		t.Fatal("GIF asset did not report an animation frame deadline")
	}
	first := append([]byte(nil), tt.Image().Pix...)
	tt.Advance(750 * time.Millisecond)
	second := tt.Image().Pix
	if bytes.Equal(first, second) {
		t.Fatal("full-screen GIF preview did not repaint with a later frame")
	}
}

func TestImagePreviewZoomClamp(t *testing.T) {
	if got := clampImageZoom(0); got != imagePreviewMinZoom {
		t.Errorf("lower clamp = %v, want %v", got, imagePreviewMinZoom)
	}
	if got := clampImageZoom(100); got != imagePreviewMaxZoom {
		t.Errorf("upper clamp = %v, want %v", got, imagePreviewMaxZoom)
	}
}

func TestImageInitialScaleFitsOnlyOversizedImages(t *testing.T) {
	if got := imageInitialScale(300, 200, 600, 400); got != 1 {
		t.Fatalf("small image initial scale = %v, want native 1", got)
	}
	if got := imageInitialScale(1200, 600, 600, 400); got != .5 {
		t.Fatalf("wide image initial scale = %v, want .5", got)
	}
	if got := imageInitialScale(400, 1200, 600, 600); got != .5 {
		t.Fatalf("tall image initial scale = %v, want .5", got)
	}
}

func TestImageDocumentCanvasZoomPanAndFullscreen(t *testing.T) {
	path := writePreviewPNG(t)
	a := New()
	a.openImageDocument(path, false)
	tt := ui.NewTester(a.View, 900, 600)
	tt.Frame()
	if !tt.HasText("100%") {
		t.Fatalf("native-size image page did not show 100%%: %q", tt.Texts())
	}
	writeImageUIScreenshot(t, tt, "image-document")
	before := a.imageDocument.zoom
	tt.Wheel(450, 300, 0, -40)
	tt.Frame()
	if a.imageDocument.zoom <= before {
		t.Fatalf("image-document wheel zoom = %v, want > %v", a.imageDocument.zoom, before)
	}
	beforePanZoom := a.imageDocument.zoom
	tt.Scroll(450, 300, 20, -25)
	tt.Frame()
	if a.imageDocument.zoom != beforePanZoom || a.imageDocument.x == 0 || a.imageDocument.y == 0 {
		t.Fatalf("image-document touchpad scroll = zoom %v, pan (%v,%v); want pan without zoom", a.imageDocument.zoom, a.imageDocument.x, a.imageDocument.y)
	}
	beforePinch := a.imageDocument.zoom
	tt.Pinch(450, 300, 1.5)
	tt.Frame()
	if a.imageDocument.zoom <= beforePinch {
		t.Fatalf("image-document pinch zoom = %v, want > %v", a.imageDocument.zoom, beforePinch)
	}
	tt.Press(450, 300)
	tt.Move(480, 320)
	tt.Frame()
	tt.Release(480, 320)
	if a.imageDocument.x == 0 || a.imageDocument.y == 0 {
		t.Fatalf("image-document pan = (%v, %v), want nonzero movement", a.imageDocument.x, a.imageDocument.y)
	}
	if err := tt.Click(locales.ImagePreviewTitle()); err != nil {
		t.Fatalf("click fullscreen preview: %v", err)
	}
	tt.Frame()
	if !a.imagePreview.open {
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
	if !a.vault.linksOpen {
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
