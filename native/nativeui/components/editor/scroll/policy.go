// Package scroll owns value-based viewport and indicator geometry used by
// editor block scroll adapters. It does not know about editor leaves or source
// positions; the host supplies measured content and viewport bounds.
package scroll

import (
	"time"

	"github.com/egoist/mygo/ui"
	"muse/ui/scrollbar"
)

// ClampOffset keeps a horizontal content offset inside its measured viewport.
func ClampOffset(contentWidth, viewportWidth, offset float32) float32 {
	return min(max(offset, 0), max(contentWidth-viewportWidth, 0))
}

// RevealOffset adjusts offset only when the caret leaves the inset viewport.
func RevealOffset(offset, viewportStart, viewportWidth, caretStart, caretEnd, inset float32) float32 {
	if caretStart < viewportStart+inset {
		return offset + caretStart - viewportStart - inset
	}
	if caretEnd > viewportStart+viewportWidth-inset {
		return offset + caretEnd - (viewportStart + viewportWidth) + inset
	}
	return offset
}

// VirtualRows reports how many text-like rows fit a standalone tall visual.
// The editor host uses these rows for navigation while retaining real source
// offsets for text operations.
func VirtualRows(height, lineHeight float32) int {
	if height <= 0 || lineHeight <= 0 {
		return 0
	}
	rows := int((height + lineHeight - 0.0001) / lineHeight)
	return max(rows, 1)
}

// HorizontalBarRects returns track and thumb geometry for a clipped block.
func HorizontalBarRects(box ui.Rect, contentWidth, offset float32) (track, thumb ui.Rect) {
	track = ui.Rect{X: box.X + 2, Y: box.Y + box.H - 7, W: max(box.W-4, 1), H: 5}
	if contentWidth <= 0 || box.W <= 0 {
		return track, ui.Rect{}
	}
	frac := box.W / contentWidth
	thumbWidth := min(max(track.W*frac, 28), track.W)
	progress := float32(0)
	if maxOffset := contentWidth - box.W; maxOffset > 0 {
		progress = min(max(offset/maxOffset, 0), 1)
	}
	thumb = ui.Rect{X: track.X + (track.W-thumbWidth)*progress, Y: track.Y, W: thumbWidth, H: track.H}
	return track, thumb
}

// HorizontalFrame is the immutable geometry snapshot used for scrollbar input.
// The editor host owns the block and applies the returned offset.
type HorizontalFrame struct {
	Viewport     ui.Rect
	ContentWidth float32
	Offset       float32
}

// HorizontalState is the per-block scroll position and transient indicator
// state. It contains no editor leaf or document reference.
type HorizontalState struct {
	Offset    float32
	Shown     bool
	Indicator *IndicatorState
}

// HorizontalDrag stores only gesture coordinates; it has no block or editor
// reference and can be discarded when the host ends the pointer gesture.
type HorizontalDrag struct {
	PointerX float32
	Offset   float32
}

// ScrollOffset computes a clamped horizontal wheel result for one block.
func ScrollOffset(frame HorizontalFrame, delta float32) float32 {
	return ClampOffset(frame.ContentWidth, frame.Viewport.W, frame.Offset+delta)
}

// BeginHorizontalDrag maps a pointer-down to a drag snapshot and starting
// offset. Clicking outside the thumb centers it at the pointer.
func BeginHorizontalDrag(frame HorizontalFrame, pointerX float32) (HorizontalDrag, float32) {
	track, thumb := HorizontalBarRects(frame.Viewport, frame.ContentWidth, frame.Offset)
	offset := ClampOffset(frame.ContentWidth, frame.Viewport.W, frame.Offset)
	if pointerX < thumb.X || pointerX > thumb.X+thumb.W {
		offset = ClampOffset(frame.ContentWidth, frame.Viewport.W,
			(pointerX-track.X-thumb.W/2)/max(track.W-thumb.W, 1)*(frame.ContentWidth-frame.Viewport.W))
	}
	return HorizontalDrag{PointerX: pointerX, Offset: offset}, offset
}

// DragHorizontal computes the offset while dragging a thumb from its initial
// pointer and offset, using only the current block viewport geometry.
func DragHorizontal(frame HorizontalFrame, drag HorizontalDrag, pointerX float32) float32 {
	track, thumb := HorizontalBarRects(frame.Viewport, frame.ContentWidth, frame.Offset)
	return ClampOffset(frame.ContentWidth, frame.Viewport.W,
		drag.Offset+(pointerX-drag.PointerX)/max(track.W-thumb.W, 1)*(frame.ContentWidth-frame.Viewport.W))
}

// IndicatorState embeds the toolkit indicator behind a package-owned API so
// host adapters do not depend on the scrollbar implementation details.
type IndicatorState struct{ indicator scrollbar.Indicator }

func (s *IndicatorState) Visible(now time.Time, x, y float32) bool {
	return s != nil && s.indicator.Visible(now, x, y)
}

func (s *IndicatorState) Remaining(now time.Time) time.Duration {
	if s == nil {
		return 0
	}
	return s.indicator.Remaining(now)
}
