package mdedit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func requireMathCodeStyle(t *testing.T, h *harness, start, end int) {
	t.Helper()
	l := h.e.leafAt(start)
	covered := start
	for _, p := range l.view.disp.pieces {
		a, z := p.s0+l.lineStart, p.s1+l.lineStart
		if z <= start || a >= end {
			continue
		}
		if p.kind != pText || p.span.Font != h.e.lk.mono || p.span.Background != h.e.lk.codeBg {
			t.Fatalf("math source %d..%d lost code style: %+v", a, z, p)
		}
		covered = max(covered, z)
	}
	if covered < end {
		t.Fatalf("source styled through %d, want %d", covered, end)
	}
}

func TestInlineMathSessionSourceStyle(t *testing.T) {
	for _, edit := range []string{"valid", "invalid TeX", "missing closer"} {
		t.Run(edit, func(t *testing.T) {
			h := newHarness(t, "**bold** $xyz$ [link](url)")
			start := len("**bold** $")
			h.caretAt(start + 1)
			switch edit {
			case "invalid TeX":
				h.typ(`\invalid{`)
			case "missing closer":
				h.caretAt(len("**bold** $xyz$"))
				h.key(0, ui.KeyBackspace)
			}
			h.frame()
			_, end, _, ok := h.e.activeInlineMath()
			if !ok {
				t.Fatal("no active session")
			}
			if edit != "missing closer" {
				end--
			}
			requireMathCodeStyle(t, h, start, end)
			h.shot("inline-math-source-" + strings.ReplaceAll(edit, " ", "-"))
			text := h.e.leaves[0].view.disp.text
			if strings.Contains(text, "**") || strings.Contains(text, "(url)") {
				t.Fatalf("surrounding projection lost: %q", text)
			}
		})
	}
}

func TestInlineMathSessionSpeculativeView(t *testing.T) {
	h := newHarness(t, "before $x^2$ after\n\nnext")
	h.caretAt(len("before $x"))
	l := h.e.leaves[0]
	// Navigation asks how the leaf would look with a caret elsewhere.
	v := h.e.viewWith(l, l.w, false, 0, -1)
	if strings.Contains(v.disp.text, "$x") {
		t.Fatalf("collapsed view cached expanded session: %q", v.disp.text)
	}
	// A hypothetical caret in a different formula must not change the session.
	h = newHarness(t, "a $x$ b $y$ c")
	h.caretAt(3)
	start, end, _, _ := h.e.activeInlineMath()
	l = h.e.leaves[0]
	h.e.viewWith(l, l.w, false, 0, 9)
	if h.e.inlineMathStart != start || h.e.inlineMathEnd != end {
		t.Fatal("speculative view changed active session")
	}
}

func TestInlineMathSessionLeavingCollapses(t *testing.T) {
	for _, method := range []string{"left", "right", "down", "mouse"} {
		t.Run(method, func(t *testing.T) {
			const src = "before $x^2$ after\n\nnext"
			h := newHarness(t, src)
			h.caretAt(len("before $x"))
			// Prime the same speculative view used by navigation, then leave.
			l := h.e.leaves[0]
			h.e.viewWith(l, l.w, false, 0, -1)
			switch method {
			case "left":
				for i := 0; i < 4; i++ {
					h.key(0, ui.KeyLeft)
				}
			case "right":
				for i := 0; i < 4; i++ {
					h.key(0, ui.KeyRight)
				}
			case "down":
				h.key(0, ui.KeyDown)
			case "mouse":
				next := h.e.leaves[len(h.e.leaves)-1]
				h.tt.ClickAt(next.x+2, next.y+next.h/2-h.e.scroll.Y)
				h.frame()
			}
			if h.e.caret >= 7 && h.e.caret <= 12 {
				t.Fatalf("caret did not leave math: %d", h.e.caret)
			}
			if strings.Contains(h.e.leaves[0].view.disp.text, "$x") {
				t.Fatalf("formula stayed expanded: %q", h.e.leaves[0].view.disp.text)
			}
			if _, _, _, ok := h.e.activeInlineMath(); ok {
				t.Fatal("session still active")
			}
			rendered := false
			for _, p := range h.e.leaves[0].view.disp.pieces {
				if p.kind == pAtom && p.obj != nil && p.s0 == 7 && p.s1 == 12 {
					rendered = true
				}
			}
			if !rendered {
				t.Fatal("formula did not return to a rendered object")
			}
			h.shot("inline-math-collapsed-" + method)
			h.want(src)
		})
	}
}

