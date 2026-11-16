package editor

import (
	"testing"
)

func TestClickInTableToHeadingSpacingTargetsBlankLine(t *testing.T) {
	const src = "| 左对齐 | 居中 | 右对齐 |\n| --- | --- | --- |\n| 甲 | 乙 | 3 |\n| 一行比较长的内容，用来看看换行 | `code` | 42 |\n| [[双链]] | $x^2$ | 注释 而已 |\n\n## 代码\n\n```go\nfunc main() {\n    println(\"hello, muse\")\n}\n```"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	h.e.layoutIfNeeded()
	var table, blank, heading *leaf
	for _, l := range h.e.leaves {
		switch l.kind {
		case kTable:
			table = l
		case kBlank:
			if table != nil && heading == nil && blank == nil {
				blank = l
			}
		case kHeading:
			heading = l
		}
	}
	if table == nil || blank == nil || heading == nil {
		t.Fatalf("fixture parse failed: table=%v blank=%v heading=%v", table != nil, blank != nil, heading != nil)
	}
	if table.view != nil {
		t.Fatal("fixture should exercise a geom-backed table without a text view")
	}
	lastRow := table.tab.rows[len(table.tab.rows)-1]
	h.e.setCaret(lastRow[0].start, false)
	h.e.modalText("j")
	if h.e.Caret() != blank.start {
		t.Fatalf("Vim j from the final table row landed at %d, want the existing blank row at %d", h.e.Caret(), blank.start)
	}
	// The heading's section margin makes this point visually empty and outside
	// the blank leaf itself. It should still resolve to the actual source blank
	// line, rather than the heading text or an invented position in the table.
	y := blank.y + blank.h + (heading.y-blank.y-blank.h)*0.65
	if h.e.onALine(y) {
		t.Fatalf("fixture click y %.1f unexpectedly falls on a leaf", y)
	}
	h.tt.ClickAt(h.e.colX+30, y)
	h.frame()
	if h.e.Caret() != blank.start || h.e.phantom != nil {
		t.Fatalf("gap click landed at %d (phantom=%v), want existing blank row at %d", h.e.Caret(), h.e.phantom, blank.start)
	}
	if got := h.e.leafAt(h.e.Caret()); got == nil || got.kind != kBlank {
		t.Fatalf("gap click landed in unit kind %v, want the existing blank row", got)
	}
	h.shot("table-gap-click-mapped-blank")

	// Removing that visible empty row must not delete bytes from the table's
	// final source row or destroy the parsed table structure.
	h.e.modalText("V")
	h.e.modalText("d")
	if got := h.e.Source(); got != "| 左对齐 | 居中 | 右对齐 |\n| --- | --- | --- |\n| 甲 | 乙 | 3 |\n| 一行比较长的内容，用来看看换行 | `code` | 42 |\n| [[双链]] | $x^2$ | 注释 而已 |\n## 代码\n\n```go\nfunc main() {\n    println(\"hello, muse\")\n}\n```" {
		t.Fatalf("linewise delete changed unexpected source: %q", got)
	}
	table = nil
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	if table == nil {
		t.Fatal("deleting the gap line made the preceding table render as raw Markdown")
	}
}
