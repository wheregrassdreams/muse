package editor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

const vimVirtualTable = "| name | value |\n| --- | ---: |\n| alpha | 1 |\n| beta | 2 |\n| gamma | 3 |"

func tableCellValues(t *testing.T, h *harness) (tables int, rows [][]string) {
	t.Helper()
	h.e.layoutIfNeeded()
	for _, node := range h.e.leaves {
		if node.kind != kTable {
			continue
		}
		tables++
		values := make([][]string, len(node.tab.rows))
		for r, row := range node.tab.rows {
			values[r] = make([]string, len(row))
			for c, cell := range row {
				values[r][c] = h.e.src[cell.start:cell.end]
			}
		}
		rows = append(rows, values...)
	}
	return tables, rows
}

func requireTableValues(t *testing.T, h *harness, want [][]string) {
	t.Helper()
	tables, got := tableCellValues(t, h)
	if tables != 1 {
		t.Fatalf("got %d tables, want one; source=%q", tables, h.e.Source())
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("table cells = %#v, want %#v; source=%q", got, want, h.e.Source())
	}
}

func TestVimVirtualTableRowPutBeforeAfterAndUndoRedo(t *testing.T) {
	for _, tc := range []struct {
		name string
		put  string
		want [][]string
	}{
		{
			name: "put after",
			put:  "p",
			want: [][]string{{"name", "value"}, {"gamma", "3"}, {"alpha", "1"}, {"beta", "2"}},
		},
		{
			name: "put before",
			put:  "P",
			want: [][]string{{"name", "value"}, {"alpha", "1"}, {"beta", "2"}, {"gamma", "3"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, vimVirtualTable)
			h.e.SetKeymap("vim")
			table := documentTestTable(t, h.e)
			h.caretAt(table.tab.rows[1][0].start)
			sendVim(h, "Vj")
			sendVim(h, "d")
			sendVim(h, tc.put)
			requireTableValues(t, h, tc.want)
			after := h.e.Source()
			if !h.e.Undo() {
				t.Fatal("Undo failed after structural row put")
			}
			if h.e.Source() != "| name | value |\n| --- | ---: |\n| gamma | 3 |" {
				t.Fatalf("first Undo did not restore the post-delete table: %q", h.e.Source())
			}
			if !h.e.Undo() || h.e.Source() != vimVirtualTable {
				t.Fatalf("second Undo did not restore original rows: %q", h.e.Source())
			}
			if !h.e.Redo() || h.e.Source() == vimVirtualTable {
				t.Fatalf("first Redo did not restore the delete state: %q", h.e.Source())
			}
			if !h.e.Redo() || h.e.Source() != after {
				t.Fatalf("Redo did not restore structural put: source=%q want=%q", h.e.Source(), after)
			}
			requireTableValues(t, h, tc.want)
		})
	}
}

func TestVimVirtualTableYankThenPutBeforeAfter(t *testing.T) {
	for _, tc := range []struct {
		name string
		put  string
		want [][]string
	}{
		{
			name: "put after",
			put:  "p",
			want: [][]string{{"name", "value"}, {"alpha", "1"}, {"beta", "2"}, {"gamma", "3"}, {"alpha", "1"}},
		},
		{
			name: "put before",
			put:  "P",
			want: [][]string{{"name", "value"}, {"alpha", "1"}, {"beta", "2"}, {"alpha", "1"}, {"gamma", "3"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, vimVirtualTable)
			h.e.SetKeymap("vim")
			table := documentTestTable(t, h.e)
			h.caretAt(table.tab.rows[1][0].start)
			sendVim(h, "Vy")
			sendVim(h, "2j")
			sendVim(h, tc.put)
			requireTableValues(t, h, tc.want)
		})
	}
}

