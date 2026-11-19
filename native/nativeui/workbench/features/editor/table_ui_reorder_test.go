package editor

import (
	"fmt"
	helix "github.com/wheregrassdreams/muse-editor/text/helix"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	syntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"
)

func TestEveryColumnHandleKeepsPointerCaptureAcrossPressedFrames(t *testing.T) {
	const target = "| A | B | C |\n| :--- | :---: | ---: |\n| one | two | three |"
	for _, precedingTable := range []bool{false, true} {
		for col := 0; col < 3; col++ {
			t.Run(fmt.Sprintf("preceding-table=%v/column=%d", precedingTable, col+1), func(t *testing.T) {
				src := target + "\n\nafter"
				if precedingTable {
					src = "| Before |\n| --- |\n| keep |\n\n" + src
				}
				h := newHarness(t, src)
				var table *leaf
				for _, l := range h.e.leaves {
					if l.kind == kTable {
						table = l
					}
				}
				x := table.x + (table.geom.colX[col]+table.geom.colX[col+1])/2
				y := table.y - 11
				h.tt.Move(x, y)
				h.frame()
				h.tt.Press(x, y)
				for frame := 0; frame < 4; frame++ {
					h.frame()
					if d := h.e.tableHandleDrag; d == nil || d.axis != tableColumns || d.index != col {
						t.Fatalf("column handle lost capture during pressed frame %d: %+v", frame, d)
					}
				}
				dropX, want := table.x-30, 0
				if col == 0 {
					dropX, want = table.x+table.geom.w+30, 2
				}
				h.tt.Move(dropX, y)
				h.frame()
				if d := h.e.tableHandleDrag; d == nil || !d.moving || h.e.Source() != src {
					t.Fatal("pressed column flashed away instead of starting a drag")
				}
				h.tt.Release(dropX, y)
				h.frame()
				table = h.e.tableSelectionLeaf()
				if table == nil || h.e.tableSelection.first != want || h.e.tableSelection.last != want || h.e.Source()[table.tab.rows[0][want].start:table.tab.rows[0][want].end] != string(rune('A'+col)) {
					t.Fatalf("column did not move after sustained press: %s", tableSelectionState(h.e))
				}
			})
		}
	}
}

func TestTableReorderPreservesContentAlignmentAndHistory(t *testing.T) {
	const tableSource = "| H1 | H2 | H3 |\n| :--- | :---: | ---: |\n| a\\|b | **乙** | 3 |\n| long text | `code` | 42 |\n| [[双链]] | $x^2$ | 注释 |"
	for _, axis := range []tableAxis{tableRows, tableColumns} {
		h := newHarness(t, "before\n\n"+tableSource+"\n\nafter")
		e := h.e
		table := documentTestTable(t, e)
		e.SetKeymap("vim")
		e.vimController().SetMode(helix.ModeVisual)
		first, gap := 1, 4
		if axis == tableColumns {
			first, gap = 0, 3
			e.vimController().SetMode(helix.ModeBlockVisual)
		}
		e.selectTableAxis(table, axis, first, first, false)
		e.tableSelection.activeRow = 1
		before := snapshotSelection(e, table)
		original := e.Source()
		if !e.reorderTable(table.lineStart, axis, first, first, gap) {
			t.Fatal("native table reorder was rejected")
		}
		h.frame()
		table = documentTestTable(t, e)
		moved := e.Source()
		if !strings.HasPrefix(moved, "before\n\n") || !strings.HasSuffix(moved, "\n\nafter") {
			t.Fatal("reorder changed text outside the table")
		}
		if axis == tableRows {
			if got := e.Source()[table.tab.rows[3][0].start:table.tab.rows[3][0].end]; got != "a\\|b" || e.tableSelection.first != 3 || e.tableSelection.last != 3 {
				t.Fatalf("row drag extended selection or damaged escaped content: %s", tableSelectionState(e))
			}
		} else {
			if got := e.Source()[table.tab.rows[0][0].start:table.tab.rows[0][0].end]; got != "H2" || table.tab.align[0] != syntax.IncrementalAlignCenter || table.tab.align[1] != syntax.IncrementalAlignRight || table.tab.align[2] != syntax.IncrementalAlignLeft || e.tableSelection.first != 2 || e.tableSelection.last != 2 {
				t.Fatalf("columns did not carry their alignment/content: %q %v", moved, table.tab.align)
			}
		}
		after := snapshotSelection(e, table)
		if !e.Undo() || e.Source() != original {
			t.Fatal("table move was not one undo step")
		}
		h.frame()
		if got := snapshotSelection(e, documentTestTable(t, e)); got != before {
			t.Fatalf("undo did not restore the original structural focus:\ngot=%+v\nwant=%+v", got, before)
		}
		if !e.Redo() || e.Source() != moved {
			t.Fatal("table move redo failed")
		}
		h.frame()
		if got := snapshotSelection(e, documentTestTable(t, e)); got != after {
			t.Fatalf("redo lost moved structural focus:\ngot=%+v\nwant=%+v", got, after)
		}
	}
}

