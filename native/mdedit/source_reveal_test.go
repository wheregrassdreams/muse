package mdedit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSelectionRevealsEditableSource(t *testing.T) {
	h := newHarness(t, "**bold** and plain\n\nnext")
	h.e.Select(2, 6)
	h.frame()
	if !h.shows("**bold**") {
		t.Fatalf("selected emphasis source is hidden: %q", h.e.leaves[0].view.disp.text)
	}
	h.shot("selection-emphasis-source-reveal")
	h.caretAt(len(h.e.src))
	h.frame()
	if h.shows("**bold**") {
		t.Fatalf("cleared selection left source revealed: %q", h.e.leaves[0].view.disp.text)
	}
}

func TestReverseSelectionRevealsAcrossParagraphs(t *testing.T) {
	h := newHarness(t, "**first**\n\n~~second~~")
	h.e.anchor, h.e.caret = len(h.e.src), 0
	h.e.wantReveal = true
	h.frame()
	if !strings.Contains(h.e.leaves[0].view.disp.text, "**first**") {
		t.Fatalf("reverse selection did not reveal first paragraph: %q", h.e.leaves[0].view.disp.text)
	}
	last := h.e.leaves[len(h.e.leaves)-1]
	if !strings.Contains(last.view.disp.text, "~~second~~") {
		t.Fatalf("reverse selection did not reveal second paragraph: %q", last.view.disp.text)
	}
}

func TestCaretCanEnterTaskPrefix(t *testing.T) {
	h := newHarness(t, "- [ ] task")
	leaf := h.e.leaves[0]
	content := leaf.start
	if content < 0 || content >= len(h.e.src) || h.e.src[content] != 't' {
		t.Fatalf("task content starts at %d", content)
	}
	h.caretAt(content)
	h.key(0, ui.KeyLeft)
	if h.e.caret != content-1 {
		t.Fatalf("left from task text landed at %d, want checkbox prefix byte %d; start=%d opens=%v reveal=%d disp=%q stops=%v", h.e.caret, content-1, leaf.start, leaf.opensItem(), h.e.revealFor(leaf, content-1), leaf.view.disp.text, leaf.view.disp.stops())
	}
	if !strings.Contains(h.e.leaves[0].view.disp.text, "- [ ] task") {
		t.Fatalf("task prefix did not reveal while caret entered it: %q", h.e.leaves[0].view.disp.text)
	}
	h.shot("task-prefix-caret-reveal")
}

func TestClickingExpandedTaskSourceDoesNotToggleCheckbox(t *testing.T) {
	h := newHarness(t, "- [ ] task")
	l := h.e.leaves[0]
	it, ok := l.innerItem()
	if !ok || it.taskAt < 0 {
		t.Fatal("task did not parse")
	}
	h.e.Select(it.taskAt, it.taskAt+1)
	h.frame()
	r, ok := h.e.caretRect(it.taskAt)
	if !ok {
		t.Fatal("expanded task source has no caret geometry")
	}
	x, y := r.X, r.Y-h.e.scroll.Y
	if _, hit := h.e.taskBoxAt(x, y); hit {
		t.Fatal("expanded task source still has a checkbox hit target")
	}
	h.tt.ClickAt(x, y+r.H/2)
	h.want("- [ ] task")
}

