package editor

import (
	"fmt"
	helix "github.com/wheregrassdreams/muse-editor/text/helix"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTablePointerReleaseKeepsBoundaryEndpoint(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, right := range []bool{false, true} {
			t.Run(fmt.Sprintf("reverse=%v/right=%v", reverse, right), func(t *testing.T) {
				h := newHarness(t, documentTable)
				table := documentTestTable(t, h.e)
				row := 1
				from := h.e.leaves[0]
				x, y := from.x+2, from.y+from.h/2
				if reverse {
					from = h.e.leaves[len(h.e.leaves)-1]
					x, y = from.x+2, from.y+from.h/2
				}
				if !h.e.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, Clicks: 1, X: x, Y: y}) {
					t.Fatal("press not handled")
				}
				x, y = table.x-4, table.y+(table.geom.rowY[row]+table.geom.rowY[row+1])/2
				if right {
					x = table.x + table.geom.w + 4
				}
				h.e.input(ui.InputEvent{Kind: ui.InputPointerMove, Button: 0, X: x, Y: y})
				anchor, caret := h.e.Session.Selection().Anchor, h.e.Caret()
				want := h.e.tableRowBoundary(table, row, right)
				if caret != want {
					t.Fatalf("drag did not hit row boundary: got %d want %d", caret, want)
				}
				h.shot(fmt.Sprintf("pointer-before-release-%v-%v", reverse, right))
				h.e.input(ui.InputEvent{Kind: ui.InputPointerUp, Button: 0, X: x, Y: y})
				h.frame()
				if h.e.Session.Selection().Anchor != anchor || h.e.Caret() != caret {
					t.Fatalf("release changed mouse endpoint: anchor/caret [%d,%d] -> [%d,%d]", anchor, caret, h.e.Session.Selection().Anchor, h.e.Caret())
				}
				h.shot(fmt.Sprintf("pointer-after-release-%v-%v", reverse, right))
			})
		}
	}
}

func TestPointerDragRecomputesTableStructure(t *testing.T) {
	h := newHarness(t, documentTable)
	table := documentTestTable(t, h.e)
	h.e.SetKeymap("vim")
	h.e.vimController().SetMode(helix.ModeVisual)
	h.e.selectTableAxis(table, tableRows, 2, 2, false)
	h.e.dragFrom = [2]int{table.tab.rows[1][0].start, table.tab.rows[1][0].start}
	h.e.dragTo(table.x+table.geom.w+4, table.y+(table.geom.rowY[1]+table.geom.rowY[2])/2)
	if s := h.e.tableSelection; s == nil || s.first != 1 || s.last != 1 {
		t.Fatalf("drag retained a stale structural focus: %s", tableSelectionState(h.e))
	}
}
