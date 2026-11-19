package render

import (
	"math"
	"testing"
)

func TestProjectCalloutsGroupsLinesAndRevealsEditedTitle(t *testing.T) {
	blocks := ProjectCallouts([]CalloutLine{
		{ID: 4, X: 12, Y: 30, Height: 20, Right: 212, FontSize: 16, Title: "Warning", Alert: AlertWarning},
		{ID: 4, X: 12, Y: 50, Height: 24, Right: 212, FontSize: 16, Title: "Warning", Alert: AlertWarning, HeaderSourceRevealed: true},
		{ID: 9, X: 0, Y: 500, Height: 20, Right: 212, FontSize: 16, Title: "Offscreen"},
	}, 0, 100)
	if len(blocks) != 1 {
		t.Fatalf("projected blocks = %+v", blocks)
	}
	block := blocks[0]
	if block.Kind != CalloutBlock || block.Alert != AlertWarning || block.Title != "" {
		t.Errorf("callout metadata = %+v", block)
	}
	if block.Bounds.X != 12 || math.Abs(float64(block.Bounds.Y-22.8)) > .001 || block.Bounds.W != 200 || math.Abs(float64(block.Bounds.H-60.8)) > .001 {
		t.Errorf("callout bounds = %+v", block.Bounds)
	}
}
