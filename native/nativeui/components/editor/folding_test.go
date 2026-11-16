package editor

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func headingAtSource(t *testing.T, e *Editor, prefix string) *leaf {
	t.Helper()
	for _, l := range e.leaves {
		if l.kind == kHeading && l.lineStart < len(e.Source()) && len(e.Source()[l.lineStart:]) >= len(prefix) && e.Source()[l.lineStart:l.lineStart+len(prefix)] == prefix {
			return l
		}
	}
	t.Fatalf("heading %q not found in %q", prefix, e.Source())
	return nil
}

func foldTestSource() string {
	return "# Top\n\nA long first section paragraph with enough text to wrap in the test column.\n\n## Child\n\nChild paragraph.\n\n### Grandchild\n\nGrandchild paragraph.\n\n# Next\n\nVisible ending."
}

func clickFoldChevron(h *harness, heading *leaf) {
	h.t.Helper()
	x, y, size, ok := h.e.foldChevronRect(heading)
	if !ok {
		h.t.Fatalf("heading %q has no fold control", h.e.Source()[heading.lineStart:heading.end])
	}
	x += size / 2
	y += size/2 - h.e.scroll.Y
	h.tt.ClickAt(x, y)
	h.frame()
}

func TestFoldClickHidesNestedSectionWithoutChangingSource(t *testing.T) {
	h := newHarness(t, foldTestSource())
	h.e.opts.MaxWidth = 430
	h.e.invalidate()
	h.frame()
	child := headingAtSource(t, h.e, "## Child")
	next := headingAtSource(t, h.e, "# Next")
	childY, nextY := child.y, next.y
	beforeH, beforeSource := h.e.docH, h.e.Source()
	clickFoldChevron(h, headingAtSource(t, h.e, "# Top"))
	h.e.layoutIfNeeded()
	child = headingAtSource(t, h.e, "## Child")
	next = headingAtSource(t, h.e, "# Next")
	if child.y < next.y && child.y >= 0 {
		t.Fatalf("folded child remains in visible layout: child y=%v next y=%v (before child=%v next=%v)", child.y, next.y, childY, nextY)
	}
	if h.e.docH >= beforeH {
		t.Fatalf("fold did not reduce document height: before=%v after=%v", beforeH, h.e.docH)
	}
	if got := h.e.Source(); got != beforeSource {
		t.Fatalf("fold changed source: %q", got)
	}
	h.shot("folded-nested-section")
}

func TestFoldCanReopenAndAddsEllipsis(t *testing.T) {
	h := newHarness(t, foldTestSource())
	top := headingAtSource(t, h.e, "# Top")
	clickFoldChevron(h, top)
	if !h.e.isFoldedHeading(top) || !strings.Contains(top.view.disp.text, "…") {
		t.Fatalf("folded heading has no collapsed state/ellipsis: %q", top.view.disp.text)
	}
	h.shot("folded-heading-ellipsis")
	clickFoldChevron(h, top)
	if h.e.isFoldedHeading(top) || headingAtSource(t, h.e, "## Child").y < 0 {
		t.Fatal("clicking the open chevron did not restore the section")
	}
	h.shot("expanded-nested-section")
}

func TestFoldChevronTracksFirstDisplayedTitleLine(t *testing.T) {
	src := "# H1 title that wraps across multiple lines when the editor is narrow\n\nbody 1\n\n## H2\n\nbody 2\n\n### H3\n\nbody 3\n\n#### H4\n\nbody 4\n\n##### H5\n\nbody 5\n\n###### H6\n\nbody 6\n\nSetext title\n============\n\nbody 7\n\n# Raw source title\n\nbody 8"
	h := newHarness(t, src)
	h.e.opts.MaxWidth = 245
	h.e.invalidate()
	h.frame()

	for _, prefix := range []string{"# H1", "## H2", "### H3", "#### H4", "##### H5", "###### H6", "Setext title", "# Raw source"} {
		title := headingAtSource(t, h.e, prefix)
		if prefix == "# H1" {
			if len(title.view.layout.Lines()) < 2 {
				t.Fatal("test fixture's H1 did not wrap")
			}
		}
		if prefix == "# Raw source" {
			h.caretAt(title.start)
		}
		lines := title.view.layout.Lines()
		if len(lines) == 0 {
			t.Fatalf("heading %q has no laid out title lines", prefix)
		}
		_, y, size, ok := h.e.foldChevronRect(title)
		if !ok {
			t.Fatalf("heading %q has no fold control", prefix)
		}
		want := title.y + title.view.ty + lines[0].Rect.Y + (lines[0].Rect.H-size)/2
		if delta := y - want; delta < -0.05 || delta > 0.05 {
			t.Fatalf("heading %q chevron y=%v, want first displayed line center %v (delta %v; lines=%v)", prefix, y, want, delta, lines)
		}
	}
	if !headingAtSource(t, h.e, "Setext title").setext {
		t.Fatal("test fixture's underline heading is not Setext")
	}
}

