package inlineui

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/editor/input"
)

func TestPopupEmitsTypedNavigationActionsAndDetachedFrames(t *testing.T) {
	var popup Popup
	items := []Suggestion{{Target: "Alpha"}, {Target: "Beta"}}
	popup.Replace("a", 4, 5, items)
	items[0].Target = "mutated input"

	frame := popup.Frame()
	if !frame.Open() || frame.Query() != "a" || frame.Selected() != 0 {
		t.Fatalf("initial frame = %+v", frame)
	}
	frameItems := frame.Items()
	frameItems[0].Target = "mutated frame"
	if got := popup.Frame().Items()[0].Target; got != "Alpha" {
		t.Fatalf("popup retained externally mutable suggestion: %q", got)
	}

	action, handled := popup.HandleKey(input.KeyEvent{Key: input.KeyDown})
	if !handled || action != (Action{Kind: ActionSelectionChanged, Index: 1}) {
		t.Fatalf("down action = %+v, %v", action, handled)
	}
	action, handled = popup.HandleKey(input.KeyEvent{Key: input.KeyEnter})
	if !handled || action != (Action{Kind: ActionAccept, Index: 1}) {
		t.Fatalf("enter action = %+v, %v", action, handled)
	}
}

func TestPopupDismissalTracksQueryAndAnchor(t *testing.T) {
	var popup Popup
	popup.Replace("target", 12, 18, []Suggestion{{Target: "Target"}})
	action, handled := popup.HandleKey(input.KeyEvent{Key: input.KeyEscape})
	if !handled || action.Kind != ActionDismiss || popup.Frame().Open() {
		t.Fatalf("escape action/frame = %+v / %+v", action, popup.Frame())
	}
	if !popup.DismissedFor("target", 12) || popup.DismissedFor("target", 13) {
		t.Fatal("dismissal was not scoped to the original query anchor")
	}
	popup.Replace("target", 12, 20, []Suggestion{{Target: "Target"}})
	if popup.Frame().Open() {
		t.Fatal("same dismissed query reopened the popup")
	}
	popup.Replace("other", 12, 17, []Suggestion{{Target: "Other"}})
	if !popup.Frame().Open() {
		t.Fatal("new query did not reopen the popup")
	}
}

func TestBoundsAndHitTestUseDetachedGeometry(t *testing.T) {
	viewport := ui.Rect{X: 10, Y: 20, W: 500, H: 400}
	caret := ui.Rect{X: 100, Y: 60, W: 2, H: 18}
	bounds := Bounds(viewport, caret, 0, 2)
	if bounds.W <= 0 || bounds.H <= 0 || bounds.X < viewport.X || bounds.X+bounds.W > viewport.X+viewport.W {
		t.Fatalf("popup bounds escaped viewport: %+v", bounds)
	}
	var popup Popup
	popup.Replace("a", 0, 1, []Suggestion{{Target: "Alpha"}, {Target: "Another"}})
	popup.SetBounds(bounds)
	index, ok := popup.HitTest(bounds.X+10, bounds.Y+10)
	if !ok || index != 0 {
		t.Fatalf("first row hit = %d, %v", index, ok)
	}
	if _, ok := popup.HitTest(bounds.X+10, bounds.Y+bounds.H+1); ok {
		t.Fatal("pointer outside popup hit a row")
	}
}
