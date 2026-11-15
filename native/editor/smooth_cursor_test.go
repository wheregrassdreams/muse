package editor

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSmoothCursorOnlyTracksModalNormalNavigation(t *testing.T) {
	for _, keymap := range []string{"vim", "helix", "default"} {
		e := &Editor{opts: Options{SmoothCursor: true}, modal: modalState{keymap: keymap, mode: "Normal"}, caret: 4, anchor: 4, smoothCursorNavigation: true}
		e.beginSmoothCursorMove(4, 8, false)
		want := keymap == "vim" || keymap == "helix"
		if e.smoothCursorActive != want {
			t.Errorf("keymap %q active=%v, want %v", keymap, e.smoothCursorActive, want)
		}
	}
	e := &Editor{opts: Options{SmoothCursor: true}, modal: modalState{keymap: "vim", mode: "Insert"}, caret: 4, anchor: 4}
	e.beginSmoothCursorMove(4, 8, false)
	if e.smoothCursorActive {
		t.Fatal("insert-mode movement should not animate")
	}
}

func TestSmoothCursorExcludesDirectCaretChangesAndTextEdits(t *testing.T) {
	e := New("alpha beta", Options{SmoothCursor: true})
	e.SetKeymap("vim")
	e.setCaret(0, false)
	if e.smoothCursorActive {
		t.Fatal("direct caret changes such as pointer placement must not animate")
	}
	e.modalText("l")
	if !e.smoothCursorActive {
		t.Fatal("modal navigation should animate")
	}
	e.smoothCursorActive = false
	e.modalText("x")
	if e.smoothCursorActive {
		t.Fatal("text edits must not animate caret relocation")
	}
}

func TestSmoothCursorAnimationClearsWhenDisabledOrKeymapLeavesModal(t *testing.T) {
	e := New("alpha", Options{SmoothCursor: true})
	e.SetKeymap("vim")
	e.smoothCursorActive = true
	e.SetOptions(Options{})
	if e.smoothCursorActive {
		t.Fatal("disabling smooth cursor should discard an in-flight animation")
	}
	e.SetOptions(Options{SmoothCursor: true})
	e.smoothCursorActive = true
	e.SetKeymap("default")
	if e.smoothCursorActive {
		t.Fatal("leaving modal keymaps should discard an in-flight animation")
	}
}

func TestSmoothCursorOverlayPassesPointerInputToEditor(t *testing.T) {
	h := newHarness(t, "click here")
	opts := h.e.Options()
	opts.SmoothCursor = true
	h.e.SetOptions(opts)
	h.e.SetKeymap("vim")
	h.frame()
	h.e.setCaret(0, false)
	h.tt.ClickAt(120, 160)
	h.frame()
	if h.e.caret == 0 {
		t.Fatal("caret overlay intercepted pointer placement")
	}
}

func TestSmoothCursorRectInterpolation(t *testing.T) {
	e := &Editor{opts: Options{SmoothCursor: true}, smoothCursorActive: true, smoothCursorStart: time.Unix(1, 0)}
	from := ui.Rect{X: 0, Y: 0, W: 2, H: 10}
	to := ui.Rect{X: 20, Y: 10, W: 4, H: 20}
	got, active := e.smoothCursorRects(time.Unix(1, 0).Add(smoothCursorDuration/2), from, to)
	if !active || got.X != 10 || got.Y != 5 || got.W != 3 || got.H != 15 {
		t.Fatalf("midpoint rect=%+v active=%v", got, active)
	}
	got, active = e.smoothCursorRects(time.Unix(1, 0).Add(smoothCursorDuration), from, to)
	if active || got != to {
		t.Fatalf("finished rect=%+v active=%v", got, active)
	}
}

func TestSmoothCursorRequestsAnimationFramesOnlyWhileActive(t *testing.T) {
	for _, tc := range []struct {
		enabled, active, want bool
	}{{false, true, false}, {true, false, false}, {true, true, true}} {
		if got := smoothCursorNeedsFrames(tc.enabled, tc.active); got != tc.want {
			t.Errorf("smoothCursorNeedsFrames(%v,%v)=%v want %v", tc.enabled, tc.active, got, tc.want)
		}
	}
}

func TestSmoothBlockCaretInterpolatesUntransformedGeometry(t *testing.T) {
	e := &Editor{opts: Options{SmoothCursor: true}, smoothCursorActive: true, smoothCursorStart: time.Unix(1, 0)}
	from := ui.Rect{X: 2, Y: 4, W: 5, H: 12}
	to := ui.Rect{X: 12, Y: 14, W: 9, H: 20}
	got, active := e.smoothCursorRects(time.Unix(1, 0).Add(smoothCursorDuration/2), from, to)
	if !active || got.X != 7 || got.Y != 9 || got.W != 7 || got.H != 16 {
		t.Fatalf("block caret interpolation should preserve raw caret geometry: %+v active=%v", got, active)
	}
}

func TestSmoothCursorNavigationRebasesFromCurrentVisualPosition(t *testing.T) {
	start := time.Unix(10, 0)
	e := &Editor{
		opts:   Options{SmoothCursor: true},
		modal:  modalState{keymap: "vim", mode: "Normal"},
		anchor: 5, smoothCursorActive: true, smoothCursorNavigation: true,
		smoothCursorStart:      start,
		smoothCursorFromRect:   ui.Rect{X: 0, Y: 0, W: 2, H: 10},
		smoothCursorToRect:     ui.Rect{X: 20, Y: 10, W: 4, H: 20},
		smoothCursorRectsValid: true,
	}
	now := start.Add(smoothCursorDuration / 2)
	e.beginSmoothCursorMoveAt(5, 6, false, now)
	want := ui.Rect{X: 10, Y: 5, W: 3, H: 15}
	if !e.smoothCursorFromSet || e.smoothCursorFromRect != want || e.smoothCursorStart != now {
		t.Fatalf("rebased animation from=%+v set=%v start=%v", e.smoothCursorFromRect, e.smoothCursorFromSet, e.smoothCursorStart)
	}
}
