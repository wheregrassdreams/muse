package editor

import (
	"muse/editor/input/vim"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Pointer placement is important here: source offsets chosen directly in a
// unit test can hide a mismatch between rendered header geometry and parser
// row coordinates.
func TestPointerPlacedHeaderCaretVisualRowsIncludeHeader(t *testing.T) {
	src := "| Local file | Source page | Creator / source | License |\n| --- | --- | --- | --- |\n| attachment | source | creator | license |\n| next | next | next | next |"
	for _, mode := range []string{"Vj", "CtrlVj"} {
		t.Run(mode, func(t *testing.T) {
			h := newHarness(t, src)
			h.e.SetKeymap("vim")
			h.e.layoutIfNeeded()
			table := documentTestTable(t, h.e)
			cell := table.tab.rows[0][1]
			off := strings.Index(src, "Source") + 2
			caret, ok := h.e.caretRect(off)
			if !ok || off < cell.start || off > cell.end {
				t.Fatalf("could not resolve caret geometry over Source: rect=%+v ok=%v off=%d cell=[%d,%d)", caret, ok, off, cell.start, cell.end)
			}
			x := caret.X
			y := caret.Y + caret.H/2 - h.e.scroll.Y
			h.tt.Move(x, y)
			h.tt.Press(x, y)
			h.tt.Release(x, y)
			_, row, col := cellAt(table, h.e.Caret())
			if row != 0 || col != 1 {
				t.Fatalf("pointer caret landed at row=%d col=%d offset=%d, want header cell (0,1)", row, col, h.e.Caret())
			}
			h.e.vimController().SetMode(vim.ModeNormal)
			if mode == "Vj" {
				sendVim(h, "Vj")
				s := h.e.tableSelection
				if s == nil || s.axis != tableRows || s.first != 0 || s.last != 1 {
					t.Fatalf("pointer-placed header Vj omitted header row: %+v", s)
				}
			} else {
				h.e.modalKey(ui.KeyV, ui.Ctrl)
				sendVim(h, "j")
				s := h.e.tableSelection
				if s == nil || !s.cellRegion || s.anchorRow != 0 || s.focusRow != 1 || s.anchorCol != 1 || s.focusCol != 1 {
					t.Fatalf("pointer-placed header Ctrl+Vj omitted header cell: %+v", s)
				}
			}
		})
	}
}

func TestPointerDragFromHeaderIntoBodyIncludesHeaderRow(t *testing.T) {
	h := newHarness(t, "| Local file | Source page | Creator / source | License |\n| --- | --- | --- | --- |\n| attachment | source | creator | license |\n| next | next | next | next |")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	point := func(row, col int) (float32, float32) {
		return table.x + (table.geom.colX[col]+table.geom.colX[col+1])/2,
			table.y + (table.geom.rowY[row]+table.geom.rowY[row+1])/2 - h.e.scroll.Y
	}
	x, y := point(0, 1)
	h.tt.Press(x, y)
	tx, ty := point(1, 1)
	h.tt.Move(tx, ty)
	s := h.e.tableSelection
	if s == nil || !s.cellRegion || s.anchorRow != 0 || s.focusRow != 1 || s.anchorCol != 1 || s.focusCol != 1 {
		t.Fatalf("drag from header into body omitted header cell: %+v", s)
	}
	h.tt.Release(tx, ty)
}

func TestWrappedTableBodyVimSelectionKeepsHeaderAnchor(t *testing.T) {
	src := "| Local file | Source page | Creator / source | License |\n| --- | --- | --- | --- |\n| `Attachments/landscape.webp` | [File:Webp (1).webp](https://commons.wikimedia.org/wiki/File:Webp_(1).webp) | Churchill Machine Tool Co. Ltd.; source: picryl.com | [CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/) |\n| another file | [File:A](https://example.test) | Public-domain artist | Public domain |"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[0][1].start + 2)
	sendVim(h, "Vj")
	if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 0 || s.last != 1 {
		t.Fatalf("wrapped body Vj from header omitted header row: %+v", s)
	}
	h.e.tableSelection = nil
	h.e.vimController().SetMode(vim.ModeNormal)
	h.caretAt(table.tab.rows[0][1].start + 2)
	h.e.modalKey(ui.KeyV, ui.Ctrl)
	sendVim(h, "j")
	if s := h.e.tableSelection; s == nil || s.anchorRow != 0 || s.focusRow != 1 {
		t.Fatalf("wrapped body Ctrl+Vj from header omitted header row: %+v", s)
	}
}

