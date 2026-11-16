// Package layout defines backend-neutral geometry snapshots for the editor.
package layout

import (
	"muse/editor/internal/core"
	"muse/markdown/render"
	"muse/markdown/syntax"
)

// Rect is a logical-pixel rectangle independent of any rendering toolkit.
type Rect struct{ X, Y, W, H float32 }

// BlockFrame places one block from the neutral Markdown render projection.
// DocumentIndex identifies its immutable model in Frame.Document.Blocks.
type BlockFrame struct {
	DocumentIndex int
	SourceStart   int
	SourceEnd     int
	Kind          syntax.Kind
	Bounds        Rect
	Estimated     bool
}

// RenderFrame is an immutable snapshot used to paint the visible editor
// viewport. Blocks contains only the visible range plus overscan; Document
// holds the revision-scoped neutral model from which their content is read.
type RenderFrame struct {
	Revision      uint64
	Source        string
	Selection     core.Selection
	Caret         int
	Viewport      Rect
	ScrollY       float32
	ContentHeight float32
	VisibleStart  int
	VisibleEnd    int
	Blocks        []BlockFrame
	Document      *render.Document
}

// BlockAt returns the visible block containing a viewport-relative point.
// The x coordinate is reserved for block-specific hit testing in the adapter.
func (f RenderFrame) BlockAt(x, y float32) (BlockFrame, bool) {
	_ = x
	for _, block := range f.Blocks {
		r := block.Bounds
		if x >= r.X && x <= r.X+r.W && y >= r.Y && y <= r.Y+r.H {
			return block, true
		}
	}
	return BlockFrame{}, false
}
