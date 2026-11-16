package editor

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/ui/scrollbar"
)

func TestHorizontalAcceptanceTableMovesAsOneClippedObject(t *testing.T) {
	const src = "before\n\n| C0 | C1 | C2 | C3 | C4 | C5 |\n| --- | --- | --- | --- | --- | --- |\n| a | b | c | d | e | f |\n\nafter"
	h := newHarness(t, src)
	opts := h.e.Options()
	opts.MaxWidth = 260
	h.e.SetOptions(opts)
	h.frame()
	h.frame()
	table := documentTestTable(t, h.e)
	box := blockViewport(table)
	if table.geom.w <= box.W+100 {
		t.Fatal("fixture must have a genuinely wider table than its viewport")
	}
	cell := table.tab.rows[0][2]
	beforeCaret, ok := h.e.caretRect(cell.start)
	if !ok {
		t.Fatal("table cell has no native caret geometry")
	}
	lineY := int(table.y + table.geom.rowY[1] - 3)
	beforeGrid := h.tt.Image().RGBAAt(int(table.x+table.geom.colX[1]), lineY)
	bg := h.tt.Image().RGBAAt(5, 5)
	if beforeGrid == bg {
		t.Fatal("grid probe does not exercise a rendered column border")
	}
	beforeY, beforeSource := h.e.scroll.Y, h.e.Source()
	h.tt.Scroll(box.X+24, box.Y+20, 90, 0)
	h.frame()
	if table.horizontal.Offset != 90 || h.e.scroll.Y != beforeY || h.e.Source() != beforeSource {
		t.Fatalf("horizontal scroll changed source/vertical page position: x=%v y=%v", table.horizontal.Offset, h.e.scroll.Y)
	}
	afterCaret, _ := h.e.caretRect(cell.start)
	if math.Abs(float64(afterCaret.X-(beforeCaret.X-90))) > .1 {
		t.Fatalf("table text did not use its grid's transform: before=%+v after=%+v", beforeCaret, afterCaret)
	}
	gridX := int(table.x + table.geom.colX[2] - 90)
	if got := h.tt.Image().RGBAAt(gridX, lineY); got != beforeGrid {
		t.Fatalf("grid stayed fixed while text moved: grid pixel=%v want=%v", got, beforeGrid)
	}
	if got := h.e.hitTest(afterCaret.X+.1, afterCaret.Y+afterCaret.H/2); got != cell.start {
		t.Fatalf("scrolled cell hit changed source column: got=%d want=%d", got, cell.start)
	}
	for y := int(box.Y + 4); y < int(box.Y+box.H-4); y++ {
		for _, x := range []int{int(box.X - 12), int(box.X+box.W) + 12} {
			if got := h.tt.Image().RGBAAt(x, y); got != bg {
				t.Fatalf("table content escaped its shared viewport at %d,%d: %v", x, y, got)
			}
		}
	}
	h.shot("horizontal-table-as-one-object")
	if !table.horizontal.Shown {
		t.Fatal("actual horizontal movement did not reveal the scrollbar")
	}
	h.tt.Advance(scrollbar.VisibleFor + time.Second)
	h.frame()
	if table.horizontal.Shown {
		t.Fatal("hover kept the block's scrollbar visible after scrolling stopped")
	}
}

func TestHorizontalAcceptanceCodeAndMathShareClippedBlockPolicy(t *testing.T) {
	src := "before\n\n```go\nvar text = \"" + strings.Repeat("long_content_", 30) + "\"\n```\n\n\\[\n" + strings.Repeat("x^2 + ", 30) + "y^2\n\\]\n\nafter"
	for _, dark := range []bool{false, true} {
		name, theme := "light", ui.LightTheme()
		if dark {
			name, theme = "dark", ui.DarkTheme()
		}
		t.Run(name, func(t *testing.T) {
			h := &harness{t: t, e: New(src, Options{FontSize: 16, MaxWidth: 420, PadX: 24})}
			h.tt = ui.NewTester(func(c *ui.Context) {
				ui.Themed(c, theme).Fill().Background(theme.Background).Children(func() { h.e.View(c).Grow(1) })
			}, 620, 520)
			h.frame()
			h.frame()
			h.e.Select(len(src), len(src))
			h.frame()
			for _, l := range h.e.leaves {
				if l.kind != kCode && l.kind != kMath {
					continue
				}
				box := blockViewport(l)
				if blockContentWidth(l) <= box.W+100 {
					t.Fatalf("long block was shrunk/wrapped instead of overflowing: kind=%v content=%v viewport=%v", l.kind, blockContentWidth(l), box.W)
				}
				h.tt.Scroll(box.X+20, box.Y+box.H/2, 120, 0)
				h.frame()
				if l.horizontal.Offset != 120 || !l.horizontal.Shown {
					t.Fatalf("block did not scroll independently: kind=%v offset=%v shown=%v", l.kind, l.horizontal.Offset, l.horizontal.Shown)
				}
				bg := h.tt.Image().RGBAAt(2, 2)
				for y := int(box.Y + 4); y < int(box.Y+box.H-4); y++ {
					for _, x := range []int{int(box.X - 8), int(box.X+box.W) + 8} {
						if pixel := h.tt.Image().RGBAAt(x, y); pixel != bg {
							t.Fatalf("block escaped viewport: kind=%v pixel=(%d,%d) color=%v", l.kind, x, y, pixel)
						}
					}
				}
			}
			if h.e.Source() != src || h.e.scroll.Y != 0 {
				t.Fatal("block scrolling changed source or moved the page vertically")
			}
			saveTableScrollShot(t, h, "horizontal-code-math-"+name)
		})
	}
}