func TestInlineMathSessionSelectionLeaving(t *testing.T) {
	for _, method := range []string{"anchor only", "keyboard", "mouse"} {
		t.Run(method, func(t *testing.T) {
			h := newHarness(t, "before $x^2$ after")
			h.caretAt(len(h.e.src))
			if strings.Contains(h.e.leaves[0].view.disp.text, "$x") {
				t.Fatal("formula starts expanded")
			}
			// Only the anchor changes; caret and reveal offset remain unchanged.
			h.e.anchor = 7
			h.frame()
			if !strings.Contains(h.e.leaves[0].view.disp.text, "$x") {
				t.Fatal("selection did not expand formula")
			}
			switch method {
			case "anchor only":
				h.e.anchor = h.e.caret
				h.frame()
			case "keyboard":
				h.key(0, ui.KeyRight)
			case "mouse":
				r, ok := h.e.caretRect(len(h.e.src))
				if !ok {
					t.Fatal("no caret rect")
				}
				h.tt.ClickAt(r.X, r.Y+r.H/2-h.e.scroll.Y)
				h.frame()
			}
			if h.e.anchor != h.e.caret {
				t.Fatal("selection did not clear")
			}
			if strings.Contains(h.e.leaves[0].view.disp.text, "$x") {
				t.Fatalf("selection left formula expanded: %q", h.e.leaves[0].view.disp.text)
			}
		})
	}
}

func TestInlineMathSessionFocusLoss(t *testing.T) {
	var other ui.Handle
	h := &harness{t: t, e: New("before $x^2$ after", Options{FontSize: 16})}
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			h.e.View(c).Grow(1)
			ui.Box(c).Height(40).Focusable().Bind(&other)
		})
	}, 1000, 700)
	h.frame()
	h.focus()
	h.caretAt(len("before $x"))
	requireMathCodeStyle(t, h, 8, 11)
	other.Focus()
	h.frame()
	h.frame()
	if h.e.focused || h.e.inlineMathOK {
		t.Fatal("focus loss retained math session")
	}
	if strings.Contains(h.e.leaves[0].view.disp.text, "$x") {
		t.Fatal("focus loss did not collapse formula")
	}
	h.focus()
	requireMathCodeStyle(t, h, 8, 11)
}

func TestInlineMathSessionSwitchFormula(t *testing.T) {
	h := newHarness(t, "a $x$ b $y$ c")
	h.caretAt(3)
	h.e.mathPreviewHidden = true
	// Mouse and selection operations may assign the caret directly.
	h.e.anchor, h.e.caret = 9, 9
	h.frame()
	start, end, tex, ok := h.e.activeInlineMath()
	if !ok || start != 8 || end != 11 || tex != "y" || h.e.mathPreviewHidden {
		t.Fatalf("switched session=%d..%d %q ok=%v hidden=%v", start, end, tex, ok, h.e.mathPreviewHidden)
	}
	requireMathCodeStyle(t, h, 9, 10)
	h.typ("+1")
	requireMathCodeStyle(t, h, 9, 12)
}

func TestInlineMathSessionEditedDelimitersStayStyled(t *testing.T) {
	for _, edit := range []string{"missing opener", "escaped closer"} {
		t.Run(edit, func(t *testing.T) {
			h := newHarness(t, "before $xyz$ after")
			h.caretAt(9)
			switch edit {
			case "missing opener":
				h.caretAt(8)
				h.key(0, ui.KeyBackspace)
			case "escaped closer":
				h.caretAt(11)
				h.typ(`\`)
			}
			start, end, tex, ok := h.e.activeInlineMath()
			if !ok {
				t.Fatal("edited delimiter lost session")
			}
			cs, ce := inlineMathContent(h.e.src[start:end])
			requireMathCodeStyle(t, h, start+cs, start+ce)
			if edit == "escaped closer" && tex != `xyz\$` {
				t.Fatalf("escaped closer omitted: %q", tex)
			}
			h.shot("inline-math-source-" + strings.ReplaceAll(edit, " ", "-"))
		})
	}
}