func TestVimTextVisualHeaderPromotesDownAndReturns(t *testing.T) {
	h := newHarness(t, "| A | Source | C |\n| --- | --- | --- |\n| 1 | two | 3 |\n| 4 | five | 6 |")
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[0][1].start + 2)
	sendVim(h, "v")
	sendVim(h, "j")
	s := h.e.tableSelection
	if s == nil || !s.cellRegion || s.anchorRow != 0 || s.focusRow != 1 || s.anchorCol != 1 || s.focusCol != 1 {
		t.Fatalf("visual text selection from header did not promote with row 0 anchor: %+v", s)
	}
	sendVim(h, "k")
	if s := h.e.tableSelection; s == nil || !s.cellRegion || s.anchorRow != 0 || s.focusRow != 0 {
		t.Fatalf("returning to header lost the promoted anchor: %+v", s)
	}
}

func TestVimVisualBodyRowExtendsUpToHeader(t *testing.T) {
	h := newHarness(t, "| A | Source | C |\n| --- | --- | --- |\n| 1 | two | 3 |\n| 4 | five | 6 |")
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[1][1].start + 1)
	sendVim(h, "V")
	sendVim(h, "k")
	if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 0 || s.last != 1 || s.anchor != 1 || s.active != 0 {
		t.Fatalf("Vk from first body row did not include the header: %+v", s)
	}
}

func TestRealKeyboardVJFromPointerPlacedHeaderIncludesHeader(t *testing.T) {
	h := newHarness(t, "| Local file | Source page | Creator / source | License |\n| --- | --- | --- | --- |\n| attachment | source | creator | license |\n| next | next | next | next |")
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	off := strings.Index(h.e.Source(), "Source") + 2
	caret, ok := h.e.caretRect(off)
	if !ok {
		t.Fatal("header caret has no geometry")
	}
	x, y := caret.X, caret.Y+caret.H/2-h.e.scroll.Y
	h.tt.Press(x, y)
	h.tt.Release(x, y)
	h.key(ui.Shift, ui.KeyV)
	h.typ("V")
	h.key(0, ui.KeyJ)
	h.typ("j")
	if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 0 || s.last != 1 {
		t.Fatalf("real Shift+V, j omitted the header row: %+v", s)
	}
}

func TestHeaderVimRowSelectionThemeShots(t *testing.T) {
	src := "| Local file | Source page | Creator / source | License |\n| --- | --- | --- | --- |\n| `Attachments/landscape.webp` | [File:Webp (1).webp](https://commons.wikimedia.org/wiki/File:Webp_(1).webp) | Churchill Machine Tool Co. Ltd.; source: picryl.com | [CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/) |\n| next | [File:A](https://example.test) | Public-domain artist | Public domain |"
	for _, dark := range []bool{false, true} {
		name, theme := "light", ui.LightTheme()
		if dark {
			name, theme = "dark", ui.DarkTheme()
		}
		t.Run(name, func(t *testing.T) {
			h := &harness{t: t, e: New(src, Options{FontSize: 18, MaxWidth: 820, PadX: 32})}
			h.e.SetKeymap("vim")
			h.tt = ui.NewTester(func(c *ui.Context) {
				ui.Themed(c, theme).Fill().Background(theme.Background).Children(func() { h.e.View(c).Grow(1) })
			}, 1040, 520)
			h.frame()
			off := strings.Index(src, "Source") + 2
			caret, ok := h.e.caretRect(off)
			if !ok {
				t.Fatal("header caret geometry unavailable")
			}
			x, y := caret.X, caret.Y+caret.H/2-h.e.scroll.Y
			h.tt.Press(x, y)
			h.tt.Release(x, y)
			_, pointerRow, pointerCol := cellAt(documentTestTable(t, h.e), h.e.Caret())
			t.Logf("after pointer: caret=%d cell=(%d,%d) mode=%s selection=%+v", h.e.Caret(), pointerRow, pointerCol, h.e.vimController().Mode(), h.e.tableSelection)
			h.key(ui.Shift, ui.KeyV)
			t.Logf("after Shift+V keydown: caret=%d mode=%s line=%v selection=%+v", h.e.Caret(), h.e.vimController().Mode(), h.e.vimController().View().Linewise, h.e.tableSelection)
			h.typ("V")
			t.Logf("after V text: caret=%d mode=%s line=%v selection=%+v", h.e.Caret(), h.e.vimController().Mode(), h.e.vimController().View().Linewise, h.e.tableSelection)
			h.key(0, ui.KeyJ)
			h.typ("j")
			s := h.e.tableSelection
			if s == nil || s.axis != tableRows || s.first != 0 || s.last != 1 {
				t.Fatalf("Vj from header did not select rows 0-1: %+v", s)
			}
			h.shot("header-row-vj-" + name)
			h.key(0, ui.KeyK)
			h.typ("k")
			if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 0 || s.last != 0 {
				t.Fatalf("Vk did not shrink selection to header: %+v", s)
			}
			h.shot("header-row-vjk-" + name)
		})
	}
}
