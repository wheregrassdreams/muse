package mdedit

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestMathPreviewBounds(t *testing.T) {
	view := ui.Rect{X: 100, Y: 50, W: 400, H: 300}
	for _, tc := range []struct {
		name           string
		anchor         ui.Rect
		w, h           float32
		visible, above bool
	}{
		{"below", ui.Rect{X: 120, Y: 80, W: 2, H: 20}, 200, 60, true, false},
		{"flip", ui.Rect{X: 480, Y: 300, W: 2, H: 20}, 200, 60, true, true},
		{"oversize", ui.Rect{X: 120, Y: 100, W: 2, H: 20}, 900, 900, true, false},
		{"scrolled out", ui.Rect{X: 120, Y: 10, W: 2, H: 20}, 200, 60, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, ok := mathPreviewBounds(view, tc.anchor, tc.w, tc.h)
			if ok != tc.visible {
				t.Fatalf("visible=%v", ok)
			}
			if !ok {
				return
			}
			if r.X < view.X || r.Y < view.Y || r.X+r.W > view.X+view.W || r.Y+r.H > view.Y+view.H {
				t.Fatalf("outside viewport: %+v", r)
			}
			if tc.above && r.Y+r.H > tc.anchor.Y {
				t.Fatalf("did not flip: %+v", r)
			}
		})
	}
}

func TestInlineMathPreview(t *testing.T) {
	h := newHarness(t, "公式 $x^2$ 后文。")
	h.caretAt(len("公式 $x"))
	h.frame()
	_, _, tex, ok := h.e.activeInlineMath()
	if !ok || tex != "x^2" {
		t.Fatalf("active formula = %q, %v", tex, ok)
	}
	docH, caret, scroll := h.e.docH, h.e.caret, h.e.scroll.Y
	h.shot("inline-math-preview")
	if h.e.docH != docH || h.e.caret != caret || h.e.scroll.Y != scroll {
		t.Fatal("preview changed editor geometry or caret")
	}
	h.typ("+1")
	_, _, tex, ok = h.e.activeInlineMath()
	if !ok || tex != "x+1^2" {
		t.Fatalf("updated formula = %q, %v", tex, ok)
	}
	h.shot("inline-math-preview-updated")
	h.key(0, ui.KeyEscape)
	if !h.e.mathPreviewHidden {
		t.Fatal("Escape did not hide preview")
	}
	h.shot("inline-math-preview-hidden")
	h.e.setCaret(0, false)
	h.frame()
	h.e.setCaret(len("公式 $x"), false)
	h.frame()
	if _, _, _, ok := h.e.activeInlineMath(); !ok || h.e.mathPreviewHidden {
		t.Fatal("preview did not recover after reentry")
	}
	h.e.SetMarkedText(nil, "a", ui.TextInputRange{Start: 1, End: 1})
	h.shot("inline-math-preview-ime")
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
	anchor, ok := h.e.caretRect(h.e.caret)
	if !ok {
		t.Fatal("missing anchor")
	}
	anchor.X += bounds.X
	anchor.Y += bounds.Y - h.e.scroll.Y
	r, ok := mathPreviewBounds(bounds, anchor, 200, 60)
	if !ok || r.X < bounds.X || r.Y < bounds.Y {
		t.Fatalf("incorrect offset preview: %+v", r)
	}
	h.shot("inline-math-preview-offset")
}

func TestDarkMathPreviewOccludesQuote(t *testing.T) {
	const src = "当 $A=\\frac{1}{2}$ 时\n\n> 函数要求 domain 中的每一个元素都映射到 codomain。\n> 第二行引用文字。\n"
	theme := ui.DarkTheme()
	var viewport ui.Rect
	h := &harness{t: t, e: New(src, Options{FontSize: 16})}
	h.tt = ui.NewTester(func(c *ui.Context) {
		viewport = h.e.scrollHandle.Bounds(c)
		ui.Themed(c, theme).Fill().Background(theme.Background).Children(func() { h.e.View(c).Grow(1) })
	}, 1000, 700)
	h.frame()
	h.focus()
	h.caretAt(len("当 $A"))
	h.frame()
	start, _, tex, ok := h.e.activeInlineMath()
	if !ok {
		t.Fatal("no formula")
	}
	anchor, _ := h.e.caretRect(start)
	anchor.X += viewport.X
	anchor.Y += viewport.Y - h.e.scroll.Y
	obj := h.e.mathObject(tex, h.e.lk.size, false)
	if obj == nil {
		t.Fatal("formula render failed")
	}
	card, ok := mathPreviewBounds(viewport, anchor, obj.w+24, obj.h+obj.descent+24)
	if !ok {
		t.Fatal("preview not visible")
	}
	img := h.tt.Image()
	matched, total := 0, 0
	for y := int(card.Y) + 4; y < int(card.Y+card.H)-4; y++ {
		for x := int(card.X) + 4; x < int(card.X+card.W)-4; x++ {
			total++
			c := img.RGBAAt(x, y)
			if c.R == theme.Background.R && c.G == theme.Background.G && c.B == theme.Background.B {
				matched++
			}
		}
	}
	if matched*100 < total*70 {
		t.Fatalf("preview surface is translucent/occluded: %d of %d opaque surface pixels", matched, total)
	}
	h.shot("inline-math-over-quote-dark")
}
