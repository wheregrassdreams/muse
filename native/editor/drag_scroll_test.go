package editor

import (
	"math"
	"strings"
	"testing"
	"time"
)

func dragScrollHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, strings.Repeat("普通正文，用于检查滚动中按下鼠标以及正常拖选。\n\n", 120))
	h.e.scroll.Y = 1000
	h.frame()
	return h
}

func TestStationaryPressDuringScrollDoesNotStartEdgeAutoscroll(t *testing.T) {
	for _, down := range []bool{true, false} {
		name, y, delta := "down", float32(120), float32(240)
		if !down {
			name, y, delta = "up", 580, -240
		}
		t.Run(name, func(t *testing.T) {
			h := dragScrollHarness(t)
			x := h.e.colX + 100
			h.tt.Move(x, y)
			h.frame()
			h.tt.Scroll(x, y, 0, delta)
			h.frame()
			before := h.e.scroll.Y
			h.tt.Press(x, y) // no intervening mouse movement
			h.frame()
			if got := h.e.scroll.Y; math.Abs(float64(got-before)) > .01 {
				t.Fatalf("stationary press started phantom autoscroll: before=%v after=%v pointerY=%v", before, got, h.e.pointerY)
			}
			caret := h.e.caret
			// The touchpad's residual deltas keep arriving under a stationary
			// held mouse. Only those deltas may move the document.
			for _, tail := range []float32{160, 80, 40, 20, 10} {
				if !down {
					tail = -tail
				}
				want := h.e.scroll.Y + tail
				h.tt.Scroll(x, y, 0, tail)
				h.frame()
				if got := h.e.scroll.Y; math.Abs(float64(got-want)) > .01 {
					t.Fatalf("residual scroll fed edge autoscroll: got=%v want=%v pointerY=%v", got, want, h.e.pointerY)
				}
			}
			settled := h.e.scroll.Y
			for range 8 {
				h.tt.Advance(32 * time.Millisecond)
				h.frame()
			}
			if h.e.scroll.Y != settled || h.e.caret != caret || h.e.anchor != caret {
				t.Fatalf("holding after scroll settled moved page or selection: scroll=%v want=%v anchor=%d caret=%d wantCaret=%d", h.e.scroll.Y, settled, h.e.anchor, h.e.caret, caret)
			}
			h.tt.Release(x, y)
			h.frame()
		})
	}
}

func TestRealSelectionDragStillAutoscrollsOutsideViewport(t *testing.T) {
	for _, down := range []bool{true, false} {
		name, edge := "bottom", float32(730)
		if !down {
			name, edge = "top", -30
		}
		t.Run(name, func(t *testing.T) {
			h := dragScrollHarness(t)
			x := h.e.colX + 100
			h.tt.Press(x, 350)
			h.frame()
			anchor, before := h.e.anchor, h.e.scroll.Y
			h.tt.Move(x, edge)
			h.frame()
			for range 4 {
				h.tt.Advance(16 * time.Millisecond)
				h.frame()
			}
			if (down && h.e.scroll.Y <= before) || (!down && h.e.scroll.Y >= before) || h.e.anchor != anchor || h.e.caret == anchor {
				t.Fatalf("real edge drag no longer scrolls and extends selection: scroll=%v before=%v anchor=%d caret=%d", h.e.scroll.Y, before, h.e.anchor, h.e.caret)
			}
			h.tt.Release(x, edge)
			h.frame()
			settled := h.e.scroll.Y
			h.tt.Advance(80 * time.Millisecond)
			h.frame()
			if h.e.dragging || h.e.scroll.Y != settled {
				t.Fatal("releasing a real drag left autoscroll running")
			}
		})
	}
}

func TestPressDuringWheelGlideDoesNotFeedSelectionAutoscroll(t *testing.T) {
	for _, down := range []bool{true, false} {
		name, y, delta := "down", float32(120), float32(240)
		if !down {
			name, y, delta = "up", 580, -240
		}
		t.Run(name, func(t *testing.T) {
			h := dragScrollHarness(t)
			x, start := h.e.colX+100, h.e.scroll.Y
			h.tt.Move(x, y)
			h.tt.Wheel(x, y, 0, delta)
			h.tt.Advance(30 * time.Millisecond)
			h.frame()
			travel := (h.e.scroll.Y - start) / delta
			if travel <= 0 || travel >= 1 {
				t.Fatalf("fixture must press while wheel glide is unfinished: fraction=%v", travel)
			}
			h.tt.Press(x, y)
			h.frame()
			caret := h.e.caret
			for range 30 {
				h.tt.Advance(16 * time.Millisecond)
				h.frame()
				travel = (h.e.scroll.Y - start) / delta
				if travel < 0 || travel > 1.01 || h.e.caret != caret || h.e.anchor != caret {
					t.Fatalf("held mouse fed wheel glide into selection autoscroll: fraction=%v anchor=%d caret=%d wantCaret=%d", travel, h.e.anchor, h.e.caret, caret)
				}
			}
			h.tt.Release(x, y)
			h.frame()
		})
	}
}
