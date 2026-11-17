package render

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRendererBuildCarriesBorrowedVisibleSnapshot(t *testing.T) {
	blocks := []Block{{Kind: TextBlock, Bounds: ui.Rect{X: 4, Y: 8, W: 80, H: 20}}}
	behind := []Overlay{{Kind: FillOverlay, Bounds: ui.Rect{W: 3, H: 5}}}
	front := []Overlay{{Kind: StrokeOverlay, Bounds: ui.Rect{X: 1, Y: 1, W: 2, H: 2}}}
	snapshot := Snapshot{
		Revision: 17, Viewport: ui.Rect{X: 2, Y: 3, W: 100, H: 60}, Style: Style{Font: "sans", FontSize: 14},
		Behind: behind, Blocks: blocks, InFront: front,
	}

	frame := (Renderer{}).Build(snapshot, Resources{})
	if frame.Revision != snapshot.Revision || frame.Viewport != snapshot.Viewport || frame.Style != snapshot.Style {
		t.Fatalf("frame metadata = %+v, want snapshot metadata", frame)
	}
	if len(frame.Behind) != 1 || len(frame.Blocks) != 1 || len(frame.InFront) != 1 {
		t.Fatalf("frame layers = %d/%d/%d", len(frame.Behind), len(frame.Blocks), len(frame.InFront))
	}
	if frame.Blocks[0].Kind != blocks[0].Kind || frame.Blocks[0].Bounds != blocks[0].Bounds ||
		frame.Behind[0].Bounds != behind[0].Bounds || frame.InFront[0].Bounds != front[0].Bounds {
		t.Fatalf("frame did not preserve the borrowed view values: %+v", frame)
	}
}
