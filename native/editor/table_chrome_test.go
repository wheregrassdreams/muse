package editor

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
	syntax "muse/markdown/syntax"
)

func TestCellAlignmentMenuTargetsDisplayedColumnAndKeepsTextSelection(t *testing.T) {
	const src = "| A | B | C |\n| --- | --- | --- |\n| one | two | three |\n\noutside selection"
	h := newHarness(t, src)
	e := h.e
	a := strings.Index(src, "outside")
	e.Select(a, a+len("outside"))
	h.frame()
	beforeScroll := e.scroll.Y
	if err := h.tt.Click(locales.EditorTableCellAlignment(2, 2)); err != nil {
		t.Fatal(err)
	}
	h.frame()
	if err := h.tt.ChooseMenuItem("居中（此列）"); err != nil {
		t.Fatal(err)
	}
	h.frame()
	table := documentTestTable(t, e)
	if table.tab.align[1] != syntax.IncrementalAlignCenter || table.tab.align[0] != syntax.IncrementalAlignNone || table.tab.align[2] != syntax.IncrementalAlignNone {
		t.Fatalf("cell menu aligned the caret's table/column instead of its target: %v", table.tab.align)
	}
	if e.SelectedText() != "outside" || e.scroll.Y != beforeScroll {
		t.Fatalf("cell alignment moved text selection or viewport: %q scroll=%v", e.SelectedText(), e.scroll.Y)
	}
	if !e.Undo() || e.Source() != src || e.SelectedText() != "outside" {
		t.Fatal("alignment did not undo as one source edit with the original selection")
	}
}

func TestColumnAlignmentPreservesForeignTableStructureThroughUndoRedo(t *testing.T) {
	const first = "| A | B |\n| ----- | ----- |\n| one | two |"
	const second = "| C | D |\n| --- | --- |\n| three | four |"
	const src = first + "\n\n" + second
	h := newHarness(t, src)
	e := h.e
	e.SetKeymap("vim")
	var tables []*leaf
	for _, l := range e.leaves {
		if l.kind == kTable {
			tables = append(tables, l)
		}
	}
	if len(tables) != 2 {
		t.Fatal("fixture must contain two tables")
	}
	e.modal.mode = "BlockVisual"
	e.selectTableAxis(tables[1], tableColumns, 1, 1, false)
	e.tableSelection.activeRow = 1
	selected := e.SelectedText()
	e.tableColumnAlignment(tables[0].lineStart, 0, "alignRight")
	updated := e.Source()
	check := func() {
		t.Helper()
		if s := e.tableSelection; s == nil || s.lineStart != strings.Index(e.Source(), "| C |") || s.axis != tableColumns || s.first != 1 || s.last != 1 || s.activeRow != 1 || e.SelectedText() != selected || e.modal.mode != "BlockVisual" {
			t.Fatalf("formatting lost the foreign table's structural focus: %s", tableSelectionState(e))
		}
	}
	check()
	if !e.Undo() || e.Source() != src {
		t.Fatal("column alignment undo failed")
	}
	check()
	if !e.Redo() || e.Source() != updated {
		t.Fatal("column alignment redo failed")
	}
	check()
}

func TestTableChromeHoverAndHandlesKeepNativeSelection(t *testing.T) {
	const src = "|   |   |   |\n| --- | --- | --- |\n|   |   |   |\n|   |   |   |\n|   |   |   |\n\nafter"
	for _, dark := range []bool{false, true} {
		name, theme := "light", ui.LightTheme()
		if dark {
			name, theme = "dark", ui.DarkTheme()
		}
		t.Run(name, func(t *testing.T) {
			h := &harness{t: t, e: New(src, Options{FontSize: 16})}
			h.tt = ui.NewTester(func(c *ui.Context) {
				ui.Themed(c, theme).Fill().Background(theme.Background).Children(func() { h.e.View(c).Grow(1) })
			}, 1000, 600)
			h.frame()
			h.focus()
			h.e.Select(len(src), len(src))
			h.frame()
			table := documentTestTable(t, h.e)
			h.shot("table-idle-" + name)
			beforeCaret := h.e.caret
			h.tt.Move(table.x+table.geom.w-2, table.y+table.h/2)
			h.frame()
			h.shot("table-add-column-capsule-" + name)
			if h.e.caret != beforeCaret || h.e.Source() != src {
				t.Fatal("table control hover changed the caret or Markdown")
			}
			h.tt.Move(table.x+table.geom.w/2, table.y+table.h+8)
			h.frame()
			h.shot("table-add-row-capsule-" + name)
			if err := h.tt.Click("选择第 2 行"); err != nil {
				t.Fatal(err)
			}
			h.frame()
			if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 1 || s.last != 1 {
				t.Fatalf("border handle did not create a native row selection: %s", tableSelectionState(h.e))
			}
			h.shot("table-selected-row-" + name)
			h.e.selectTableAxis(table, tableColumns, 1, 1, false)
			h.frame()
			h.shot("table-selected-column-" + name)
			if h.e.Source() != src {
				t.Fatal("row/column chrome wrote selection state into Markdown")
			}
		})
	}
}

