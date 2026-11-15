package mdedit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func TestCopyTableCellRegionPreservesSourceHeaderInHTML(t *testing.T) {
	h := newHarness(t, "| Head | Other |\n| --- | --- |\n| **bold** | *italic* |\n| tail | <script>x</script> |")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)

	h.e.selectTableCellRegion(table, 1, 0, 1, 1)
	if !h.e.Copy() {
		t.Fatal("copy data region failed")
	}
	got, err := h.clip.Read(transfer.HTML)
	if err != nil {
		t.Fatal(err)
	}
	html := string(got)
	if strings.Contains(html, "<th>") || !strings.Contains(html, "<td>") {
		t.Fatalf("data-only region HTML has wrong header semantics: %s", html)
	}
	if strings.Contains(html, "<thead>") || !strings.Contains(html, "<tbody>") {
		t.Fatalf("data-only region retained a synthetic table header: %s", html)
	}
	if !strings.Contains(html, "<strong>bold</strong>") || !strings.Contains(html, "<em>italic</em>") {
		t.Fatalf("inline formatting lost in HTML: %s", html)
	}
	if strings.Contains(html, "<script>") {
		t.Fatalf("raw HTML was emitted from cell content: %s", html)
	}
	markdown, err := h.clip.Read(transfer.Markdown)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(markdown); !strings.HasPrefix(got, "| **bold** | *italic* |\n|") || strings.Contains(got, "Head") {
		t.Fatalf("Markdown did not serialize only the selected rectangle with its first row as header: %s", got)
	}

	h.e.selectTableCellRegion(table, 0, 0, 0, 0)
	if !h.e.Copy() {
		t.Fatal("copy source header region failed")
	}
	got, err = h.clip.Read(transfer.HTML)
	if err != nil {
		t.Fatal(err)
	}
	html = string(got)
	if !strings.Contains(html, "<th>Head</th>") {
		t.Fatalf("source header was not preserved as th: %s", html)
	}
}

func TestTableCellRegionSelectionThemeShots(t *testing.T) {
	src := "| Header A | Header B | Header C |\n| --- | --- | --- |\n| alpha | beta | gamma |\n| delta | epsilon | zeta |"
	for _, dark := range []bool{false, true} {
		name, theme := "light", ui.LightTheme()
		if dark {
			name, theme = "dark", ui.DarkTheme()
		}
		t.Run(name, func(t *testing.T) {
			h := &harness{t: t, e: New(src, Options{FontSize: 16, MaxWidth: 560, PadX: 24})}
			h.tt = ui.NewTester(func(c *ui.Context) {
				ui.Themed(c, theme).Fill().Background(theme.Background).Children(func() { h.e.View(c).Grow(1) })
			}, 720, 400)
			h.frame()
			table := documentTestTable(t, h.e)
			if !h.e.selectTableCellRegion(table, 1, 1, 2, 2) {
				t.Fatal("could not activate cell region")
			}
			caret, ok := h.e.tableSelectionCaretRect()
			if !ok || caret.X < tableContentX(table)+table.geom.colX[2] || caret.X > tableContentX(table)+table.geom.colX[3] || caret.Y < table.y+table.geom.rowY[2] || caret.Y >= table.y+table.geom.rowY[3] {
				t.Fatalf("focus caret escaped focus cell: rect=%+v", caret)
			}
			h.shot("table-region-" + name)
		})
	}
}

func TestMixedDocumentCellRegionThemeShots(t *testing.T) {
	src := "before\n\n| Header A | Header B | Header C |\n| --- | --- | --- |\n| alpha | beta | gamma |\n| delta | epsilon | zeta |\n\nafter"
	for _, dark := range []bool{false, true} {
		name, theme := "light", ui.LightTheme()
		if dark {
			name, theme = "dark", ui.DarkTheme()
		}
		t.Run(name, func(t *testing.T) {
			h := &harness{t: t, e: New(src, Options{FontSize: 16, MaxWidth: 620, PadX: 24})}
			h.tt = ui.NewTester(func(c *ui.Context) {
				ui.Themed(c, theme).Fill().Background(theme.Background).Children(func() { h.e.View(c).Grow(1) })
			}, 780, 420)
			h.frame()
			table := documentTestTable(t, h.e)
			h.e.anchor = 0
			h.e.caret = table.tab.rows[1][0].start + 2
			region := h.e.tableStructureSelection(table)
			if region == nil || !region.cellRegion {
				t.Fatalf("mixed range did not project to cells: %+v", region)
			}
			if a, z := h.e.textSelectionForUnit(&leaf{table: table, start: table.tab.rows[2][2].start, end: table.tab.rows[2][2].end}); a != z {
				t.Fatalf("table text selection leaked under cell mode: [%d,%d)", a, z)
			}
			if a, z := h.e.textSelectionForUnit(h.e.leaves[0]); a == z {
				t.Fatal("external text range disappeared")
			}
			h.shot("mixed-cell-region-" + name)
		})
	}
}

func TestTableMenuFocusUsesActiveRegionSemantics(t *testing.T) {
	h := newHarness(t, "| A | B | C |\n| --- | --- | --- |\n| a | b | c |\n| d | e | f |")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	if !h.e.selectTableCellRegion(table, 1, 0, 2, 1) {
		t.Fatal("could not select initial region")
	}
	active := *h.e.tableSelection
	h.e.focusTableMenuCell(table, 1, 1)
	if *h.e.tableSelection != active {
		t.Fatalf("menu focus inside region changed active selection: got %+v want %+v", *h.e.tableSelection, active)
	}
	h.e.focusTableMenuCell(table, 2, 2)
	region, ok := h.e.tableSelectionCellRegion(h.e.tableSelection)
	if !ok || region.anchorRow != 2 || region.anchorCol != 2 || region.focusRow != 2 || region.focusCol != 2 {
		t.Fatalf("menu focus outside region did not focus its cell: region=%+v ok=%v", region, ok)
	}
}
