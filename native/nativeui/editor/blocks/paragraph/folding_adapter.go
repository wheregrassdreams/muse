package paragraphui

import "github.com/egoist/mygo/ui"

// HeadingLine is a detached text-line rectangle in editor coordinates.
type HeadingLine struct {
	Rect  ui.Rect
	First bool
}

// FoldHeadingView contains only the presentation data needed to hit-test and
// paint one fold affordance. Source identity is an opaque host-owned offset.
type FoldHeadingView struct {
	SourceStart int
	Foldable    bool
	Folded      bool
	Chevron     ui.Rect
	Lines       []HeadingLine
}

// FoldFrame is a detached presentation snapshot. SourceStart is an opaque
// identity that the host resolves back to its private heading state.
type FoldFrame struct {
	Headings     []FoldHeadingView
	HoveredStart int
}

type FoldActionKind uint8

const (
	FoldHeadingAction FoldActionKind = iota + 1
	ToggleFoldAction
)

// FoldAction is the only interaction result returned to the host.
type FoldAction struct {
	Kind        FoldActionKind
	SourceStart int
}

// HeadingAt returns the source identity of the topmost heading title or
// chevron corridor under the pointer.
func HeadingAt(frame FoldFrame, x, y float32) (FoldAction, bool) {
	start, ok := headingAt(frame.Headings, x, y)
	if !ok {
		return FoldAction{}, false
	}
	return FoldAction{Kind: FoldHeadingAction, SourceStart: start}, true
}

func headingAt(views []FoldHeadingView, x, y float32) (int, bool) {
	for i := len(views) - 1; i >= 0; i-- {
		view := views[i]
		if !view.Foldable {
			continue
		}
		for _, line := range view.Lines {
			if HeadingLineHit(line.Rect, line.First, view.Chevron, x, y) {
				return view.SourceStart, true
			}
		}
		if ChevronCorridorHit(view.Chevron, x, y) {
			return view.SourceStart, true
		}
	}
	return 0, false
}

// ChevronAt returns a heading only when its expanded control target is under
// the pointer and that same heading owns the hover state.
func ChevronAt(frame FoldFrame, x, y float32) (FoldAction, bool) {
	start, ok := headingAt(frame.Headings, x, y)
	if !ok || start != frame.HoveredStart {
		return FoldAction{}, false
	}
	for _, view := range frame.Headings {
		if view.SourceStart == start && view.Foldable && ChevronHit(view.Chevron, x, y) {
			return FoldAction{Kind: ToggleFoldAction, SourceStart: start}, true
		}
	}
	return FoldAction{}, false
}

// PaintChevron paints the hovered heading's affordance if the pointer remains
// on that heading. origin shifts document geometry into the painter's space.
func PaintFoldChevron(p *ui.Painter, frame FoldFrame, sourceStart int, pointerX, pointerY, originX, originY float32, stroke ui.Color, strokeWidth float32) bool {
	if frame.HoveredStart != sourceStart {
		return false
	}
	if at, ok := headingAt(frame.Headings, pointerX, pointerY); !ok || at != sourceStart {
		return false
	}
	for _, view := range frame.Headings {
		if view.SourceStart != sourceStart || !view.Foldable {
			continue
		}
		bounds := ui.Rect{
			X: view.Chevron.X + originX,
			Y: view.Chevron.Y + originY,
			W: view.Chevron.W,
			H: view.Chevron.H,
		}
		PaintChevron(p, bounds, view.Folded, stroke, strokeWidth)
		return true
	}
	return false
}
