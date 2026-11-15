package mdedit

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

const smoothCursorDuration = 110 * time.Millisecond
const smoothCursorFrameRate = float32(60)

func smoothCursorNeedsFrames(enabled, active bool) bool { return enabled && active }

// beginSmoothCursorMove records only modal normal-mode navigation. The real
// caret and selection still move synchronously; paintCaret may interpolate
// between these source positions as a presentation-only effect.
func (e *Editor) beginSmoothCursorMove(from, to int, extend bool) {
	e.beginSmoothCursorMoveAt(from, to, extend, time.Now())
}

func (e *Editor) beginSmoothCursorMoveAt(from, to int, extend bool, now time.Time) {
	if !e.opts.SmoothCursor || !e.smoothCursorNavigation || extend || from == to || e.anchor != from {
		e.smoothCursorActive = false
		return
	}
	keymap := e.Keymap()
	if (keymap != "vim" && keymap != "helix") || e.modal.mode != "Normal" {
		e.smoothCursorActive = false
		return
	}
	e.smoothCursorFrom = from
	if e.smoothCursorActive && e.smoothCursorRectsValid {
		current, _ := interpolateSmoothCursor(e.smoothCursorStart, now, e.smoothCursorFromRect, e.smoothCursorToRect)
		e.smoothCursorFromRect = current
		e.smoothCursorFromSet = true
	} else {
		e.smoothCursorFromSet = false
		e.smoothCursorRectsValid = false
	}
	e.smoothCursorStart = now
	e.smoothCursorActive = true
}

// smoothCursorRects returns the visual caret rectangle interpolated from the
// previous source position to the current one. Callers should request
// refresh-aligned animation frames while active. It becomes inactive at
// completion.
func (e *Editor) smoothCursorRects(now time.Time, from, to ui.Rect) (ui.Rect, bool) {
	if !e.opts.SmoothCursor || !e.smoothCursorActive {
		return to, false
	}
	r, active := interpolateSmoothCursor(e.smoothCursorStart, now, from, to)
	if !active {
		e.smoothCursorActive = false
		return to, false
	}
	return r, true
}

func interpolateSmoothCursor(start, now time.Time, from, to ui.Rect) (ui.Rect, bool) {
	progress := float64(now.Sub(start)) / float64(smoothCursorDuration)
	if progress >= 1 {
		return to, false
	}
	if progress < 0 {
		progress = 0
	}
	// Smoothstep avoids abrupt starts/stops while keeping the motion short.
	t := float32(progress * progress * (3 - 2*progress))
	return ui.Rect{
		X: lerp(from.X, to.X, t), Y: lerp(from.Y, to.Y, t),
		W: lerp(from.W, to.W, t), H: lerp(from.H, to.H, t),
	}, true
}

func lerp(a, b, t float32) float32 {
	return a + (b-a)*t
}

// smoothCursorProgress is kept as a small pure helper for edge-case tests.
func smoothCursorProgress(elapsed time.Duration) float32 {
	p := float64(elapsed) / float64(smoothCursorDuration)
	p = math.Max(0, math.Min(1, p))
	return float32(p * p * (3 - 2*p))
}
