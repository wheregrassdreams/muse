package filetree

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPlaceImagePreviewAnchorsToHoveredRow(t *testing.T) {
	viewport := ui.Rect{W: 1000, H: 720}
	anchor := ui.Rect{X: 16, Y: 180, W: 416, H: 40}
	got := PlaceImagePreview(viewport, anchor, 280, 220)
	if got.X != anchor.X+anchor.W+8 || got.Y != anchor.Y || got.W != 280 || got.H != 220 {
		t.Fatalf("preview rect = %+v, want adjacent and top-aligned to row %+v", got, anchor)
	}
}

func TestPlaceImagePreviewFlipsAndClampsAtViewportEdges(t *testing.T) {
	viewport := ui.Rect{W: 500, H: 360}
	anchor := ui.Rect{X: 360, Y: 320, W: 120, H: 40}
	got := PlaceImagePreview(viewport, anchor, 280, 220)
	if got.X != anchor.X-280-8 || got.Y+got.H > viewport.H-8 {
		t.Fatalf("preview did not flip left and clamp vertically: %+v", got)
	}

	viewport = ui.Rect{W: 240, H: 180}
	anchor = ui.Rect{X: 100, Y: -20, W: 120, H: 40}
	got = PlaceImagePreview(viewport, anchor, 280, 220)
	if got.X < 8 || got.Y < 8 || got.X+got.W > viewport.W-8 || got.Y+got.H > viewport.H-8 {
		t.Fatalf("preview escaped viewport after clamping: %+v", got)
	}
}
