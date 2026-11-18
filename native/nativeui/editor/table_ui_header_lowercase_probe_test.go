package editor

import (
	"muse/editor/input/vim"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLowercaseVHorizontalCellSelectionThemes(t *testing.T) {
	src := "| Local file | Source page | Creator / source | License |\n| --- | --- | --- | --- |\n| `Attachments/landscape.webp` | [File:Webp (1).webp](https://commons.wikimedia.org/wiki/File:Webp_(1).webp) | Churchill Machine Tool Co. Ltd.; source: picryl.com | [CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/) |\n| another file | [File:A](https://example.test) | Public-domain artist | Public domain |"
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
			h.e.Focus()
			h.frame()
			table := documentTestTable(t, h.e)
			rowTop, rowBottom := table.geom.rowY[1], table.geom.rowY[2]
			x := table.x + (table.geom.colX[3]+table.geom.colX[4])/2
			y := table.y + rowTop + (rowBottom-rowTop)*0.78 - h.e.scroll.Y
			h.tt.Move(x, y)
			h.tt.Press(x, y)
			h.tt.Release(x, y)
			h.frame()
			_, row, col := cellAt(table, h.e.Caret())
			if row != 1 || col != 3 {
				t.Fatalf("blank License click landed at (%d,%d), want (1,3)", row, col)
			}
			h.tt.Key(0, ui.KeyV)
			h.frame()
			h.tt.Type("v")
			h.frame()
			for i := 0; i < 100; i++ {
				h.tt.Key(0, ui.KeyH)
				h.frame()
				h.tt.Type("h")
				h.frame()
				if s := h.e.tableSelection; s != nil && s.cellRegion && s.focusRow == 1 && s.focusCol == 0 {
					if s.axis != tableRows || s.first != 1 || s.last != 1 || s.anchorRow != 1 || s.anchorCol != 3 {
						t.Fatalf("leftward lowercase-v selection has inconsistent row bounds: %+v", s)
					}
					// Completing the first body row horizontally and moving one
					// more cell left must include the table header row too.
					h.tt.Key(0, ui.KeyH)
					h.frame()
					h.tt.Type("h")
					h.frame()
					s = h.e.tableSelection
					if s == nil || !s.cellRegion || s.axis != tableRows || s.first != 0 || s.last != 1 || s.anchorRow != 1 || s.anchorCol != 3 || s.focusRow != 0 || s.focusCol != 0 {
						t.Fatalf("h past first body cell did not extend the rectangle into the header: %+v", s)
					}
					if h.e.tableBridge != nil || h.e.vimController().Mode() != vim.ModeVisual || h.e.vimController().View().Linewise || h.e.Session.Selection().Inclusive || h.e.Session.Selection().Linewise {
						t.Fatalf("cell region mixed with another active selection: mode=%s line=%v inclusive=%v linewise=%v region=%+v bridge=%+v", h.e.vimController().Mode(), h.e.vimController().View().Linewise, h.e.Session.Selection().Inclusive, h.e.Session.Selection().Linewise, s, h.e.tableBridge)
					}
					h.shot("lowercase-v-leftmost-body-cell-" + name)
					// The opposite motion must shrink back to the body row.
					h.tt.Key(0, ui.KeyL)
					h.frame()
					h.tt.Type("l")
					h.frame()
					s = h.e.tableSelection
					if s == nil || s.first != 1 || s.last != 1 || s.focusRow != 1 || s.focusCol != 0 {
						t.Fatalf("l did not reverse the header-row extension: %+v", s)
					}
					h.tt.Key(0, ui.KeyK)
					h.frame()
					h.tt.Type("k")
					h.frame()
					if s := h.e.tableSelection; s == nil || s.first != 0 || s.last != 1 || s.focusRow != 0 {
						t.Fatalf("k did not expand back into the header: %+v", s)
					}
					h.tt.Key(0, ui.KeyJ)
					h.frame()
					h.tt.Type("j")
					h.frame()
					if s := h.e.tableSelection; s == nil || s.first != 1 || s.last != 1 || s.focusRow != 1 {
						t.Fatalf("j did not return to the body row: %+v", s)
					}
					// Cross left into the header, then leave the table. The document
					// selection keeps the complete two-row rectangle as a semantic part.
					h.tt.Key(0, ui.KeyH)
					h.frame()
					h.tt.Type("h")
					h.frame()
					h.tt.Key(0, ui.KeyH)
					h.frame()
					h.tt.Type("h")
					h.frame()
					if h.e.tableSelection != nil || h.e.tableDocumentRegion == nil || h.e.tableDocumentRegion.first != 0 || h.e.tableDocumentRegion.last != 1 {
						t.Fatalf("leaving the header/body rectangle lost its document region: active=%+v document=%+v", h.e.tableSelection, h.e.tableDocumentRegion)
					}
					return
				}
			}
			t.Fatal("lowercase-v horizontal motion did not reach the leftmost body cell")
		})
	}
}

func TestLowercaseVHorizontalWrapRightAndReverse(t *testing.T) {
	h := newHarness(t, "| A | B | C | D |\n| --- | --- | --- | --- |\n| a | b | c | d |\n| e | f | g | h |")
	h.e.SetKeymap("vim")
	table := documentTestTable(t, h.e)
	h.caretAt(table.tab.rows[1][0].start)
	sendVim(h, "v")
	for i := 0; i < 40; i++ {
		sendVim(h, "l")
		if s := h.e.tableSelection; s != nil && s.cellRegion && s.focusRow == 1 && s.focusCol == 3 {
			break
		}
		if i == 39 {
			t.Fatalf("lowercase v/l did not reach the last cell in row 1: %+v", h.e.tableSelection)
		}
	}
	sendVim(h, "l")
	if s := h.e.tableSelection; s == nil || !s.cellRegion || s.axis != tableRows || s.first != 1 || s.last != 2 || s.focusRow != 2 || s.focusCol != 3 {
		t.Fatalf("l at the right row edge did not extend into the next row: %+v", s)
	}
	sendVim(h, "h")
	if s := h.e.tableSelection; s == nil || !s.cellRegion || s.axis != tableRows || s.first != 1 || s.last != 1 || s.focusRow != 1 || s.focusCol != 3 {
		t.Fatalf("h did not reverse the right row-wrap transition: %+v", s)
	}
}
