package mdedit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

const headerBoundarySource = "Before paragraph.\n\n| Local file | Source page | Creator / source | License |\n| --- | --- | --- | --- |\n| attachment | source | creator | license |\n| next | next | next | next |\n\nAfter paragraph."

func pointerPlaceVimCaret(t *testing.T, h *harness, needle string, delta int) {
	t.Helper()
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	off := strings.Index(h.e.src, needle) + delta
	caret, ok := h.e.caretRect(off)
	if !ok {
		t.Fatalf("no caret geometry for %q at %d", needle, off)
	}
	x, y := caret.X, caret.Y+caret.H/2-h.e.scroll.Y
	h.tt.Move(x, y)
	h.tt.Press(x, y)
	t.Logf("after blank-area press: tableCellDrag=%+v caret=%d tableSelection=%+v", h.e.tableCellDrag, h.e.caret, h.e.tableSelection)
	h.tt.Release(x, y)
	h.frame()
}

func physicalVim(t *testing.T, h *harness, key ui.Key, text string, mods ui.Modifiers) {
	t.Helper()
	h.tt.Key(mods, key)
	h.frame()
	h.tt.Type(text)
	h.frame()
}

func logHeaderSelection(t *testing.T, label string, h *harness) {
	t.Helper()
	row, col := -1, -1
	if table := documentTestTable(t, h.e); table != nil {
		_, row, col = cellAt(table, h.e.caret)
	}
	t.Logf("%s: caret=%d cell=(%d,%d) anchor=%d mode=%s line=%v table=%+v documentRegion=%+v bridge=%+v ranges=%+v", label, h.e.caret, row, col, h.e.anchor, h.e.modal.mode, h.e.modal.line, h.e.tableSelection, h.e.tableDocumentRegion, h.e.tableBridge, h.e.SelectionRanges())
}

func TestTextVisualHFromBodyLicenseAcrossWrappedTableTracksSelectionState(t *testing.T) {
	src := "Before paragraph.\n\n| Local file | Source page | Creator / source | License |\n| --- | --- | --- | --- |\n| `Attachments/landscape.webp` | [File:Webp (1).webp](https://commons.wikimedia.org/wiki/File:Webp_(1).webp) | Churchill Machine Tool Co. Ltd.; source: picryl.com | [CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/) |\n| another file | [File:A](https://example.test) | Public-domain artist | Public domain |\n\nAfter paragraph."
	h := &harness{t: t, e: New(src, Options{FontSize: 18, MaxWidth: 820, PadX: 32})}
	h.e.SetKeymap("vim")
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { h.e.View(c).Grow(1) })
	}, 1040, 520)
	h.frame()
	h.frame()
	h.e.Focus()
	h.frame()
	table := documentTestTable(t, h.e)
	start := table.tab.rows[1][3].start + 2
	caret, ok := h.e.caretRect(start)
	if !ok {
		t.Fatal("wrapped body License caret geometry unavailable")
	}
	x, y := caret.X, caret.Y+caret.H/2-h.e.scroll.Y
	h.tt.Move(x, y)
	h.tt.Press(x, y)
	h.tt.Release(x, y)
	h.frame()
	logHeaderSelection(t, "pointer-license", h)
	physicalVim(t, h, ui.KeyV, "v", 0)
	logHeaderSelection(t, "v", h)
	for i := 0; i < 240; i++ {
		physicalVim(t, h, ui.KeyH, "h", 0)
		logHeaderSelection(t, "v-h"+strings.Repeat("h", i), h)
		table = documentTestTable(t, h.e)
		_, row, col := cellAt(table, h.e.caret)
		if row == 0 && col == 1 {
			if h.e.tableSelection != nil && h.e.modal.mode == "Visual" && h.e.modal.vimTextVisualActive {
				t.Fatalf("mixed active visual+table selection at Source header: selection=%+v bridge=%+v ranges=%+v", h.e.tableSelection, h.e.tableBridge, h.e.SelectionRanges())
			}
			return
		}
		if row < 0 {
			if h.e.tableSelection != nil || h.e.tableDocumentRegion == nil || h.e.tableDocumentRegion.first != 0 || h.e.tableDocumentRegion.last != 1 {
				t.Fatalf("leaving table lost the active header/body region alongside external text selection after h=%d: selection=%+v documentRegion=%+v bridge=%+v ranges=%+v", i+1, h.e.tableSelection, h.e.tableDocumentRegion, h.e.tableBridge, h.e.SelectionRanges())
			}
			return
		}
	}
	t.Fatalf("240 h motions did not reach Source header or table boundary; caret=%d selection=%+v documentRegion=%+v bridge=%+v ranges=%+v", h.e.caret, h.e.tableSelection, h.e.tableDocumentRegion, h.e.tableBridge, h.e.SelectionRanges())
}

