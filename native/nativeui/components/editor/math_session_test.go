package editor

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	inlineblock "muse/editor/blocks/inline"
)

func requireMathCodeStyle(t *testing.T, h *harness, start, end int) {
	t.Helper()
	l := h.e.leafAt(start)
	covered := start
	displayStart, displayEnd := -1, 0
	for _, p := range l.view.disp.pieces {
		a, z := p.s0+l.lineStart, p.s1+l.lineStart
		if z <= start || a >= end {
			continue
		}
		if p.kind != pText || p.span.Font != h.e.lk.mono || p.span.Background.A != 0 {
			t.Fatalf("math source %d..%d lost code style: %+v", a, z, p)
		}
		if a != covered {
			t.Fatalf("math source byte mapping has a gap or overlap at %d, next piece starts at %d", covered, a)
		}
		if got, want := p.text, h.e.Source()[a:z]; got != want {
			t.Fatalf("math source piece changed source bytes at %d..%d: got %q want %q", a, z, got, want)
		}
		if p.span.Color != h.e.lk.codeText {
			matched := false
			for kind := editor.TokenKeyword; kind <= editor.TokenInvalid; kind++ {
				if p.span.Color == h.e.lk.code.TokenColor(kind) {
					matched = true
					break
				}
			}
			if !matched {
				t.Fatalf("math source has an unrecognized syntax color at %d..%d: %+v", a, z, p.span.Color)
			}
		}
		if displayStart < 0 {
			displayStart = p.d0
		} else {
			displayStart = min(displayStart, p.d0)
		}
		displayEnd = max(displayEnd, p.d1)
		covered = max(covered, z)
	}
	if covered < end {
		t.Fatalf("source styled through %d, want %d", covered, end)
	}
	boxFound := false
	for _, box := range l.view.disp.codeBoxes {
		if box.range_.Start <= displayStart && box.range_.End >= displayEnd {
			boxFound = true
			for _, r := range l.view.layout.SelectionRects(box.range_) {
				padX := h.e.lk.size * 0.2
				x := int(l.x + l.view.tx + r.X - padX + 1)
				y := int(l.y + l.view.ty + r.Y + r.H/2)
				if pixel := h.tt.Image().RGBAAt(x, y); !sameRGB(pixel, h.e.lk.codeBg) {
					t.Fatalf("math source shared background missing at %d,%d: pixel=%v want=%v", x, y, pixel, h.e.lk.codeBg)
				}
			}
		}
	}
	if !boxFound {
		t.Fatalf("math source %d..%d has no shared code background box", start, end)
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
	activeRange, _ := h.e.inlineMath.Range()
	if activeRange.Start != start || activeRange.End != end {
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
			if h.e.Caret() >= 7 && h.e.Caret() <= 12 {
				t.Fatalf("caret did not leave math: %d", h.e.Caret())
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
			h.caretAt(len(h.e.Source()))
			if strings.Contains(h.e.leaves[0].view.disp.text, "$x") {
				t.Fatal("formula starts expanded")
			}
			// Only the anchor changes; caret and reveal offset remain unchanged.
			h.e.setSessionAnchor(7)
			h.frame()
			if !strings.Contains(h.e.leaves[0].view.disp.text, "$x") {
				t.Fatal("selection did not expand formula")
			}
			switch method {
			case "anchor only":
				h.e.setSessionAnchor(h.e.Caret())
				h.frame()
			case "keyboard":
				h.key(0, ui.KeyRight)
			case "mouse":
				r, ok := h.e.caretRect(len(h.e.Source()))
				if !ok {
					t.Fatal("no caret rect")
				}
				h.tt.ClickAt(r.X, r.Y+r.H/2-h.e.scroll.Y)
				h.frame()
			}
			if h.e.Session.Selection().Anchor != h.e.Caret() {
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
	if h.e.focused || h.e.inlineMath.Active() {
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
	h.e.inlineMath.SetPreviewHidden(true)
	// Mouse and selection operations may assign the caret directly.
	h.e.setSessionSelection(9, 9)
	h.frame()
	start, end, tex, ok := h.e.activeInlineMath()
	if !ok || start != 8 || end != 11 || tex != "y" || h.e.inlineMath.PreviewHidden() {
		t.Fatalf("switched session=%d..%d %q ok=%v hidden=%v", start, end, tex, ok, h.e.inlineMath.PreviewHidden())
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
			content := inlineblock.MathContentBounds(h.e.Source()[start:end])
			requireMathCodeStyle(t, h, start+content.Start, start+content.End)
			if edit == "escaped closer" && tex != `xyz\$` {
				t.Fatalf("escaped closer omitted: %q", tex)
			}
			h.shot("inline-math-source-" + strings.ReplaceAll(edit, " ", "-"))
		})
	}
}
