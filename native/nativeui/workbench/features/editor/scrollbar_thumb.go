package editor

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/ui/scrollbar"
)

const scrollbarThumbSettle = 180 * time.Millisecond

type scrollbarThumbState struct {
	initialized bool
	lastOffset  float32
	length      float32
	heldLength  float32
	holdUntil   time.Time
	animFrom    float32
	animStarted time.Time
	dragging    bool
	grab        float32
}

// update keeps the thumb's length steady while the document is moving and
// eases it to the latest measured extent after scrolling settles.
func (s *scrollbarThumbState) update(now time.Time, offset, view, content float32, visible bool) (show, animate bool) {
	natural := scrollbarLength(view, content)
	if !s.initialized {
		s.initialized = true
		s.lastOffset = offset
		s.length = natural
	}
	if offset != s.lastOffset {
		if !now.Before(s.holdUntil) {
			s.heldLength = s.length
		}
		s.holdUntil = now.Add(scrollbar.VisibleFor)
		s.animStarted = time.Time{}
	}
	s.lastOffset = offset
	if s.dragging {
		return true, false
	}
	if now.Before(s.holdUntil) {
		if s.heldLength > 0 {
			s.length = s.heldLength
		}
		return visible, false
	}
	if s.heldLength > 0 && math.Abs(float64(natural-s.heldLength)) > 0.5 {
		if s.animStarted.IsZero() {
			s.animFrom = s.length
			s.animStarted = now
		}
		progress := min(float32(now.Sub(s.animStarted))/float32(scrollbarThumbSettle), 1)
		// Ease out cubic to make the small size correction feel settled.
		eased := 1 - float32(math.Pow(float64(1-progress), 3))
		s.length = s.animFrom + (natural-s.animFrom)*eased
		if progress < 1 {
			return true, true
		}
		s.length = natural
		s.heldLength = 0
		s.animStarted = time.Time{}
	} else {
		s.length = natural
		s.heldLength = 0
		s.animStarted = time.Time{}
	}
	return visible, false
}

func scrollbarLength(view, content float32) float32 {
	if view <= 0 || content <= view {
		return 0
	}
	bar := ui.Scrollbar{Box: ui.Rect{H: view}, View: view, Content: content}
	_, length := bar.Thumb()
	return length
}

// visualScrollbar keeps the thumb length independent of the true document
// extent while preserving its position as a fraction of the real scroll range.
func (e *Editor) visualScrollbar(box ui.Rect) ui.Scrollbar {
	view := box.H
	if view <= 0 || e.scroll.MaxY <= 0 {
		return ui.Scrollbar{Box: box, View: view, Content: view}
	}
	length := max(e.scrollbarThumb.length, float32(24))
	inner := max(view-4, 1)
	length = min(length, inner)
	content := max(view+1, inner*view/length)
	fraction := min(max(e.scroll.Y/e.scroll.MaxY, 0), 1)
	return ui.Scrollbar{
		Box: box, View: view, Content: content,
		Offset: fraction * (content - view),
	}
}

func (e *Editor) scrollbarPointerDown(ev ui.InputEvent) bool {
	if ev.Button != 0 || !e.scrollbarShown || e.viewH <= 0 || e.scroll.MaxY <= 0 || ev.X < e.elemW-18 || ev.X > e.elemW {
		return false
	}
	y := ev.Y - e.scroll.Y
	if y < 0 || y > e.viewH {
		return false
	}
	box := ui.Rect{X: e.elemRect.X, Y: e.scroll.Y + e.elemRect.Y, W: e.elemW, H: e.viewH}
	bar := e.visualScrollbar(box)
	start, length := bar.Thumb()
	start -= box.Y
	s := &e.scrollbarThumb
	s.heldLength = length
	if y >= start && y <= start+length {
		s.dragging, s.grab = true, y-start
	} else {
		travel := max(e.viewH-4-length, 0)
		if travel > 0 {
			fraction := min(max((y-length/2-2)/travel, 0), 1)
			e.scroll.Y = fraction * e.scroll.MaxY
		}
		s.dragging, s.grab = true, length/2
	}
	e.invalidate()
	return true
}

func (e *Editor) dragScrollbarThumb(ev ui.InputEvent) bool {
	s := &e.scrollbarThumb
	if !s.dragging {
		return false
	}
	view := e.viewH
	length := min(max(s.length, float32(24)), max(view-4, 1))
	travel := max(view-4-length, 0)
	if travel > 0 && e.scroll.MaxY > 0 {
		y := ev.Y - e.scroll.Y
		fraction := min(max((y-s.grab-2)/travel, 0), 1)
		e.scroll.Y = fraction * e.scroll.MaxY
	}
	e.invalidate()
	return true
}