func TestTextVisualHAfterPointerClickInWrappedLicenseCellBlankArea(t *testing.T) {
	src := "Before paragraph.\n\n| Local file | Source page | Creator / source | License |\n| --- | --- | --- | --- |\n| `Attachments/landscape.webp` | [File:Webp (1).webp](https://commons.wikimedia.org/wiki/File:Webp_(1).webp) | Churchill Machine Tool Co. Ltd.; source: picryl.com | [CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/) |\n| another file | [File:A](https://example.test) | Public-domain artist | Public domain |\n\nAfter paragraph."
	h := &harness{t: t, e: New(src, Options{FontSize: 18, MaxWidth: 820, PadX: 32})}
	h.e.SetKeymap("vim")
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { h.e.View(c).Grow(1) })
	}, 1040, 520)
	h.frame()
	table := documentTestTable(t, h.e)
	left, right := table.geom.colX[3], table.geom.colX[4]
	top, bottom := table.geom.rowY[1], table.geom.rowY[2]
	// The link is laid out at the top of the tall row. This point is well below
	// its rendered text but still inside the License cell, matching the report.
	x := table.x + (left+right)*0.5
	y := table.y + top + (bottom-top)*0.78 - h.e.scroll.Y
	if row, col, inside := tableCellIndexAt(table, x, y); !inside || row != 1 || col != 3 {
		t.Fatalf("blank-area test point maps to (%d,%d), inside=%v", row, col, inside)
	}
	h.tt.Move(x, y)
	h.tt.Press(x, y)
	h.tt.Release(x, y)
	h.frame()
	logHeaderSelection(t, "pointer-license-blank", h)
	_, row, col := cellAt(table, h.e.caret)
	if row != 1 || col != 3 {
		t.Fatalf("blank-area click caret mapped to (%d,%d), want body License (1,3), caret=%d", row, col, h.e.caret)
	}
	physicalVim(t, h, ui.KeyV, "v", 0)
	logHeaderSelection(t, "blank-v", h)
	for i := 0; i < 10; i++ {
		physicalVim(t, h, ui.KeyH, "h", 0)
		logHeaderSelection(t, "blank-v-h"+strings.Repeat("h", i), h)
	}
}

func TestVimHorizontalMotionFromBodyLicenseToHeaderSourceKeepsRow(t *testing.T) {
	h := newHarness(t, headerBoundarySource)
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	cell := table.tab.rows[1][3]
	off := cell.start + 2
	caret, ok := h.e.caretRect(off)
	if !ok {
		t.Fatal("body License caret geometry unavailable")
	}
	x, y := caret.X, caret.Y+caret.H/2-h.e.scroll.Y
	h.tt.Move(x, y)
	h.tt.Press(x, y)
	h.tt.Release(x, y)
	h.frame()
	_, row, col := cellAt(table, h.e.caret)
	if row != 1 || col != 3 {
		t.Fatalf("pointer landed at (%d,%d), want first body row License cell", row, col)
	}
	logHeaderSelection(t, "pointer-body-license", h)
	physicalVim(t, h, ui.KeyV, "V", ui.Shift)
	logHeaderSelection(t, "V", h)
	for i := 0; i < 48; i++ {
		physicalVim(t, h, ui.KeyH, "h", 0)
		if i == 0 || i == 7 || i == 15 || i == 31 || i == 47 {
			logHeaderSelection(t, "h"+strings.Repeat("h", i), h)
		}
	}
	if s := h.e.tableSelection; s != nil {
		t.Logf("final row structure after h motion: %+v", s)
	}
}

// Lowercase v starts as a text selection. Moving left from the rightmost body
// cell should promote to a cell rectangle as soon as the focus crosses a cell;
// subsequent h motions must keep the same anchor and never leave a stale
// header text caret paired with a body-only structural highlight.
func TestVimTextVisualHorizontalMotionFromBodyLicenseKeepsCellFocusConsistent(t *testing.T) {
	h := newHarness(t, headerBoundarySource)
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	caret, ok := h.e.caretRect(table.tab.rows[1][3].start + 2)
	if !ok {
		t.Fatal("body License caret geometry unavailable")
	}
	x, y := caret.X, caret.Y+caret.H/2-h.e.scroll.Y
	h.tt.Move(x, y)
	h.tt.Press(x, y)
	h.tt.Release(x, y)
	h.frame()
	physicalVim(t, h, ui.KeyV, "v", 0)
	logHeaderSelection(t, "body-v", h)
	for i := 0; i < 120; i++ {
		physicalVim(t, h, ui.KeyH, "h", 0)
		if s := h.e.tableSelection; s != nil && s.cellRegion {
			if s.lineStart != table.lineStart || s.anchorRow != 1 || s.anchorCol != 3 {
				t.Fatalf("h%d changed the lowercase-v anchor: %+v", i+1, s)
			}
		}
		if i == 0 || i == 3 || i == 7 || i == 15 || i == 31 || i == 63 || i == 119 {
			logHeaderSelection(t, "body-v"+strings.Repeat("h", i+1), h)
		}
		if i == 3 {
			h.shot("body-license-v-leftmost-cell")
		}
	}
	if s := h.e.tableSelection; s != nil && s.cellRegion && s.anchorRow != s.focusRow {
		t.Fatalf("horizontal motion changed row focus unexpectedly: %+v", s)
	}
	h.shot("body-license-v-left")
}

