package mdedit

import (
	"image/color"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Inline code is a shared background shape so a text selection can paint over
// its body and horizontal padding as one continuous region.
func TestInlineCodeSelectionCoversPillAndPadding(t *testing.T) {
	const src = "前普通文字 `混合 inline code` 后普通文字"
	for _, dark := range []bool{false, true} {
		name, theme := "light", ui.LightTheme()
		if dark {
			name, theme = "dark", ui.DarkTheme()
		}
		t.Run(name, func(t *testing.T) {
			h := &harness{t: t, e: New(src, Options{FontSize: 16})}
			h.tt = ui.NewTester(func(c *ui.Context) {
				ui.Themed(c, theme).Fill().Background(theme.Background).Children(func() { h.e.View(c).Grow(1) })
			}, 1000, 700)
			h.frame()
			h.focus()
			codeStart, codeEnd := len("前普通文字 "), len("前普通文字 `混合 inline code`")
			h.caretAt(codeStart + len("`混"))
			partialStart, partialEnd := codeStart+len("`混"), codeEnd-len("code`")
			for _, tc := range []struct {
				name      string
				anchor    int
				caret     int
				wholeCode bool
			}{
				{name: "forward whole", anchor: 0, caret: len(src), wholeCode: true},
				{name: "reverse whole", anchor: len(src), caret: 0, wholeCode: true},
				{name: "forward partial", anchor: partialStart, caret: partialEnd},
				{name: "reverse partial", anchor: partialEnd, caret: partialStart},
			} {
				t.Run(tc.name, func(t *testing.T) {
					h.e.anchor, h.e.caret = tc.anchor, tc.caret
					h.e.wantReveal = true
					h.frame()
					l := h.e.leafAt(codeStart)
					v := l.view
					if len(v.disp.codeBoxes) == 0 {
						t.Fatal("revealed inline code has no shared background box")
					}
					box := v.disp.codeBoxes[0]
					var boxRect ui.Rect
					for _, r := range v.layout.SelectionRects(box.range_) {
						padX := h.e.lk.size * 0.2
						boxRect = ui.Rect{X: l.x + v.tx + r.X - padX, Y: l.y + v.ty + r.Y, W: r.W + 2*padX, H: r.H}
						break
					}
					if boxRect.W <= 0 || boxRect.H <= 0 {
						t.Fatal("inline code has no measurable background geometry")
					}
					img := h.tt.Image()
					if tc.wholeCode {
						for _, x := range []float32{boxRect.X + 1, boxRect.X + boxRect.W - 1} {
							pixel := img.RGBAAt(int(x), int(boxRect.Y+boxRect.H/2))
							if sameRGB(pixel, h.e.lk.codeBg) {
								t.Fatalf("selection misses code pill padding at x=%.1f: code background is still visible", x)
							}
						}
					} else {
						da := v.disp.looseDisp(partialStart - l.lineStart)
						dz := v.disp.looseDisp(partialEnd - l.lineStart)
						rects := v.layout.SelectionRects(ui.TextInputRange{Start: da, End: dz})
						if len(rects) == 0 {
							t.Fatal("partial selection has no layout geometry")
						}
						r := rects[0]
						selected := img.RGBAAt(int(l.x+v.tx+r.X+1), int(l.y+v.ty+r.Y+r.H/2))
						if sameRGB(selected, h.e.lk.codeBg) {
							t.Fatal("partial selected code text remains covered by code background")
						}
						unselected := img.RGBAAt(int(boxRect.X+1), int(boxRect.Y+boxRect.H/2))
						if !sameRGB(unselected, h.e.lk.codeBg) {
							t.Fatalf("partial selection spread to unselected code padding: %v", unselected)
						}
						if h.e.SelectedText() == "" {
							t.Fatal("partial inline code selection was lost")
						}
					}
					h.shot("inline-code-selection-" + name + "-" + tc.name)
				})
			}
		})
	}
}

func sameRGB(got color.RGBA, want ui.Color) bool {
	return got.R == want.R && got.G == want.G && got.B == want.B
}

func TestInlineCodeFoldAndWrapKeepSharedStyle(t *testing.T) {
	const src = "折叠样式 `中英混排 inline code wraps across this narrow paragraph` 后文"
	h := newHarness(t, src)
	opts := h.e.Options()
	opts.MaxWidth = 260
	h.e.SetOptions(opts)
	h.caretAt(len(src))
	l := h.e.leafAt(len("折叠样式 "))
	if strings.Contains(l.view.disp.text, "`中英") {
		t.Fatalf("code is not folded with the caret outside it: %q", l.view.disp.text)
	}
	if len(l.view.disp.codeBoxes) != 1 {
		t.Fatalf("folded code has %d shared backgrounds, want one", len(l.view.disp.codeBoxes))
	}
	box := l.view.disp.codeBoxes[0]
	rects := l.view.layout.SelectionRects(box.range_)
	if len(rects) < 2 {
		t.Fatalf("test code did not wrap: got %d line rects", len(rects))
	}
	padX := h.e.lk.size * 0.2
	first := rects[0]
	codeBG := h.tt.Image().RGBAAt(int(l.x+l.view.tx+first.X-padX/2), int(l.y+l.view.ty+first.Y+first.H/2))
	if !sameRGB(codeBG, h.e.lk.codeBg) {
		t.Fatalf("folded code lost its pill background: pixel=%v want=%v", codeBG, h.e.lk.codeBg)
	}
	h.shot("inline-code-folded-wrap")
	h.caretAt(len("折叠样式 `中英"))
	l = h.e.leafAt(len("折叠样式 "))
	if !strings.Contains(l.view.disp.text, "`中英") {
		t.Fatalf("code did not expand at its caret: %q", l.view.disp.text)
	}
	h.e.anchor, h.e.caret = 0, len(src)
	h.e.wantReveal = true
	h.frame()
	l = h.e.leafAt(len("折叠样式 "))
	box = l.view.disp.codeBoxes[0]
	if rects := l.view.layout.SelectionRects(box.range_); len(rects) < 2 {
		t.Fatalf("expanded selected code lost wrapping: got %d line rects", len(rects))
	}
	h.shot("inline-code-expanded-wrap-selection")
}