func TestFoldChevronOnlyAppearsAndHitsWhileTitleIsHovered(t *testing.T) {
	h := newHarness(t, foldTestSource())
	top := headingAtSource(t, h.e, "# Top")
	beforeCaret, beforeAnchor, beforeScroll := h.e.Caret(), h.e.Session.Selection().Anchor, h.e.scroll
	iconX, iconY, size, ok := h.e.foldChevronRect(top)
	if !ok {
		t.Fatal("fold control setup failed")
	}
	if h.e.foldChevronAt(iconX+size/2, iconY+size/2) != nil {
		t.Fatal("unhovered fold control retained a hit target")
	}
	line := top.view.layout.Lines()[0]
	titleX := top.x + top.view.tx + line.Rect.X + line.Rect.W/2
	titleY := top.y + top.view.ty + line.Rect.Y + line.Rect.H/2
	h.e.hover(titleX, titleY)
	if h.e.foldHover != top.lineStart {
		t.Fatal("hovering the title did not reveal its fold control")
	}
	h.shot("fold-hover-title")
	// Moving from the title to the adjacent left-side arrow corridor must
	// retain the same heading as the hover target.
	h.e.hover(iconX+size/2, iconY+size/2)
	if h.e.foldHover != top.lineStart || h.e.foldChevronAt(iconX+size/2, iconY+size/2) != top {
		t.Fatal("moving into the arrow corridor hid or disconnected the fold control")
	}
	if h.e.Caret() != beforeCaret || h.e.Session.Selection().Anchor != beforeAnchor || h.e.scroll != beforeScroll {
		t.Fatal("fold hover changed caret, selection, or scroll")
	}
	h.e.toggleFold(top)
	h.e.hover(titleX, titleY)
	if h.e.foldHover != top.lineStart || !strings.Contains(top.view.disp.text, "…") {
		t.Fatal("folded heading did not retain hover visibility and its ellipsis")
	}
	lines := top.view.layout.Lines()
	_, foldedY, foldedSize, ok := h.e.foldChevronRect(top)
	if !ok || len(lines) == 0 {
		t.Fatal("folded heading lost its fold control or title layout")
	}
	wantFoldedY := top.y + top.view.ty + lines[0].Rect.Y + (lines[0].Rect.H-foldedSize)/2
	if delta := foldedY - wantFoldedY; delta < -0.05 || delta > 0.05 {
		t.Fatalf("folded chevron y=%v, want title line center %v", foldedY, wantFoldedY)
	}
	h.shot("fold-hover-folded")
	h.e.hover(980, 680)
	if h.e.foldHover != -1 || h.e.foldChevronAt(iconX+size/2, iconY+size/2) != nil {
		t.Fatal("fold control remained visible or hit-testable after leaving the title")
	}
	h.shot("fold-hover-left")
}

func TestFoldHoverAndClickUseDocumentCoordinatesAfterScroll(t *testing.T) {
	var src strings.Builder
	for i := 0; i < 16; i++ {
		src.WriteString("# Filler\n\nparagraph text\n\n")
	}
	src.WriteString("# Scrolled heading\n\nbody\n\n# End\n\nend")
	h := newHarness(t, src.String())
	target := headingAtSource(t, h.e, "# Scrolled heading")
	h.e.scroll.Y = target.y - 120
	h.frame()
	target = headingAtSource(t, h.e, "# Scrolled heading")
	line := target.view.layout.Lines()[0]
	titleX := target.x + target.view.tx + line.Rect.X + line.Rect.W/2
	titleScreenY := target.y - h.e.scroll.Y + target.view.ty + line.Rect.Y + line.Rect.H/2
	h.tt.Move(titleX, titleScreenY)
	h.frame()
	if h.e.foldHover != target.lineStart {
		t.Fatalf("hovering scrolled title set foldHover=%d, want %d", h.e.foldHover, target.lineStart)
	}
	iconX, iconY, size, ok := h.e.foldChevronRect(target)
	if !ok {
		t.Fatal("scrolled heading lost fold geometry")
	}
	iconScreenY := iconY - h.e.scroll.Y + size/2
	h.tt.Move(iconX+size/2, iconScreenY)
	h.frame()
	if h.e.foldHover != target.lineStart {
		t.Fatal("moving to the scrolled arrow corridor cleared the heading hover")
	}
	h.tt.ClickAt(iconX+size/2, iconScreenY)
	h.frame()
	if !h.e.isFoldedHeading(target) {
		t.Fatal("clicking the scrolled arrow did not fold its heading")
	}
}

