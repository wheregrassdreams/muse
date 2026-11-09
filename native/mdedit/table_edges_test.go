package mdedit

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
	"strings"
	"testing"
	"time"
)

func TestTableEdgesControls(t *testing.T) {
	const table = "| X | 0 |\n| :--- | ---: |\n| a\\|b |  |"
	for _, dark := range []bool{false, true} {
		for _, column := range []bool{false, true} {
			t.Run(map[bool]string{false: "row", true: "column"}[column], func(t *testing.T) {
				src := table + "\n\n" + strings.Repeat("paragraph\n\n", 18) + table + "\n\nselected text"
				h := newHarness(t, src)
				e := h.e
				if dark {
					e = New(src, Options{FontSize: 16})
					h.e = e
					theme := ui.DarkTheme()
					h.tt = ui.NewTester(func(c *ui.Context) {
						ui.Themed(c, theme).Fill().Background(theme.Background).Children(func() { e.View(c).Grow(1) })
					}, 1000, 700)
					h.frame()
					h.focus()
				}
				e.anchor, e.caret = len(src)-13, len(src)-4
				e.wantReveal = false
				h.frame()
				var target *leaf
				for _, l := range e.leaves {
					if l.tab != nil {
						target = l
					}
				}
				e.scroll.Y = target.y - 100
				h.frame()
				x, y := target.x+target.geom.w/2, target.y-e.scroll.Y+target.geom.rowY[len(target.geom.rowY)-1]-2
				label, name := "在下方新增行", "table-edge-row"
				if column {
					x, y = target.x+target.geom.w-2, target.y-e.scroll.Y+15
					label, name = "在右侧新增列", "table-edge-column"
				}
				rows, cols := len(target.tab.rows), target.tab.cols()
				align := append([]parser.Align(nil), target.tab.align...)
				a, b, scroll := e.anchor, e.caret, e.scroll.Y
				h.tt.Move(x, y)
				h.frame()
				h.tt.Advance(time.Second)
				if dark {
					name += "-dark"
				}
				h.shot(name)
				if err := h.tt.Click(label); err != nil {
					t.Fatal(err)
				}
				h.frame()
				var got *leaf
				for _, l := range e.leaves {
					if l.tab != nil {
						got = l
					}
				}
				wr, wc := rows+1, cols
				if column {
					wr, wc = rows, cols+1
				}
				if got == nil || len(got.tab.rows) != wr || got.tab.cols() != wc {
					t.Fatal("parsed table dimensions")
				}
				for i, a := range align {
					if got.tab.align[i] != a {
						t.Fatal("alignment changed")
					}
				}
				if !strings.Contains(e.src, "a\\|b") {
					t.Fatal("escaped cell changed")
				}
				if e.src == src {
					t.Fatal("control did not edit source")
				}
				if !strings.HasPrefix(e.src, table+"\n\n") {
					t.Fatal("edited wrong table")
				}
				if e.src[e.anchor:e.caret] != src[a:b] {
					t.Fatal("selection changed")
				}
				if e.scroll.Y != scroll {
					t.Fatalf("scroll changed: %v -> %v", scroll, e.scroll.Y)
				}
				if !e.focused {
					t.Fatal("lost editing focus")
				}
				if !e.Undo() {
					t.Fatal("missing undo")
				}
				h.frame()
				h.want(src)
				if e.anchor != a || e.caret != b {
					t.Fatal("undo selection")
				}
			})
		}
	}

}

func TestTableEdgeWithoutOuterPipes(t *testing.T) {
	h := newHarness(t, "X | Y\n:--- | ---:\na\\|b | \n\nafter")
	e := h.e
	var target *leaf
	for _, l := range e.leaves {
		if l.tab != nil {
			target = l
		}
	}
	if target == nil {
		t.Fatal("missing table")
	}
	e.tableEdgeEdit(target, true)
	h.frame()
	for _, l := range e.leaves {
		if l.tab != nil {
			if l.tab.cols() != 3 {
				t.Fatalf("cols %d source %s", l.tab.cols(), e.src)
			}
			return
		}
	}
	t.Fatal("lost table")
}

func TestTableEdgeHoverLeaves(t *testing.T) {
	h := newHarness(t, "| A | B |\n| --- | --- |\n|  |  |")
	var l *leaf
	for _, v := range h.e.leaves {
		if v.tab != nil {
			l = v
		}
	}
	x, y := l.x+l.geom.w/2, l.y+l.geom.rowY[len(l.geom.rowY)-1]+12
	h.tt.Move(x, y)
	h.frame()
	on := h.tt.Image().At(int(x), int(y))
	h.tt.Move(900, 600)
	h.frame()
	off := h.tt.Image().At(int(x), int(y))
	if on == off {
		t.Fatal("hover strip did not disappear")
	}
}
