package editor

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	media "muse/nativeui/workbench/features/editor/blocks/media"
)

func TestInlineMathPreview(t *testing.T) {
	h := newHarness(t, "公式 $x^2$ 后文。")
	h.caretAt(len("公式 $x"))
	h.frame()
	_, _, tex, ok := h.e.activeInlineMath()
	if !ok || tex != "x^2" {
		t.Fatalf("active formula = %q, %v", tex, ok)
	}
	docH, caret, scroll := h.e.docH, h.e.Caret(), h.e.scroll.Y
	h.shot("inline-math-preview")
	if h.e.docH != docH || h.e.Caret() != caret || h.e.scroll.Y != scroll {
		t.Fatal("preview changed editor geometry or caret")
	}
	h.typ("+1")
	_, _, tex, ok = h.e.activeInlineMath()
	if !ok || tex != "x+1^2" {
		t.Fatalf("updated formula = %q, %v", tex, ok)
	}
	h.shot("inline-math-preview-updated")
	h.key(0, ui.KeyEscape)
	if !h.e.inlineMath.PreviewHidden() {
		t.Fatal("Escape did not hide preview")
	}
	h.shot("inline-math-preview-hidden")
	h.e.setCaret(0, false)
	h.frame()
	h.e.setCaret(len("公式 $x"), false)
	h.frame()
	if _, _, _, ok := h.e.activeInlineMath(); !ok || h.e.inlineMath.PreviewHidden() {
		t.Fatal("preview did not recover after reentry")
	}
	h.e.imeClient.SetMarkedText(nil, "a", ui.TextInputRange{Start: 1, End: 1})
	h.shot("inline-math-preview-ime")
}

func TestInlineMathPreviewStaysVisibleWhenCaretScrollsPastFormulaStart(t *testing.T) {
	tex := strings.Repeat(`\unsupported `, 120)
	src := "A long formula: $" + tex + "$"
	h := &harness{t: t, e: New(src, Options{FontSize: 16})}
	var viewport ui.Rect
	h.tt = ui.NewTester(func(c *ui.Context) {
		viewport = h.e.scrollHandle.Bounds(c)
		ui.Column(c).Fill().Children(func() { h.e.View(c).Grow(1) })
	}, 500, 180)
	h.frame()
	h.focus()
	h.caretAt(len(src) - 2)
	start := strings.IndexByte(src, '$')
	h.e.inlineMath.Begin(start, len(src))
	h.frame()

	startRect, startOK := h.e.caretRect(start)
	caretRect, caretOK := h.e.caretRect(h.e.Caret())
	if !startOK || !caretOK {
		t.Fatalf("missing formula/caret geometry: start=%v caret=%v", startOK, caretOK)
	}
	startRect.Y += viewport.Y - h.e.scroll.Y
	caretRect.Y += viewport.Y - h.e.scroll.Y
	if startRect.Y+startRect.H > viewport.Y {
		t.Fatalf("fixture did not scroll the formula start out of view: start=%+v viewport=%+v scroll=%v", startRect, viewport, h.e.scroll.Y)
	}
	if caretRect.Y+caretRect.H <= viewport.Y || caretRect.Y >= viewport.Y+viewport.H {
		t.Fatalf("caret is not visible in the viewport: caret=%+v viewport=%+v scroll=%v", caretRect, viewport, h.e.scroll.Y)
	}
	frame, ok := h.e.InlineMathPreview()
	if !ok || frame.Anchor != (media.Viewport{X: caretRect.X, Y: caretRect.Y, W: caretRect.W, H: caretRect.H}) {
		t.Fatalf("preview did not follow the visible caret after the formula start scrolled away: frame=%+v caret=%+v ok=%v", frame, caretRect, ok)
	}
	h.shot("inline-math-preview-after-source-start-scrolls-away")
}

func TestMathPreviewOffsetViewport(t *testing.T) {
	h := &harness{t: t, e: New("公式 $x^2$ 后文。", Options{FontSize: 16})}
	var bounds ui.Rect
	h.tt = ui.NewTester(func(c *ui.Context) {
		bounds = h.e.scrollHandle.Bounds(c)
		ui.Column(c).Fill().Children(func() {
			ui.Box(c).Height(56)
			ui.Row(c).Grow(1).Children(func() {
				ui.Box(c).Width(260)
				h.e.View(c).Grow(1)
			})
		})
	}, 1000, 700)
	h.frame()
	h.focus()
	h.caretAt(len("公式 $x"))
	h.frame()
	if bounds.X < 260 || bounds.Y < 56 {
		t.Fatalf("missing viewport offset: %+v", bounds)
	}
	frame, ok := h.e.InlineMathPreview()
	if !ok || frame.Viewport.X != bounds.X || frame.Viewport.Y != bounds.Y || frame.Anchor.X < bounds.X || frame.Anchor.Y < bounds.Y {
		t.Fatalf("incorrect offset preview snapshot: %+v", frame)
	}
	h.shot("inline-math-preview-offset")
}