func TestListPrefixStopsMatchSourceAndHitGeometry(t *testing.T) {
	h := newHarness(t, "- first\n12. second\n  - [ ] nested\n  - [x] done")
	h.e.layoutIfNeeded()
	items := 0
	for _, l := range h.e.leaves {
		s, end, ok := itemPrefixRange(l)
		if !ok {
			continue
		}
		items++
		for off := s; off <= end; off++ {
			h.caretAt(l.start)
			v := h.e.unitView(l, off)
			d, mapped := v.disp.srcToDisp(off - l.lineStart)
			if !mapped || v.disp.dispToSrc(d, false)+l.lineStart != off {
				t.Fatalf("list prefix source offset %d is not a stable display stop (display=%d mapped=%v text=%q)", off, d, mapped, v.disp.text)
			}
			r, visible := h.e.caretRect(off)
			if !visible || !h.e.stopOK(l, off) {
				t.Fatalf("list prefix offset %d has no visible caret stop", off)
			}
			if hit := h.e.hitUnit(l, v, r.X, r.Y+r.H/2); hit != off {
				t.Fatalf("list prefix hit at caret geometry returned %d, want %d (rect=%+v text=%q)", hit, off, r, v.disp.text)
			}
		}
	}
	if items < 4 {
		t.Fatalf("covered %d list/task prefixes, want bullet, ordered, nested unchecked and checked", items)
	}
}

func TestCellTextSelectionRevealsInlineSyntax(t *testing.T) {
	h := newHarness(t, "| cell |\n| --- |\n| **bold** |")
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	if table == nil {
		t.Fatal("table did not parse")
	}
	cell := table.tab.rows[1][0]
	h.e.Select(cell.start+2, cell.start+6)
	h.frame()
	if !strings.Contains(cell.view.disp.text, "**bold**") {
		t.Fatalf("cell text selection did not reveal inline syntax: %q", cell.view.disp.text)
	}
}

func TestStructuralTableSelectionKeepsGridRendering(t *testing.T) {
	h := newHarness(t, "| cell |\n| --- |\n| **bold** |")
	h.e.layoutIfNeeded()
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	if table == nil {
		t.Fatal("table did not parse")
	}
	h.e.selectTableAxis(table, tableRows, 0, len(table.tab.rows)-1, false)
	h.frame()
	cell := table.tab.rows[1][0]
	if strings.Contains(cell.view.disp.text, "**") {
		t.Fatalf("structural table selection revealed cell syntax: %q", cell.view.disp.text)
	}
}

func TestSelectionRevealsEditableBlockSyntax(t *testing.T) {
	src := "## heading\n\n```go\nx := 1\n```\n\n$$\nx^2\n$$\n\n> quote\n\n> [!NOTE]\n> callout\n\n<div>\nhtml\n</div>\n\noutside"
	h := newHarness(t, src)
	h.e.anchor, h.e.caret = 0, len(src)
	h.frame()
	h.shot("selection-editable-block-syntax")
	wantByKind := map[leafKind]string{
		kHeading: "## heading",
		kCode:    "```go",
		kMath:    "$$",
		kHTML:    "<div>",
	}
	seen := map[leafKind]bool{}
	for _, l := range h.e.leaves {
		if want, ok := wantByKind[l.kind]; ok && !seen[l.kind] {
			if l.view == nil || !strings.Contains(l.view.disp.text, want) {
				t.Fatalf("selection did not reveal %v syntax %q: %q", l.kind, want, dispText(l))
			}
			seen[l.kind] = true
		}
		if l.kind == kPara && strings.Contains(src[l.lineStart:min(l.end, len(src))], "> quote") {
			if !strings.Contains(l.view.disp.text, "> quote") {
				t.Fatalf("selection did not reveal quote prefix: %q", l.view.disp.text)
			}
		}
		if l.kind == kPara {
			if _, _, ok := h.e.calloutHeader(l); ok && !strings.Contains(l.view.disp.text, "> [!NOTE]") {
				t.Fatalf("selection did not reveal callout header: %q", l.view.disp.text)
			}
		}
	}
	for kind := range wantByKind {
		if !seen[kind] {
			t.Fatalf("fixture did not produce leaf kind %v", kind)
		}
	}
	if got := h.e.SelectionRange(); got.Start != 0 || got.End != len(src) || h.e.Source() != src {
		t.Fatalf("source reveal altered source selection or text: range=%+v source=%q", got, h.e.Source())
	}
}

func dispText(l *leaf) string {
	if l == nil || l.view == nil {
		return "<no view>"
	}
	return l.view.disp.text
}