func TestNestedFoldStateSurvivesParentFold(t *testing.T) {
	h := newHarness(t, foldTestSource())
	child := headingAtSource(t, h.e, "## Child")
	top := headingAtSource(t, h.e, "# Top")
	clickFoldChevron(h, child)
	if !h.e.isFoldedHeading(child) {
		t.Fatal("child fold did not activate")
	}
	clickFoldChevron(h, top)
	clickFoldChevron(h, top)
	if !h.e.isFoldedHeading(child) || !h.e.foldHidden(headingAtSource(t, h.e, "### Grandchild")) {
		t.Fatal("unfolding the parent discarded the nested child fold")
	}
}

func TestFoldRangeSupportsSetextAndNoContentHeadings(t *testing.T) {
	h := newHarness(t, "Setext title\n============\n\nbody\n\nNext\n----\n\nlast\n\n# Empty\n\n# End")
	setext := headingAtSource(t, h.e, "Setext title")
	if !setext.setext {
		t.Fatal("fixture's first heading is not Setext")
	}
	if section, ok := h.e.foldSectionAtHeading(setext); !ok || !section.content {
		t.Fatalf("Setext section not foldable: %+v content=%v", section, ok)
	}
	empty := headingAtSource(t, h.e, "# Empty")
	if _, ok := h.e.foldSectionAtHeading(empty); ok {
		t.Fatal("heading without content exposed a fold control")
	}
}

func TestExplicitSelectAndScrollExpandFoldedBody(t *testing.T) {
	h := newHarness(t, foldTestSource())
	top := headingAtSource(t, h.e, "# Top")
	child := headingAtSource(t, h.e, "## Child")
	clickFoldChevron(h, top)
	if !h.e.foldHidden(child) {
		t.Fatal("test setup did not hide child heading")
	}
	h.e.Select(child.start, child.end)
	if h.e.foldHidden(child) {
		t.Fatal("explicit source selection did not expand its parent fold")
	}
	clickFoldChevron(h, top)
	paragraph := paragraphContaining(h.e, "Grandchild paragraph.")
	if paragraph == nil {
		t.Fatal("test paragraph not found")
	}
	clickFoldChevron(h, top)
	h.e.ScrollToOffset(paragraph.start)
	if h.e.foldHidden(paragraph) {
		t.Fatal("explicit scroll-to-offset did not expand its parent fold")
	}
}

func TestFoldNavigationAndHitTestingSkipHiddenLeaves(t *testing.T) {
	h := newHarness(t, foldTestSource())
	top := headingAtSource(t, h.e, "# Top")
	child := headingAtSource(t, h.e, "## Child")
	childY := child.y
	clickFoldChevron(h, top)
	if !h.e.foldHidden(child) || child.y > -1000 {
		t.Fatalf("child is not projected out of layout: hidden=%v y=%v", h.e.foldHidden(child), child.y)
	}
	hitLeaf := h.e.leafAtPoint(childY)
	if hitLeaf == child || h.e.isFoldedSourceOffset(h.e.hitTest(child.x+12, childY)) {
		t.Fatal("hit testing returned hidden folded content")
	}
	h.caretAt(top.end)
	h.key(0, ui.KeyDown)
	if h.e.isFoldedSourceOffset(h.e.Caret()) {
		t.Fatalf("vertical navigation landed in hidden source at %d", h.e.Caret())
	}
	if got := h.e.nextVisibleSourceOffset(child.start, 1); h.e.isFoldedSourceOffset(got) {
		t.Fatalf("next visible source helper returned hidden offset %d", got)
	}
}

func TestVimLinewiseSelectionKeepsFocusVisibleAndCopiesHiddenSource(t *testing.T) {
	h := newHarness(t, foldTestSource())
	top := headingAtSource(t, h.e, "# Top")
	child := headingAtSource(t, h.e, "## Child")
	clickFoldChevron(h, top)
	h.e.nativeSelectLine(top.lineStart)
	h.e.nativeMoveLineSelection(1, 1)
	if h.e.isFoldedSourceOffset(h.e.Caret()) {
		t.Fatalf("linewise selection focus is hidden at %d", h.e.Caret())
	}
	if got := h.e.SelectedText(); !strings.Contains(got, "Child paragraph") {
		t.Fatalf("linewise selection lost source inside the folded section: %q", got)
	}
	if !h.e.foldHidden(child) {
		t.Fatal("linewise selection opened the fold")
	}
}

