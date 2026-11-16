package editor

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	editorScroll "muse/nativeui/components/editor/scroll"
	"muse/ui/scrollbar"
)

func scrollControlHarness(t *testing.T, wide bool) (*harness, *leaf) {
	t.Helper()
	var src string
	if wide {
		cells := make([]string, 6)
		for i := range cells {
			cells[i] = strings.Repeat("wide-content-", 5) + string(rune('A'+i))
		}
		src = "| " + strings.Join(cells, " | ") + " |\n|" + strings.Repeat(" --- |", len(cells)) + "\n| " + strings.Join(cells, " | ") + " |"
	} else {
		src = "| A | B |\n| --- | --- |\n| one | two |"
	}
	src += "\n\n```go\npackage sample\n```\n\nafter marker\n\n" + strings.Repeat("scrollable document content\n\n", 50)
	h := newHarness(t, src)
	if wide {
		opts := h.e.Options()
		opts.MaxWidth = 420
		h.e.SetOptions(opts)
		h.frame()
	}
	return h, documentTestTable(t, h.e)
}

func tableOverlayControlRects(t *testing.T, h *harness, table *leaf) map[string]ui.Rect {
	t.Helper()
	viewport := ui.Rect{X: 0, Y: 0, W: h.e.elemW, H: h.e.viewH}
	rowY0, rowY1 := table.geom.rowY[0], table.geom.rowY[1]
	colRight := tableContentX(table) + table.geom.colX[1]
	addColumnX := tableContentX(table) + table.geom.w + 4
	return map[string]ui.Rect{
		"cell menu":     {X: colRight - 6, Y: table.y + (rowY0+rowY1)/2 - 12 - h.e.scroll.Y, W: 12, H: 24},
		"row handle":    {X: table.x - 18, Y: table.y + (rowY0+rowY1)/2 - h.e.lk.size*1.15/2 - h.e.scroll.Y, W: 14, H: h.e.lk.size * 1.15},
		"column handle": {X: (max(table.x, tableContentX(table)+table.geom.colX[0])+min(table.x+tableViewportWidth(table), tableContentX(table)+table.geom.colX[1]))/2 - 10, Y: table.y - 18 - h.e.scroll.Y, W: 20, H: 14},
		"add row":       {X: table.x + 2, Y: table.y + table.geom.rowY[len(table.geom.rowY)-1] + 10 - h.e.scroll.Y, W: min(table.geom.w, tableViewportWidth(table)), H: 14},
		"add column":    {X: addColumnX, Y: table.y + 2, W: 14, H: table.geom.rowY[len(table.geom.rowY)-1]},
		"viewport":      viewport,
	}
}

func TestTableOverlayControlsRouteActualMixedScroll(t *testing.T) {
	for _, name := range []string{"cell menu", "row handle", "column handle", "add row", "add column"} {
		t.Run(name, func(t *testing.T) {
			h, table := scrollControlHarness(t, true)
			if blockContentWidth(table) <= blockViewport(table).W {
				t.Fatalf("fixture table does not overflow: content=%v viewport=%v", blockContentWidth(table), blockViewport(table).W)
			}
			if name == "add column" {
				contentW := blockContentWidth(table)
				h.e.setBlockHorizontal(table, contentW-blockViewport(table).W)
				h.frame()
			}
			r := tableOverlayControlRects(t, h, table)[name]
			if r.W <= 0 || r.H <= 0 {
				t.Fatalf("invalid %s hit rect: %+v", name, r)
			}
			x, y := r.X+r.W/2, r.Y+r.H/2
			beforeX, beforeY := table.horizontal.Offset, h.e.scroll.Y
			dx := float32(24)
			if name == "add column" {
				dx = -24 // its external capsule is exposed at the rightmost edge
			}
			h.tt.Scroll(x, y, dx, 12)
			h.frame()
			if (table.horizontal.Offset-beforeX)*dx <= 0 || h.e.scroll.Y <= beforeY {
				t.Fatalf("%s did not route mixed touchpad scroll: horizontal %v -> %v, document %v -> %v", name, beforeX, table.horizontal.Offset, beforeY, h.e.scroll.Y)
			}
		})
	}
}

func TestTableOverlayControlsRouteActualVerticalScroll(t *testing.T) {
	for _, name := range []string{"cell menu", "row handle", "column handle", "add row", "add column"} {
		t.Run(name, func(t *testing.T) {
			h, table := scrollControlHarness(t, false)
			r := tableOverlayControlRects(t, h, table)[name]
			if r.W <= 0 || r.H <= 0 {
				t.Fatalf("invalid %s hit rect: %+v", name, r)
			}
			before := h.e.scroll.Y
			h.tt.Scroll(r.X+r.W/2, r.Y+r.H/2, 0, 24)
			h.frame()
			if h.e.scroll.Y <= before {
				t.Fatalf("%s blocked vertical touchpad scroll: %v -> %v", name, before, h.e.scroll.Y)
			}
		})
	}
}