func TestWrappedCellHoverUsesContainingRowAndOnlyExposedEdges(t *testing.T) {
	src := "above\n\n| Left | Middle | Right |\n| --- | --- | --- |\n| " + strings.Repeat("一行比较长的内容，用来观察换行。", 8) + " | code | 42 |\n| last | math | end |\n\n" + strings.Repeat("trailing paragraph\n\n", 24)
	h := newHarness(t, src)
	opts := h.e.Options()
	opts.MaxWidth = 420
	h.e.SetOptions(opts)
	h.frame()
	table := documentTestTable(t, h.e)
	g := table.geom
	if len(table.tab.rows[1][0].view.layout.Lines()) < 3 {
		t.Fatalf("fixture must exercise a tall, wrapped body row: rows=%v cols=%v width=%v", g.rowY, g.colW, table.w)
	}
	h.e.scroll.Y = table.y + g.rowY[1] - 100
	h.e.wantReveal = false
	h.frame()
	h.tt.Move(950, 550)
	h.frame()
	rowCenter := table.y + (g.rowY[1]+g.rowY[2])/2 - h.e.scroll.Y
	nextCenter := table.y + (g.rowY[2]+g.rowY[3])/2 - h.e.scroll.Y
	x := int(table.x)
	baseRow := h.tt.Image().RGBAAt(x, int(rowCenter))
	baseNext := h.tt.Image().RGBAAt(x, int(nextCenter))
	for _, tc := range []struct {
		name             string
		row, col         int
		yFraction        float32
		wantRow, wantCol int
	}{
		{"wrapped first column lower half", 1, 0, .85, 1, -1},
		{"first row middle column", 0, 1, .5, -1, 1},
		{"top left corner", 0, 0, .5, 0, 0},
		{"interior wrapped cell", 1, 1, .85, -1, -1},
	} {
		// A wide table may expose only part of this column. Hover a real
		// visible fragment, rather than a point outside the block viewport.
		left := max(table.x, tableContentX(table)+g.colX[tc.col])
		right := min(table.x+tableViewportWidth(table), tableContentX(table)+g.colX[tc.col+1])
		if right <= left {
			t.Fatalf("%s: fixture column is completely outside the viewport", tc.name)
		}
		px := (left + right) / 2
		py := table.y + g.rowY[tc.row] + (g.rowY[tc.row+1]-g.rowY[tc.row])*tc.yFraction
		h.tt.Move(px, py-h.e.scroll.Y)
		h.frame()
		row, col := tableHoverAxes(table, h.e.pointerX, h.e.pointerY)
		if row != tc.wantRow || col != tc.wantCol {
			t.Fatalf("%s: hover axes=(%d,%d), want=(%d,%d), pointer=(%v,%v)", tc.name, row, col, tc.wantRow, tc.wantCol, h.e.pointerX, h.e.pointerY)
		}
		if tc.row == 1 && tc.col == 0 {
			if h.tt.Image().RGBAAt(x, int(rowCenter)) == baseRow || h.tt.Image().RGBAAt(x, int(nextCenter)) != baseNext {
				t.Fatal("wrapped-row hover marker was missing or painted on the next row")
			}
			h.shot("table-wrapped-first-column-hover")
		}
		if tc.row == 1 && tc.col == 1 {
			if h.tt.Image().RGBAAt(x, int(rowCenter)) != baseRow {
				t.Fatal("interior cell retained the left-edge hover marker")
			}
			h.shot("table-interior-cell-no-edge-hover")
		}
	}
	// The surface scrolls beneath a stationary pointer. No new mouse move
	// should be required to move the indicator to the newly hovered row.
	px := table.x + g.colW[0]/2
	py := table.y + g.rowY[1] + (g.rowY[2]-g.rowY[1])*.85
	h.tt.Move(px, py-h.e.scroll.Y)
	h.frame()
	delta := (g.rowY[2]-g.rowY[1])*.15 + (g.rowY[3]-g.rowY[2])*.3
	h.e.scroll.Y += delta
	h.frame()
	h.frame()
	newNextCenter := table.y + (g.rowY[2]+g.rowY[3])/2 - h.e.scroll.Y
	if h.tt.Image().RGBAAt(x, int(newNextCenter)) == baseNext {
		t.Fatal("stationary pointer retained a stale hover row after scrolling")
	}
	h.shot("table-stationary-pointer-after-scroll")
	h.tt.Move(1100, 500)
	h.frame()
	if h.tt.Image().RGBAAt(x, int(newNextCenter)) != baseNext {
		t.Fatal("table hover indicator remained after the pointer left the editor")
	}
}
