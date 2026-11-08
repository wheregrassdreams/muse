package mdedit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// wideDiagram is a Mermaid graph wider than the editor's column: pele lays a
// row of seven boxes out at more than the width of the page.
const wideDiagram = "```mermaid\ngraph LR\n" +
	"A[开始] --> B[解析]\n" +
	"B --> C[布局]\n" +
	"C --> D[绘制]\n" +
	"D --> E[校验]\n" +
	"E --> F[保存]\n" +
	"F --> G[完成]\n" +
	"G --> H[再校验一遍这块的画法]\n" +
	"```"

// diagramHarness returns a harness showing wideDiagram, and its leaf, with
// the caret outside the diagram so that it draws.
func diagramHarness(t *testing.T) (*harness, *leaf) {
	t.Helper()
	h := newHarness(t, wideDiagram+"\n\n尾\n")
	caret := len(h.e.Source())
	h.caretAt(caret)
	h.frame()
	l := h.e.leaves[0]
	if l.view == nil || l.view.diagram == nil {
		t.Fatal("the mermaid block did not draw as a diagram")
	}
	return h, l
}

func TestDiagramScrollsSideways(t *testing.T) {
	h, l := diagramHarness(t)
	v := l.view
	box, contentW := h.e.diagramBox(l, v, 0, 0)
	if contentW <= box.W {
		t.Fatalf("diagram width %v does not overflow the column %v", contentW, box.W)
	}
	if v.diagX != 0 {
		t.Fatalf("diagram starts scrolled: %v", v.diagX)
	}
	// A sideways swipe over the diagram scrolls it, not the document.
	h.tt.Scroll(box.X+30, box.Y+30, 120, 0)
	h.frame()
	if v.diagX <= 0 {
		t.Fatalf("swipe did not scroll the diagram: %v", v.diagX)
	}
	// It stops at the content's end.
	h.tt.Scroll(box.X+30, box.Y+30, contentW*2, 0)
	h.frame()
	if v.diagX != contentW-box.W {
		t.Fatalf("diagram scrolled to %v, want %v", v.diagX, contentW-box.W)
	}
	// Shift turns a wheel notch sideways, back toward the start. A wheel
	// event carries the modifiers the platform reports, which a Tester's
	// Wheel does not, so the event goes to the editor as it would come.
	h.e.scrollDiagram(ui.InputEvent{Kind: ui.InputScroll, X: box.X + 30, Y: box.Y + 30, DY: -60, Mods: ui.Shift})
	h.frame()
	if v.diagX >= contentW-box.W {
		t.Fatalf("Shift+wheel did not scroll the diagram back: %v", v.diagX)
	}
}

func TestDiagramScrollBarDrags(t *testing.T) {
	h, l := diagramHarness(t)
	v := l.view
	box, contentW := h.e.diagramBox(l, v, 0, 0)
	track, thumb := diagramBarRects(box, contentW, v.diagX)
	if !track.Contains(thumb.X+thumb.W/2, thumb.Y+1) {
		t.Fatal("the thumb is not on its track")
	}
	h.tt.Press(thumb.X+thumb.W/2, thumb.Y+1)
	h.frame()
	if h.e.diagDrag == nil {
		t.Fatal("pressing the bar did not start a drag")
	}
	h.tt.Move(track.X+track.W-1, track.Y+1)
	h.frame()
	h.tt.Release(track.X+track.W-1, track.Y+1)
	h.frame()
	if v.diagX != contentW-box.W {
		t.Fatalf("dragging the thumb to the end left the diagram at %v, want %v", v.diagX, contentW-box.W)
	}
	if h.e.diagDrag != nil {
		t.Fatal("letting go did not end the drag")
	}
}

func TestDiagramBarShowsOnlyOnOverflow(t *testing.T) {
	h := newHarness(t, "```mermaid\ngraph LR\nA[一] --> B[二]\n```\n\n尾\n")
	h.caretAt(len(h.e.Source()))
	h.frame()
	l := h.e.leaves[0]
	if l.view == nil || l.view.diagram == nil {
		t.Fatal("the mermaid block did not draw as a diagram")
	}
	box, contentW := h.e.diagramBox(l, l.view, 0, 0)
	if contentW > box.W {
		t.Fatalf("a small diagram overflows: %v > %v", contentW, box.W)
	}
	if _, _, _, _, ok := h.e.diagramBarAt(box.X+10, box.Y+box.H+4); ok {
		t.Fatal("a diagram that fits shows a scroll bar")
	}
}