func TestClickFoldWhileCaretIsInBodyMovesCaretToVisibleHeading(t *testing.T) {
	h := newHarness(t, foldTestSource())
	top := headingAtSource(t, h.e, "# Top")
	body := paragraphContaining(h.e, "A long first section paragraph")
	h.caretAt(body.start + 5)
	clickFoldChevron(h, top)
	if h.e.isFoldedSourceOffset(h.e.Caret()) || h.e.Caret() != top.end || h.e.Session.Selection().Anchor != h.e.Caret() {
		t.Fatalf("caret remained in hidden body after folding: anchor=%d caret=%d", h.e.Session.Selection().Anchor, h.e.Caret())
	}
	if h.e.tableSelection != nil || h.e.tableBridge != nil {
		t.Fatal("fold retained a hidden table structural selection")
	}
}

func TestFoldHidesTaskTableAndImageHitTargets(t *testing.T) {
	src := "# Top\n\n- [ ] task\n\n| cell |\n| --- |\n| value |\n\n![image](missing.png)\n\n# Next\n\nend"
	h := newHarness(t, src)
	top := headingAtSource(t, h.e, "# Top")
	var task, table, picture *leaf
	for _, l := range h.e.leaves {
		if l.opensItem() {
			task = l
		}
		if l.kind == kTable {
			table = l
		}
		if l.kind == kPara && strings.Contains(h.e.Source()[l.start:l.end], "image") {
			picture = l
		}
	}
	if task == nil || table == nil || picture == nil {
		t.Fatal("fixture did not produce the task, table and image leaves")
	}
	taskY := task.y + h.e.lk.size*.8 - h.e.scroll.Y
	taskX := h.e.colX + h.e.lk.size*itemIndent - h.e.lk.size*.9
	clickFoldChevron(h, top)
	h.frame()
	if !h.e.foldHidden(task) || !h.e.foldHidden(table) || !h.e.foldHidden(picture) {
		t.Fatal("fold did not hide all nested controls")
	}
	if _, ok := h.e.taskBoxAt(taskX, taskY); ok {
		t.Fatal("folded task checkbox kept a hit target")
	}
	if hit := h.e.leafAtPoint(table.y); hit == table {
		t.Fatal("folded table kept a mouse hit target")
	}
	for _, hit := range h.e.imageHits {
		if hit.leaf == picture {
			t.Fatal("folded image kept an image control hit target")
		}
	}
}

func TestOrdinaryHeadingSelectionDoesNotExpandFold(t *testing.T) {
	h := newHarness(t, foldTestSource())
	top := headingAtSource(t, h.e, "# Top")
	child := headingAtSource(t, h.e, "## Child")
	clickFoldChevron(h, top)
	h.e.Select(top.lineStart, top.end)
	if !h.e.isFoldedHeading(top) || !h.e.foldHidden(child) {
		t.Fatal("selecting the folded heading opened its section")
	}
}

func TestSelectAllAndCopyKeepFoldedSourceWithoutOpening(t *testing.T) {
	h := newHarness(t, foldTestSource())
	src := h.e.Source()
	top := headingAtSource(t, h.e, "# Top")
	clickFoldChevron(h, top)
	h.e.SelectAll()
	if !h.e.isFoldedHeading(top) || !h.e.foldHidden(headingAtSource(t, h.e, "## Child")) {
		t.Fatal("ordinary SelectAll expanded explicit fold")
	}
	if got := h.e.SelectedText(); got != src {
		t.Fatalf("copy selection lost folded source: %q", got)
	}
}

func TestSourceEditAndUndoClearFoldState(t *testing.T) {
	h := newHarness(t, foldTestSource())
	top := headingAtSource(t, h.e, "# Top")
	clickFoldChevron(h, top)
	if !h.e.isFoldedHeading(top) {
		t.Fatal("fold setup failed")
	}
	h.e.Replace(0, 0, "Intro\n\n")
	if len(h.e.folded) != 0 {
		t.Fatal("source edit left offset-based fold state alive")
	}
	h.e.Undo()
	if len(h.e.folded) != 0 {
		t.Fatal("undo restored stale fold state")
	}
	h.e.SetSource(foldTestSource())
	if len(h.e.folded) != 0 {
		t.Fatal("SetSource leaked fold state to the new document")
	}
}

func paragraphContaining(e *Editor, s string) *leaf {
	for _, l := range e.leaves {
		if l.kind == kPara && strings.Contains(e.Source()[l.start:l.end], s) {
			return l
		}
	}
	return nil
}
