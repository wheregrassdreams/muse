package mdedit

import (
	"fmt"
	"testing"
)

// These cases go through Tester so the pointer is transformed by the real UI
// scroll and element coordinate path before the editor receives it.
func TestPointerTableCellDragFromInlineContentUsesRealHarnessDispatch(t *testing.T) {
	cases := []struct {
		name, content, wantLink string
	}{
		{name: "plain text", content: "plain"},
		{name: "wiki link", content: "[[Target]]", wantLink: "wiki:Target"},
		{name: "inline code", content: "`code`"},
		{name: "markdown link", content: "[link](https://example.test)", wantLink: "https://example.test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "Before.\n\n| A | B | C |\n| --- | --- | --- |\n| " + tc.content + " | target | third |"
			h := newHarness(t, src)
			h.e.layoutIfNeeded()
			table := documentTestTable(t, h.e)
			cell := table.tab.rows[1][0]
			startX, startY, ok := pointerOnCellContent(h.e, table, cell, tc.wantLink)
			if !ok {
				t.Fatalf("could not find display point for cell content %q (linkAt=%q)", tc.content, tc.wantLink)
			}
			row, col, inside := tableCellIndexAt(table, startX, startY)
			if !inside || row != 1 || col != 0 {
				t.Fatalf("source point (%v,%v) hit (%d,%d,%v), want row 1 col 0", startX, startY, row, col, inside)
			}
			if tc.wantLink != "" && h.e.linkAt(h.e.hitTest(startX, startY)) != tc.wantLink {
				t.Fatalf("source point did not hit link: got %q want %q", h.e.linkAt(h.e.hitTest(startX, startY)), tc.wantLink)
			}

			// Tester takes viewport coordinates; the table is below the lead-in
			// paragraph, so map its document point back to the viewport.
			screenX, screenY := startX, startY-h.e.scroll.Y
			h.tt.Press(screenX, screenY)
			captureAfterDown := snapshotPointerTableDrag(h.e)
			t.Logf("down: %s", captureAfterDown)
			if h.e.tableCellDrag == nil || h.e.tableCellDrag.crossed || h.e.tableSelection != nil {
				t.Fatalf("pointer down failed to capture a cell drag: %s", captureAfterDown)
			}

			// A move inside the same cell must remain an ordinary text drag.
			h.tt.Move(screenX+1, screenY)
			t.Logf("same-cell move: %s", snapshotPointerTableDrag(h.e))
			if h.e.tableCellDrag == nil || h.e.tableCellDrag.crossed || h.e.tableSelection != nil {
				t.Fatalf("same-cell move changed structural state: %s", snapshotPointerTableDrag(h.e))
			}

			target := table.tab.rows[1][2]
			targetX := table.x + (table.geom.colX[2]+table.geom.colX[3])/2
			targetY := table.y + (table.geom.rowY[1]+table.geom.rowY[2])/2
			h.tt.Move(targetX, targetY-h.e.scroll.Y)
			t.Logf("cross-cell move: %s", snapshotPointerTableDrag(h.e))
			if s := h.e.tableSelection; s == nil || !s.cellRegion || s.anchorRow != 1 || s.anchorCol != 0 || s.focusRow != 1 || s.focusCol != 2 {
				t.Fatalf("cross-cell move failed to select the rectangle: state=%s selection=%+v target=%+v", snapshotPointerTableDrag(h.e), s, target)
			}
			if h.e.tableCellDrag == nil || !h.e.tableCellDrag.crossed {
				t.Fatalf("crossed capture did not stay latched: %s", snapshotPointerTableDrag(h.e))
			}

			h.tt.Release(targetX, targetY-h.e.scroll.Y)
			t.Logf("release: %s", snapshotPointerTableDrag(h.e))
			if h.e.tableCellDrag != nil {
				t.Fatalf("pointer release retained transient capture: %s", snapshotPointerTableDrag(h.e))
			}
			if s := h.e.tableSelection; s == nil || !s.cellRegion || s.focusCol != 2 {
				t.Fatalf("pointer release discarded the cell region: %+v", s)
			}
		})
	}
}

func TestPointerTableSourceChipPressDoesNotStartCellDrag(t *testing.T) {
	h := newHarness(t, "Before.\n\n| A | B |\n| --- | --- |\n| one | two |")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	chip := h.e.sourceChip(table)
	cellX := table.x + (table.geom.colX[0]+table.geom.colX[1])/2
	cellY := table.y + (table.geom.rowY[1]+table.geom.rowY[2])/2 - h.e.scroll.Y
	h.tt.Move(cellX, cellY)
	h.frame() // reveal the table's source chip
	h.tt.Move(chip.X+chip.W/2, chip.Y+chip.H/2-h.e.scroll.Y)
	h.frame()
	h.tt.Press(chip.X+chip.W/2, chip.Y+chip.H/2-h.e.scroll.Y)
	t.Logf("source-chip down: %s", snapshotPointerTableDrag(h.e))
	if h.e.tableCellDrag != nil || h.e.dragging {
		t.Fatalf("source chip press entered cell drag: %s", snapshotPointerTableDrag(h.e))
	}
	var source *leaf
	for _, candidate := range h.e.leaves {
		if candidate.lineStart == table.lineStart {
			source = candidate
			break
		}
	}
	if source == nil || source.kind != kRaw {
		t.Fatalf("source chip did not show table source: leaf=%+v", source)
	}
	h.tt.Release(chip.X+chip.W/2, chip.Y+chip.H/2-h.e.scroll.Y)
	t.Logf("source-chip release: %s", snapshotPointerTableDrag(h.e))
}

func pointerOnCellContent(e *Editor, table, cell *leaf, wantLink string) (float32, float32, bool) {
	// Brute force display pixels instead of guessing source character widths;
	// this correctly handles hidden Markdown delimiters and shortened aliases.
	for y := cell.y + 2; y < cell.y+cell.h-1; y++ {
		for x := cell.x + 2; x < cell.x+cell.w-2; x++ {
			row, col, inside := tableCellIndexAt(table, x, y)
			if !inside || row != 1 || col != 0 {
				continue
			}
			off := e.hitTest(x, y)
			if off < cell.start || off > cell.end {
				continue
			}
			if wantLink != "" && e.linkAt(off) != wantLink {
				continue
			}
			return x, y, true
		}
	}
	return 0, 0, false
}

func snapshotPointerTableDrag(e *Editor) string {
	capture := "nil"
	if d := e.tableCellDrag; d != nil {
		capture = fmt.Sprintf("line=%d anchor=(%d,%d) crossed=%v", d.lineStart, d.anchorRow, d.anchorCol, d.crossed)
	}
	selection := "nil"
	if s := e.tableSelection; s != nil {
		selection = fmt.Sprintf("cellRegion=%v axis=%d anchor=(%d,%d) focus=(%d,%d)", s.cellRegion, s.axis, s.anchorRow, s.anchorCol, s.focusRow, s.focusCol)
	}
	return fmt.Sprintf("capture={%s} dragging=%v selection={%s} bridge=%+v text=[%d,%d)", capture, e.dragging, selection, e.tableBridge, e.anchor, e.caret)
}
