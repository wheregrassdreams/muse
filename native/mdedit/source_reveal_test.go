package mdedit

import (
	"math"
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

func TestSearchHighlightsRevealOnlyMatchedInlineSource(t *testing.T) {
	const src = "**bold** and ~~strike~~ tail"
	h := newHarness(t, src)
	h.caretAt(len(h.e.src))
	h.frame()
	if h.shows("**bold**") || h.shows("~~strike~~") {
		t.Fatalf("fixture started with inline syntax visible: %q", h.e.leaves[0].view.disp.text)
	}

	// A non-current match must reveal the same hidden source as a caret or
	// selection, while an unrelated hidden span remains in preview form.
	h.e.SetHighlights([]Range{{Start: strings.Index(src, "bold"), End: strings.Index(src, "bold") + len("bold")}}, -1)
	if !sourceRangeTargeted(h.e, 0, len(src), false) {
		t.Fatalf("highlight range index was not active: %+v", h.e.sourceRevealRanges)
	}
	h.frame()
	if !h.shows("**bold**") {
		t.Fatalf("highlighted match did not reveal its Markdown delimiters: %q", h.e.leaves[0].view.disp.text)
	}
	if h.shows("~~strike~~") {
		t.Fatalf("unmatched inline syntax was revealed: %q", h.e.leaves[0].view.disp.text)
	}

	strike := strings.Index(src, "strike")
	h.e.SetHighlights([]Range{{Start: strike, End: strike + len("strike")}}, 0)
	h.frame()
	if h.shows("**bold**") || !h.shows("~~strike~~") {
		t.Fatalf("incremental match replacement kept stale reveal state: %q", h.e.leaves[0].view.disp.text)
	}

	h.e.SetHighlights(nil, -1)
	h.e.setVimSearchHighlights([]Range{{Start: strike, End: strike + len("strike")}}, 0)
	h.frame()
	if !h.shows("~~strike~~") || h.shows("**bold**") {
		t.Fatalf("Vim match did not share targeted reveal behavior: %q", h.e.leaves[0].view.disp.text)
	}
}

func TestVirtualSearchHighlightRevealsVisibleMatch(t *testing.T) {
	var parts []string
	for i := 0; i < 40; i++ {
		parts = append(parts, "ordinary paragraph")
	}
	parts[0] = "**search target**"
	src := strings.Join(parts, "\n\n")
	h := newHarness(t, src)
	if !h.e.virtualLayout() {
		t.Fatal("fixture did not activate virtual layout")
	}
	h.e.anchor, h.e.caret = len(src), len(src)
	h.e.wantReveal = false
	h.frame()
	h.e.SetHighlights([]Range{{Start: 2, End: len("**search target")}}, -1)
	h.frame()
	var visibleMatch *leaf
	for _, l := range h.e.leaves {
		if strings.Contains(src[l.lineStart:min(l.end, len(src))], "search target") {
			visibleMatch = l
			break
		}
	}
	if visibleMatch == nil || visibleMatch.view == nil {
		t.Fatal("search target was not shaped in the virtual viewport")
	}
	if !strings.Contains(visibleMatch.view.disp.text, "**search target**") {
		t.Fatalf("visible virtual search match did not reveal source: %q", visibleMatch.view.disp.text)
	}
}

func TestVirtualSearchRevealIsAppliedWhenOffscreenMatchEntersViewport(t *testing.T) {
	var parts []string
	for i := 0; i < 40; i++ {
		parts = append(parts, "ordinary paragraph")
	}
	parts[30] = "**search target**"
	src := strings.Join(parts, "\n\n")
	h := newHarness(t, src)
	h.e.anchor, h.e.caret = len(src), len(src)
	h.e.wantReveal = false
	h.frame()
	var target *leaf
	for _, l := range h.e.leaves {
		if strings.Contains(src[l.lineStart:min(l.end, len(src))], "search target") {
			target = l
			break
		}
	}
	if target == nil || target.view != nil {
		t.Fatalf("fixture target should be outside the shaped viewport: %+v", target)
	}
	h.e.SetHighlights([]Range{{Start: target.start + 2, End: target.start + 15}}, -1)
	h.frame()
	if target.view != nil {
		t.Fatal("offscreen search result was eagerly shaped")
	}
	h.e.scroll.Y = target.y
	h.frame()
	if target.view == nil || !strings.Contains(target.view.disp.text, "**search target**") {
		t.Fatalf("offscreen match did not reveal source when virtual scrolling reached it: view=%+v scroll=%.1f targetY=%.1f view=[%.1f,%.1f] stale=%v", target.view, h.e.scroll.Y, target.y, h.e.viewTop, h.e.viewBottom, h.e.sourceRevealStale(target))
	}
}

func TestVirtualSearchHeightCorrectionUsesIndexedDeltas(t *testing.T) {
	longTarget := "<https://example.com/" + strings.Repeat("segment/", 100) + ">"
	parts := []string{"[findme](" + longTarget + ")"}
	for i := 0; i < 40; i++ {
		parts = append(parts, "ordinary paragraph")
	}
	src := strings.Join(parts, "\n\n")
	h := newHarness(t, src)
	h.e.anchor, h.e.caret = len(src), len(src)
	h.e.wantReveal = false
	h.frame()
	if !h.e.virtualIndexReady {
		t.Fatal("fixture did not initialize virtual height index")
	}
	target := h.e.leaves[0]
	oldHeight := target.h
	oldNextY := h.e.virtualY(1)
	baseDocH := h.e.virtualBaseDocH
	match := strings.Index(src, "findme")
	h.e.SetHighlights([]Range{{Start: match, End: match + len("findme")}}, -1)
	h.frame()
	if target.h <= oldHeight {
		t.Fatalf("revealing the long link source did not increase height: old=%v new=%v view=%q", oldHeight, target.h, target.view.disp.text)
	}
	delta := target.h - oldHeight
	if math.Abs(float64(h.e.virtualY(1)-oldNextY-delta)) > 0.05 {
		t.Fatalf("following leaf did not move by indexed height delta: before=%v after=%v delta=%v", oldNextY, h.e.virtualY(1), delta)
	}
	if want := h.e.virtualBaseDocH + h.e.virtualHeightPrefix(len(h.e.visibleLeaves)); math.Abs(float64(h.e.docH-want)) > 0.05 || math.Abs(float64(h.e.docH-baseDocH-delta)) > 0.05 {
		t.Fatalf("document extent did not use indexed correction: before=%v got=%v want=%v", baseDocH, h.e.docH, want)
	}
	h.e.scroll.Y = max(h.e.virtualY(len(h.e.visibleLeaves)-10)-h.e.viewH/2, 0)
	h.frame()
	previousY := float32(-1)
	for i, leaf := range h.e.visibleLeaves {
		y := h.e.virtualY(i)
		if y < previousY {
			t.Fatalf("virtual geometry is not ordered after height correction: leaf=%d y=%v previous=%v", i, y, previousY)
		}
		previousY = y
		if y+leaf.h > h.e.scroll.Y && y < h.e.scroll.Y+h.e.viewH && math.Abs(float64(leaf.y-y)) > 0.05 {
			t.Fatalf("visible leaf %d cached y=%v differs from indexed y=%v", i, leaf.y, y)
		}
		if leaf.kind == kPara && y+leaf.h > h.e.scroll.Y && y < h.e.scroll.Y+h.e.viewH && leaf.view == nil {
			t.Fatalf("visible leaf %d remained unshaped after scrolling through indexed correction", i)
		}
	}
}

func TestSearchHighlightRevealsFencedCodeSource(t *testing.T) {
	const src = "before\n\n```go\nx := 1\n```\n\nafter"
	h := newHarness(t, src)
	var code *leaf
	for _, l := range h.e.leaves {
		if l.kind == kCode {
			code = l
			break
		}
	}
	if code == nil {
		t.Fatal("fixture has no code leaf")
	}
	h.caretAt(len(src))
	h.frame()
	if strings.Contains(code.view.disp.text, "```go") {
		t.Fatalf("fixture unexpectedly exposed code fence: %q", code.view.disp.text)
	}
	match := strings.Index(src, "x := 1")
	h.e.SetHighlights([]Range{{Start: match, End: match + len("x := 1")}}, 0)
	h.frame()
	if !strings.Contains(code.view.disp.text, "```go") {
		t.Fatalf("search match inside code block did not reveal fence source: %q", code.view.disp.text)
	}
}

func TestSearchHighlightRevealsHTMLBlockSource(t *testing.T) {
	const src = "<section>\ncontent\n</section>\n\nafter"
	h := newHarness(t, src)
	var html *leaf
	for _, l := range h.e.leaves {
		if l.kind == kHTML {
			html = l
			break
		}
	}
	if html == nil {
		t.Fatal("fixture has no HTML leaf")
	}
	h.e.anchor, h.e.caret = len(src), len(src)
	h.e.wantReveal = false
	h.frame()
	h.e.SetHighlights([]Range{{Start: strings.Index(src, "content"), End: strings.Index(src, "content") + len("content")}}, 0)
	h.frame()
	if !strings.Contains(html.view.disp.text, "<section>") {
		t.Fatalf("search match inside HTML block did not reveal its source: %q", html.view.disp.text)
	}
}

func TestSearchHighlightRevealsInlineSourceInsideTableCell(t *testing.T) {
	const src = "| heading |\n| --- |\n| **bold** |\n\nafter"
	h := newHarness(t, src)
	var table *leaf
	for _, l := range h.e.leaves {
		if l.kind == kTable {
			table = l
			break
		}
	}
	if table == nil {
		t.Fatal("fixture has no table")
	}
	h.e.anchor, h.e.caret = len(src), len(src)
	h.e.wantReveal = false
	h.frame()
	start := strings.Index(src, "bold")
	h.e.SetHighlights([]Range{{Start: start, End: start + len("bold")}}, -1)
	h.frame()
	cell := table.tab.rows[1][0]
	if !strings.Contains(cell.view.disp.text, "**bold**") {
		t.Fatalf("search match did not reveal cell syntax: %q", cell.view.disp.text)
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
