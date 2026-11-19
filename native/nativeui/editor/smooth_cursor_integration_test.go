package editor

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	editormodel "github.com/wheregrassdreams/muse-editor/text/session"
	helix "github.com/wheregrassdreams/muse-editor/text/helix"
	"muse/nativeui/editor/feedback"
)

func TestSmoothCursorOnlyTracksModalNormalNavigation(t *testing.T) {
	for _, keymap := range []string{"vim", "helix", "default"} {
		session := editormodel.NewSession("0123456789")
		_ = session.SetSelection(editormodel.Selection{Anchor: 4, Head: 4})
		controller := helix.New()
		controller.SetKeymap(keymap)
		e := &Editor{Session: session, opts: Options{SmoothCursor: true}, vimState: controller, smoothCursorNavigation: true}
		e.beginSmoothCursorMove(4, 8, false)
		want := keymap == "vim" || keymap == "helix"
		if e.feedbackState.CursorState().Active != want {
			t.Errorf("keymap %q active=%v, want %v", keymap, e.feedbackState.CursorState().Active, want)
		}
	}
	session := editormodel.NewSession("0123456789")
	_ = session.SetSelection(editormodel.Selection{Anchor: 4, Head: 4})
	controller := helix.New()
	controller.SetKeymap("vim")
	controller.SetMode(helix.ModeInsert)
	e := &Editor{Session: session, opts: Options{SmoothCursor: true}, vimState: controller}
	e.beginSmoothCursorMove(4, 8, false)
	if e.feedbackState.CursorState().Active {
		t.Fatal("insert-mode movement should not animate")
	}
}

func TestSmoothCursorExcludesDirectCaretChangesAndTextEdits(t *testing.T) {
	e := New("alpha beta", Options{SmoothCursor: true})
	e.SetKeymap("vim")
	e.setCaret(0, false)
	if e.feedbackState.CursorState().Active {
		t.Fatal("direct caret changes such as pointer placement must not animate")
	}
	e.modalText("l")
	if !e.feedbackState.CursorState().Active {
		t.Fatal("modal navigation should animate")
	}
	e.feedbackState.CancelCursor()
	e.modalText("x")
	if e.feedbackState.CursorState().Active {
		t.Fatal("text edits must not animate caret relocation")
	}
}

func TestSmoothCursorAnimationClearsWhenDisabledOrKeymapLeavesModal(t *testing.T) {
	e := New("alpha", Options{SmoothCursor: true})
	e.SetKeymap("vim")
	e.feedbackState.BeginCursor(1, 2, time.Now())
	e.SetOptions(Options{})
	if e.feedbackState.CursorState().Active {
		t.Fatal("disabling smooth cursor should discard an in-flight animation")
	}
	e.SetOptions(Options{SmoothCursor: true})
	e.feedbackState.BeginCursor(1, 2, time.Now())
	e.SetKeymap("default")
	if e.feedbackState.CursorState().Active {
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
	if h.e.Caret() == 0 {
		t.Fatal("caret overlay intercepted pointer placement")
	}
}

func TestSmoothCursorRectInterpolation(t *testing.T) {
	state := feedback.Handle{}
	from := ui.Rect{X: 0, Y: 0, W: 2, H: 10}
	to := ui.Rect{X: 20, Y: 10, W: 4, H: 20}
	state.BeginCursor(0, 1, time.Unix(1, 0))
	state.SetCursorRects(feedback.Rect{X: from.X, Y: from.Y, W: from.W, H: from.H}, feedback.Rect{X: to.X, Y: to.Y, W: to.W, H: to.H})
	got, active := state.CursorRects(time.Unix(1, 0).Add(feedback.SmoothCursorDuration / 2))
	gotUI := ui.Rect{X: got.X, Y: got.Y, W: got.W, H: got.H}
	if !active || gotUI.X != 10 || gotUI.Y != 5 || gotUI.W != 3 || gotUI.H != 15 {
		t.Fatalf("midpoint rect=%+v active=%v", gotUI, active)
	}
	got, active = state.CursorRects(time.Unix(1, 0).Add(feedback.SmoothCursorDuration))
	gotUI = ui.Rect{X: got.X, Y: got.Y, W: got.W, H: got.H}
	if active || gotUI != to {
		t.Fatalf("finished rect=%+v active=%v", gotUI, active)
	}
}

func TestSmoothCursorRequestsAnimationFramesOnlyWhileActive(t *testing.T) {
	for _, tc := range []struct {
		enabled, active, want bool
	}{{false, true, false}, {true, false, false}, {true, true, true}} {
		if got := feedback.SmoothCursorNeedsFrames(tc.enabled, tc.active); got != tc.want {
			t.Errorf("SmoothCursorNeedsFrames(%v,%v)=%v want %v", tc.enabled, tc.active, got, tc.want)
		}
	}
}

func TestSmoothBlockCaretInterpolatesUntransformedGeometry(t *testing.T) {
	state := feedback.Handle{}
	from := ui.Rect{X: 2, Y: 4, W: 5, H: 12}
	to := ui.Rect{X: 12, Y: 14, W: 9, H: 20}
	state.BeginCursor(0, 1, time.Unix(1, 0))
	state.SetCursorRects(feedback.Rect{X: from.X, Y: from.Y, W: from.W, H: from.H}, feedback.Rect{X: to.X, Y: to.Y, W: to.W, H: to.H})
	got, active := state.CursorRects(time.Unix(1, 0).Add(feedback.SmoothCursorDuration / 2))
	if !active || got.X != 7 || got.Y != 9 || got.W != 7 || got.H != 16 {
		t.Fatalf("block caret interpolation should preserve raw caret geometry: %+v active=%v", got, active)
	}
}

func TestSmoothCursorNavigationRebasesFromCurrentVisualPosition(t *testing.T) {
	start := time.Unix(10, 0)
	session := editormodel.NewSession("0123456789")
	_ = session.SetSelection(editormodel.Selection{Anchor: 5, Head: 5})
	controller := helix.New()
	controller.SetKeymap("vim")
	e := &Editor{
		Session:                session,
		opts:                   Options{SmoothCursor: true},
		vimState:               controller,
		smoothCursorNavigation: true,
		feedbackState:          feedback.Handle{},
	}
	e.feedbackState.BeginCursor(4, 5, start)
	e.feedbackState.SetCursorRects(
		feedback.Rect{X: 0, Y: 0, W: 2, H: 10},
		feedback.Rect{X: 20, Y: 10, W: 4, H: 20},
	)
	now := start.Add(feedback.SmoothCursorDuration / 2)
	e.beginSmoothCursorMoveAt(5, 6, false, now)
	want := feedback.Rect{X: 10, Y: 5, W: 3, H: 15}
	transition := e.feedbackState.CursorState()
	if !transition.FromRectSet || transition.FromRect != want || transition.Started != now {
		t.Fatalf("rebased animation from=%+v set=%v start=%v", transition.FromRect, transition.FromRectSet, transition.Started)
	}
}