func TestHorizontalBarHoverDoesNotShowOrExtendIt(t *testing.T) {
	h, table := scrollControlHarness(t, true)
	box := blockViewport(table)
	barY := box.Y + box.H - 4 - h.e.scroll.Y
	h.tt.Move(box.X+box.W/2, barY)
	h.frame()
	if table.horizontal.Shown || table.horizontal.Indicator.Visible(time.Now(), table.horizontal.Offset, 0) {
		t.Fatal("hover showed the horizontal scrollbar without scrolling")
	}
	h.tt.Scroll(box.X+box.W/2, box.Y+30-h.e.scroll.Y, 40, 0)
	h.frame()
	if !table.horizontal.Shown {
		t.Fatal("actual horizontal scrolling did not show the scrollbar")
	}
	until := table.horizontal.Indicator.Remaining(time.Now())
	track, _ := editorScroll.HorizontalBarRects(box, blockContentWidth(table), table.horizontal.Offset)
	h.tt.Move(track.X+track.W/2, track.Y+1-h.e.scroll.Y)
	h.tt.Advance(scrollbar.VisibleFor + time.Second)
	h.frame()
	if table.horizontal.Shown {
		t.Fatal("hover kept the horizontal scrollbar visible")
	}
	if until <= 0 {
		t.Fatal("actual scroll did not start the indicator lifetime")
	}
}

func TestHorizontalThumbDragPreservesSelectionsAndSourceMode(t *testing.T) {
	for _, mode := range []string{"text", "rectangle", "structure"} {
		t.Run(mode, func(t *testing.T) {
			h, table := scrollControlHarness(t, true)
			e := h.e
			code := -1
			for _, l := range e.leaves {
				if l.kind == kCode {
					code = l.lineStart
					break
				}
			}
			if code < 0 {
				t.Fatal("fixture has no source-toggleable code block")
			}
			e.sourceOK, e.sourceAt = true, code
			switch mode {
			case "text":
				a := strings.Index(e.Source(), "wide-content")
				e.Select(a, a+5)
			case "rectangle":
				a := strings.Index(e.Source(), "wide-content")
				e.setSessionSelection(a, a+5)
				e.block = &blockSelection{x0: 10, y0: 20, left: 10, right: 20, top: 20, bottom: 30, ranges: []Range{{Start: a, End: a + 5}}}
			case "structure":
				e.selectTableAxis(table, tableRows, 1, 1, false)
			}
			h.frame()
			table = documentTestTable(t, e)
			e.scroll.Y = max(table.y-20, 0)
			h.frame()
			beforeText := e.SelectedText()
			beforeAnchor, beforeCaret, beforeMode := e.Session.Selection().Anchor, e.Caret(), e.vimController().Mode()
			var beforeBlock *blockSelection
			if e.block != nil {
				b := *e.block
				b.ranges = append([]Range(nil), e.block.ranges...)
				beforeBlock = &b
			}
			var beforeTable *tableSelection
			if e.tableSelection != nil {
				s := *e.tableSelection
				beforeTable = &s
			}
			beforeSource, beforeSourceAt, beforeMarkdown := e.sourceOK, e.sourceAt, e.Source()

			box := blockViewport(table)
			h.tt.Scroll(box.X+box.W/2, box.Y+30-e.scroll.Y, 40, 0)
			h.frame()
			track, thumb := editorScroll.HorizontalBarRects(box, blockContentWidth(table), table.horizontal.Offset)
			if !table.horizontal.Shown || thumb.W <= 0 {
				t.Fatalf("scroll did not expose a usable horizontal thumb: shown=%v x=%v content=%v viewport=%+v scrollY=%v", table.horizontal.Shown, table.horizontal.Offset, blockContentWidth(table), box, e.scroll.Y)
			}
			x, y := thumb.X+thumb.W/2, thumb.Y+1-e.scroll.Y
			h.tt.Press(x, y)
			h.frame()
			if e.horizontalDrag == nil {
				t.Fatal("pressing the visible horizontal thumb did not start its drag")
			}
			h.tt.Move(track.X+track.W-1, y)
			h.frame()
			h.tt.Release(track.X+track.W-1, y)
			h.frame()
			if e.SelectedText() != beforeText || e.Session.Selection().Anchor != beforeAnchor || e.Caret() != beforeCaret || e.vimController().Mode() != beforeMode || !reflect.DeepEqual(e.block, beforeBlock) || !reflect.DeepEqual(e.tableSelection, beforeTable) {
				t.Fatalf("thumb drag changed %s selection: text=%q anchor=%d caret=%d mode=%q block=%+v table=%+v", mode, e.SelectedText(), e.Session.Selection().Anchor, e.Caret(), e.vimController().Mode(), e.block, e.tableSelection)
			}
			if e.sourceOK != beforeSource || e.sourceAt != beforeSourceAt || e.Source() != beforeMarkdown {
				t.Fatal("thumb drag changed source mode or Markdown")
			}
		})
	}
}