func TestTableHandleDragMovesInsteadOfExtendingAndCanCancel(t *testing.T) {
	const src = "| H | V |\n| --- | --- |\n| one | A |\n| two | B |\n| three | C |\n\nafter"
	h := newHarness(t, src)
	table := documentTestTable(t, h.e)
	r, ok := h.tt.Find("选择第 2 行")
	if !ok {
		t.Fatal("row handle missing")
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	probeX, probeY := int(table.x+table.geom.colW[0]/2), int(table.y)
	beforeTop := h.tt.Image().RGBAAt(probeX, probeY)
	h.tt.Press(x, y)
	h.frame()
	if d := h.e.tableHandleDrag; d == nil || d.axis != tableRows || d.first != 1 || d.last != 1 {
		t.Fatal("press did not create an independent row handle gesture")
	}
	if h.tt.Image().RGBAAt(probeX, probeY) != beforeTop {
		t.Fatal("pressing a row handle activated an unrelated column hover marker")
	}
	h.shot("table-row-handle-pressed")
	dropY := table.y + table.h + 100
	h.tt.Move(x, dropY)
	h.frame()
	if d := h.e.tableHandleDrag; d == nil || !d.moving || d.gap != 4 || d.position > table.y+table.h || h.e.tableSelection.first != 1 || h.e.tableSelection.last != 1 || h.e.Source() != src {
		t.Fatalf("drag extended/edited instead of previewing a clamped move: %+v %s", d, tableSelectionState(h.e))
	}
	h.shot("table-row-reorder-preview")
	h.tt.Release(x, dropY)
	h.frame()
	table = documentTestTable(t, h.e)
	if h.e.tableHandleDrag != nil || h.e.tableSelection.first != 3 || h.e.tableSelection.last != 3 || h.e.Source()[table.tab.rows[3][0].start:table.tab.rows[3][0].end] != "one" {
		t.Fatalf("release did not move the selected row: %s", tableSelectionState(h.e))
	}
	if !h.e.Undo() || h.e.Source() != src {
		t.Fatal("pointer move did not undo in one step")
	}
	h.frame()
	for _, cancel := range []string{"escape", "pointercancel"} {
		r, _ = h.tt.Find("选择第 2 行")
		x, y = r.X+r.W/2, r.Y+r.H/2
		h.tt.Press(x, y)
		h.frame()
		h.tt.Move(x, dropY)
		h.frame()
		if cancel == "escape" {
			h.key(0, ui.KeyEscape)
			h.tt.Release(x, dropY)
		} else {
			h.tt.CancelPress()
		}
		h.frame()
		if h.e.tableHandleDrag != nil || h.e.Source() != src || h.e.Session.CanUndo() {
			t.Fatalf("%s committed a canceled reorder", cancel)
		}
	}
}

func TestKeyboardStructureSelectionDoesNotShowMouseHandles(t *testing.T) {
	const src = "| A | B |\n| --- | --- |\n| one | two |\n| three | four |\n\nafter"
	h := newHarness(t, src)
	table := documentTestTable(t, h.e)
	h.tt.Move(950, 550)
	h.frame()
	bg := h.tt.Image().RGBAAt(int(table.x-9), int(table.y+table.h/2))
	h.e.SetKeymap("vim")
	h.e.vimController().SetMode(helix.ModeVisual)
	h.e.selectTableAxis(table, tableRows, 0, 2, false)
	h.frame()
	for row := 0; row < len(table.tab.rows); row++ {
		y := int(table.y + (table.geom.rowY[row]+table.geom.rowY[row+1])/2)
		if got := h.tt.Image().RGBAAt(int(table.x-9), y); got != bg {
			t.Fatalf("keyboard-selected row %d rendered a mouse handle without hover", row)
		}
	}
	h.shot("table-visual-rows-no-hover-handles")
}

func TestColumnHandleDragPreviewsAndMovesItsAlignment(t *testing.T) {
	const src = "| Left | Center | Right |\n| :--- | :---: | ---: |\n| a | b | c |\n\nafter"
	h := newHarness(t, src)
	table := documentTestTable(t, h.e)
	r, ok := h.tt.Find("选择第 1 列")
	if !ok {
		t.Fatal("column handle missing")
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	h.tt.Press(x, y)
	h.frame()
	h.tt.Move(table.x+table.geom.w+100, y)
	h.frame()
	if d := h.e.tableHandleDrag; d == nil || d.axis != tableColumns || d.gap != 3 || d.position > table.x+table.geom.w || h.e.tableSelection.first != 0 || h.e.tableSelection.last != 0 || h.e.Source() != src {
		t.Fatalf("column drag resized the range or left its rail: %+v", d)
	}
	h.shot("table-column-reorder-preview")
	h.tt.Release(table.x+table.geom.w+100, y)
	h.frame()
	table = documentTestTable(t, h.e)
	if table.tab.align[2] != syntax.IncrementalAlignLeft || h.e.Source()[table.tab.rows[0][2].start:table.tab.rows[0][2].end] != "Left" || h.e.tableSelection.first != 2 || h.e.tableSelection.last != 2 {
		t.Fatal("pointer column reorder did not move its values/alignment together")
	}
}

func TestTableReorderGroupsAndContainersKeepValidHeader(t *testing.T) {
	for _, prefix := range []string{"", "> ", "- "} {
		cont := blankMarkers(prefix)
		src := prefix + "| H | V |\n" + cont + "| :--- | ---: |\n" + cont + "| one | A |\n" + cont + "| two | B |\n" + cont + "| three | C |\n"
		h := newHarness(t, src)
		table := documentTestTable(t, h.e)
		h.e.selectTableAxis(table, tableRows, 1, 2, false)
		beforeUndo := h.e.Session.CanUndo()
		for _, gap := range []int{1, 2, 3} {
			if h.e.reorderTable(table.lineStart, tableRows, 1, 2, gap) || h.e.Source() != src || h.e.Session.CanUndo() != beforeUndo {
				t.Fatalf("drop inside the selected group must be a no-op: prefix=%q gap=%d", prefix, gap)
			}
		}
		if !h.e.reorderTable(table.lineStart, tableRows, 1, 2, 0) {
			t.Fatal("group move to the header position failed")
		}
		h.frame()
		table = documentTestTable(t, h.e)
		if len(table.tab.rows) != 4 || table.tab.cols() != 2 || h.e.Source()[table.tab.rows[0][0].start:table.tab.rows[0][0].end] != "one" || h.e.Source()[table.tab.rows[1][0].start:table.tab.rows[1][0].end] != "two" || !strings.HasPrefix(h.e.Source(), prefix+"|") {
			t.Fatalf("moving a group broke its GFM header or container: %q", h.e.Source())
		}
		if !h.e.Undo() || h.e.Source() != src {
			t.Fatalf("group/container move did not restore exact original bytes: prefix=%q", prefix)
		}
	}
}

func TestTableBoundaryTextDragStaysSeparateFromHandleReorder(t *testing.T) {
	const src = "before\n\n| A | B |\n| --- | --- |\n| one | two |\n| three | four |\n\nafter"
	h := newHarness(t, src)
	table := documentTestTable(t, h.e)
	x := table.x - 1
	y := table.y + (table.geom.rowY[1]+table.geom.rowY[2])/2
	h.tt.Move(x, y)
	h.frame()
	h.tt.Press(x, y)
	h.frame()
	if h.e.tableHandleDrag != nil || !h.e.dragging {
		t.Fatal("the border caret strip was intercepted by the separate handle gesture")
	}
	x = table.x + table.geom.w + 1
	y = table.y + (table.geom.rowY[2]+table.geom.rowY[3])/2
	h.tt.Move(x, y)
	h.frame()
	h.tt.Release(x, y)
	h.frame()
	if h.e.Source() != src || h.e.tableHandleDrag != nil || h.e.dragging {
		t.Fatal("a native structure-selection drag reordered the table")
	}
	if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 1 || s.last != 2 {
		t.Fatalf("border drag did not extend native structural rows: %s", tableSelectionState(h.e))
	}
	x = table.x + table.geom.w + 1
	y = table.y + (table.geom.rowY[1]+table.geom.rowY[2])/2
	h.tt.Press(x, y)
	h.frame()
	if !h.e.dragging || h.e.tableHandleDrag != nil || h.e.Source() != src {
		t.Fatal("right boundary caret was intercepted by the add-column capsule")
	}
	h.tt.Release(x, y)
	h.frame()
	if h.e.Source() != src {
		t.Fatal("clicking the right boundary inserted a column")
	}
}