func TestVimVirtualTablePutAfterDeletingOnlyBodyRow(t *testing.T) {
	const src = "| name | value |\n| --- | ---: |\n| only | row |"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[1][0].start)
	sendVim(h, "Vdp")
	want := [][]string{{"name", "value"}, {"only", "row"}}
	requireTableValues(t, h, want)
	after := h.e.Source()
	if !h.e.Undo() || h.e.Source() != "| name | value |\n| --- | ---: |\n" {
		t.Fatalf("first Undo did not restore the post-delete table: %q", h.e.Source())
	}
	if !h.e.Undo() || h.e.Source() != src {
		t.Fatalf("second Undo did not restore original only-body-row source: %q", h.e.Source())
	}
	if !h.e.Redo() || h.e.Source() != "| name | value |\n| --- | ---: |\n" {
		t.Fatalf("first Redo did not restore the deletion: %q", h.e.Source())
	}
	if !h.e.Redo() || h.e.Source() != after {
		t.Fatalf("Redo failed to restore the put row: %q", h.e.Source())
	}
	requireTableValues(t, h, want)
}

func TestVimVirtualTableCellPutExpandsRightAndUndoRedo(t *testing.T) {
	h := newHarness(t, vimVirtualTable)
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[1][1].start)
	if !h.e.modalKey(ui.KeyV, ui.Ctrl) {
		t.Fatal("Ctrl+v did not enter table cell region")
	}
	h.frame()
	sendVim(h, "d")
	sendVim(h, "p")
	want := [][]string{{"name", "value", ""}, {"alpha", "", "1"}, {"beta", "2", ""}, {"gamma", "3", ""}}
	requireTableValues(t, h, want)
	after := h.e.Source()
	afterPut := h.e.Source()
	if !h.e.Undo() {
		t.Fatalf("Undo failed to restore deleted column state: %q", h.e.Source())
	}
	afterDelete := h.e.Source()
	if !h.e.Undo() || h.e.Source() != vimVirtualTable {
		t.Fatalf("second Undo did not restore original columns: %q", h.e.Source())
	}
	if !h.e.Redo() || h.e.Source() != afterDelete {
		t.Fatalf("first Redo did not restore deleted column: %q", h.e.Source())
	}
	if !h.e.Redo() || h.e.Source() != afterPut || afterPut != after {
		t.Fatalf("Redo failed to restore column put: %q", h.e.Source())
	}
	requireTableValues(t, h, want)
}

func TestVimVirtualTableWholeTableDeleteThenPut(t *testing.T) {
	const src = "before\n\n| name | value |\n| --- | ---: |\n| alpha | 1 |\n| beta | 2 |\n\nafter"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[0][0].start)
	sendVim(h, "V2j") // Select the header and every body row.
	sendVim(h, "d")
	if _, tables := tableCellValues(t, h); len(tables) != 0 {
		t.Fatalf("whole-table delete left table cells behind: %#v source=%q", tables, h.e.Source())
	}
	sendVim(h, "p")
	h.e.layoutIfNeeded()
	if !strings.Contains(h.e.Source(), "before") || !strings.Contains(h.e.Source(), "after") {
		t.Fatalf("whole-table put damaged adjacent paragraphs: %q", h.e.Source())
	}
	want := [][]string{{"name", "value"}, {"alpha", "1"}, {"beta", "2"}}
	requireTableValues(t, h, want)
	after := h.e.Source()
	if !h.e.Undo() {
		t.Fatal("Undo failed after whole-table put")
	}
	if tables, _ := tableCellValues(t, h); tables != 0 {
		t.Fatalf("Undo of whole-table put restored a table too early: source=%q", h.e.Source())
	}
	if !h.e.Undo() || h.e.Source() != src {
		t.Fatalf("second Undo did not restore original whole table: %q", h.e.Source())
	}
	if !h.e.Redo() {
		t.Fatal("first Redo failed for whole-table deletion")
	}
	if tables, _ := tableCellValues(t, h); tables != 0 {
		t.Fatalf("first Redo did not remove the whole table: source=%q", h.e.Source())
	}
	if !h.e.Redo() || h.e.Source() != after {
		t.Fatalf("Redo failed to restore whole table: %q", h.e.Source())
	}
	requireTableValues(t, h, want)
}
