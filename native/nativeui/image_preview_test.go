package nativeui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
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
	tt.Frame()
	if !a.imagePreview.open || a.imagePreview.bmp == nil {
		t.Fatalf("image preview did not load %q: %+v", path, a.imagePreview)
	}
	if !tt.HasText("preview.png") || !tt.HasText("100%") {
		t.Fatalf("preview overlay text = %q", tt.Texts())
	}
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

func TestImagePreviewZoomClamp(t *testing.T) {
	if got := clampImageZoom(0); got != imagePreviewMinZoom {
		t.Errorf("lower clamp = %v, want %v", got, imagePreviewMinZoom)
	}
	if got := clampImageZoom(100); got != imagePreviewMaxZoom {
		t.Errorf("upper clamp = %v, want %v", got, imagePreviewMaxZoom)
	}
}