func TestNormalVimHorizontalMotionFromBodyLicense(t *testing.T) {
	h := newHarness(t, headerBoundarySource)
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	cell := table.tab.rows[1][3]
	caret, ok := h.e.caretRect(cell.start + 2)
	if !ok {
		t.Fatal("body License caret geometry unavailable")
	}
	x, y := caret.X, caret.Y+caret.H/2-h.e.scroll.Y
	h.tt.Press(x, y)
	h.tt.Release(x, y)
	h.frame()
	for i := 0; i < 80; i++ {
		physicalVim(t, h, ui.KeyH, "h", 0)
		if i == 0 || i == 7 || i == 15 || i == 31 || i == 47 || i == 63 || i == 79 {
			logHeaderSelection(t, "normal-h"+strings.Repeat("h", i+1), h)
		}
	}
}

func TestBodyLicenseVimVKHRetainsHeaderAndBodyRows(t *testing.T) {
	h := newHarness(t, headerBoundarySource)
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	caret, ok := h.e.caretRect(table.tab.rows[1][3].start + 2)
	if !ok {
		t.Fatal("body License caret geometry unavailable")
	}
	x, y := caret.X, caret.Y+caret.H/2-h.e.scroll.Y
	h.tt.Press(x, y)
	h.tt.Release(x, y)
	h.frame()
	physicalVim(t, h, ui.KeyV, "V", ui.Shift)
	logHeaderSelection(t, "body-V", h)
	physicalVim(t, h, ui.KeyK, "k", 0)
	logHeaderSelection(t, "body-Vk", h)
	for i := 0; i < 20; i++ {
		physicalVim(t, h, ui.KeyH, "h", 0)
	}
	logHeaderSelection(t, "body-Vk-h20", h)
	if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 0 || s.last != 1 {
		t.Fatalf("Vk followed by horizontal motion omitted header row: %+v", s)
	}
}

func TestHeaderVimSelectionBoundaryRoundTrips(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cell  string
		delta int
	}{
		{name: "first-header-column", cell: "Local file", delta: 2},
		{name: "middle-header-column", cell: "Source page", delta: 3},
		{name: "last-header-column", cell: "License", delta: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, headerBoundarySource)
			pointerPlaceVimCaret(t, h, tc.cell, tc.delta)
			logHeaderSelection(t, "pointer", h)
			physicalVim(t, h, ui.KeyV, "V", ui.Shift)
			logHeaderSelection(t, "V", h)
			physicalVim(t, h, ui.KeyJ, "j", 0)
			logHeaderSelection(t, "Vj", h)
			if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 0 || s.last != 1 {
				t.Fatalf("Vj omitted header: %+v", s)
			}
			physicalVim(t, h, ui.KeyK, "k", 0)
			logHeaderSelection(t, "Vjk", h)
			if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 0 || s.last != 0 {
				t.Fatalf("Vjk did not shrink to header: %+v", s)
			}
		})
	}
}

func TestHeaderVimSelectionLeavesAndReturnsAcrossTableBoundary(t *testing.T) {
	h := newHarness(t, headerBoundarySource)
	pointerPlaceVimCaret(t, h, "Source page", 3)
	physicalVim(t, h, ui.KeyV, "V", ui.Shift)
	physicalVim(t, h, ui.KeyJ, "j", 0)
	physicalVim(t, h, ui.KeyJ, "j", 0)
	physicalVim(t, h, ui.KeyJ, "j", 0)
	logHeaderSelection(t, "Vjjj-outside", h)
	physicalVim(t, h, ui.KeyK, "k", 0)
	logHeaderSelection(t, "Vjjjk-return", h)
	if s := h.e.tableSelection; s == nil || s.axis != tableRows || s.first != 0 || s.last != 2 {
		t.Fatalf("returning into table lost header row selection: %+v", s)
	}
}