// TestDiagramPans checks that a rendered diagram is dragged sideways with
// the pointer, not only with its bar, and that a press without a drag
// leaves the source alone.
func TestDiagramPans(t *testing.T) {
	h, l := diagramHarness(t)
	v := l.view
	box, contentW := h.e.diagramBox(l, v, 0, 0)
	if contentW <= box.W {
		t.Fatalf("diagram width %v does not overflow the column %v", contentW, box.W)
	}
	caret := h.e.caret
	x, y := box.X+box.W/2, box.Y+box.H/2

	h.tt.Move(x, y)
	if h.e.cursor != ui.CursorGrab {
		t.Fatalf("the pointer over a diagram is %v, want the grab shape", h.e.cursor)
	}
	h.tt.Press(x, y)
	h.frame()
	if h.e.diagPan == nil {
		t.Fatal("pressing the diagram did not start a pan")
	}
	if h.e.caret != caret {
		t.Fatal("pressing the diagram moved the caret into its source")
	}
	h.tt.Move(x-120, y)
	h.frame()
	if v.diagX <= 0 {
		t.Fatalf("dragging left did not pan the diagram: %v", v.diagX)
	}
	// Dragging past the start clamps at the first column.
	h.tt.Move(x+400, y)
	h.frame()
	if v.diagX != 0 {
		t.Fatalf("dragging right left the diagram at %v, want its start", v.diagX)
	}
	// A little more than the whole content travels puts the end in view.
	h.tt.Move(x-2*contentW, y)
	h.frame()
	if v.diagX != contentW-box.W {
		t.Fatalf("dragging left left the diagram at %v, want its end %v", v.diagX, contentW-box.W)
	}
	h.tt.Release(x-2*contentW, y)
	h.frame()
	if h.e.diagPan != nil {
		t.Fatal("letting go did not end the pan")
	}
	if h.e.caret != caret {
		t.Fatal("panning moved the caret into the diagram")
	}
}

// TestDiagramToggleSource checks that the control in a diagram's corner
// shows its Mermaid source, and brings the diagram back.
func TestDiagramToggleSource(t *testing.T) {
	h, l := diagramHarness(t)
	size := h.e.lk.size
	x := l.x + l.w - size*0.6 - 1
	y := l.y + size*0.7
	if got := h.e.diagramToggleAt(x, y); got != l {
		t.Fatal("the source control is not in the diagram's corner")
	}
	h.tt.Move(x, y)
	if h.e.cursor != ui.CursorPointer {
		t.Fatalf("the pointer over the source control is %v, want a hand", h.e.cursor)
	}

	caret := h.e.caret
	h.tt.ClickAt(x, y)
	h.frame()
	if h.e.caret != caret {
		t.Fatalf("the control moved the caret from %d to %d", caret, h.e.caret)
	}
	// The leaves are built again when the block changes what it shows.
	l = blockOfKind(h, kCode)
	if l == nil || l.view == nil || l.view.diagram != nil {
		t.Fatal("the block still draws its diagram after showing the source")
	}
	if !strings.Contains(l.view.disp.text, "```") {
		t.Fatalf("the source shows %q, without its fence", l.view.disp.text)
	}

	// With the source showing, the same control brings the diagram back,
	// and the caret is still where it was.
	x = l.x + l.w - size*0.6 - 1
	y = l.y + size*0.7
	if got := h.e.diagramToggleAt(x, y); got != l {
		t.Fatal("the diagram control is not in the source block's corner")
	}
	h.tt.ClickAt(x, y)
	for i := 0; i < 2; i++ {
		h.frame()
	}
	if h.e.caret != caret {
		t.Fatalf("the control moved the caret from %d to %d", caret, h.e.caret)
	}
	l = blockOfKind(h, kCode)
	if l == nil || l.view == nil || l.view.diagram == nil {
		t.Fatal("the diagram did not come back")
	}
}

// blockOfKind returns the first leaf of a kind, in the layout as it stands.
func blockOfKind(h *harness, k leafKind) *leaf {
	for _, l := range h.e.leaves {
		if l.kind == k {
			return l
		}
	}
	return nil
}

// TestDiagramKeepsItsScroll proves the sideways scroll belongs to the
// view: editing another block leaves it alone.
func TestDiagramKeepsItsScroll(t *testing.T) {
	h, l := diagramHarness(t)
	v := l.view
	box, _ := h.e.diagramBox(l, v, 0, 0)
	h.tt.Scroll(box.X+30, box.Y+30, 90, 0)
	h.frame()
	at := v.diagX
	if at <= 0 {
		t.Fatal("the diagram did not scroll")
	}
	h.caretAt(len(h.e.Source()))
	h.typ("tail")
	h.frame()
	if l.view == nil || l.view.diagram == nil {
		t.Fatal("the diagram is gone")
	}
	if got := l.view.diagX; got != at {
		t.Fatalf("an edit outside the diagram moved its scroll from %v to %v", at, got)
	}
	if !strings.HasSuffix(h.e.Source(), "tail") {
		t.Fatal("the edit did not land")
	}
}
