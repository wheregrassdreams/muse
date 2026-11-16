package editor

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestEditorFoldOperationsAreKeymapNeutral(t *testing.T) {
	h := newHarness(t, foldTestSource())
	top := headingAtSource(t, h.e, "# Top")
	child := headingAtSource(t, h.e, "## Child")
	h.caretAt(top.lineStart)
	if !h.e.ApplyOperation(EditorOperation{Kind: OperationFoldAtCaret, Fold: FoldClose}) || !h.e.isFoldedHeading(top) {
		t.Fatal("generic fold close did not close the caret section")
	}
	h.e.ApplyOperation(EditorOperation{Kind: OperationFoldAtCaret, Fold: FoldOpen})
	if h.e.isFoldedHeading(top) {
		t.Fatal("generic fold open did not open the caret section")
	}
	h.e.ApplyOperation(EditorOperation{Kind: OperationFoldAtCaret, Fold: FoldCloseRecursive})
	if !h.e.isFoldedHeading(top) || !h.e.isFoldedHeading(child) {
		t.Fatal("recursive close did not close nested sections")
	}
	h.e.ApplyOperation(EditorOperation{Kind: OperationFoldAtCaret, Fold: FoldOpenRecursive})
	if h.e.isFoldedHeading(top) || h.e.isFoldedHeading(child) {
		t.Fatal("recursive open left nested sections closed")
	}
	h.e.ApplyOperation(EditorOperation{Kind: OperationFoldAll, Fold: FoldCloseAll})
	for _, l := range []*leaf{top, child, headingAtSource(t, h.e, "### Grandchild"), headingAtSource(t, h.e, "# Next")} {
		if _, canFold := h.e.foldSectionAtHeading(l); canFold && !h.e.isFoldedHeading(l) {
			t.Fatalf("fold all left %q open", h.e.src[l.lineStart:l.end])
		}
	}
	h.e.ApplyOperation(EditorOperation{Kind: OperationFoldAll, Fold: FoldOpenAll})
	if len(h.e.folded) != 0 {
		t.Fatalf("open all retained %d folds", len(h.e.folded))
	}
}

func TestPageJumpDurationThresholdAndMaximum(t *testing.T) {
	viewH := float32(700)
	threshold := viewH * pageJumpMinDistanceInViews
	if got := pageJumpDuration(threshold, viewH); got != 0 {
		t.Fatalf("distance at threshold duration=%v, want instant", got)
	}
	near := pageJumpDuration(threshold*2, viewH)
	far := pageJumpDuration(threshold*64, viewH)
	if near <= 0 || far < near || far > pageJumpMaxDuration || far != pageJumpMaxDuration {
		t.Fatalf("distance-based durations near=%v far=%v, maximum=%v", near, far, pageJumpMaxDuration)
	}
	if got := pageJumpAnimationDuration(threshold*2, viewH, pageJumpMaxAnimatedLeaves+1, true); got != 0 {
		t.Fatalf("large virtual document animation=%v, want instant to bound per-frame layout work", got)
	}
	if got := pageJumpAnimationDuration(threshold*2, viewH, 10, false); got != 0 {
		t.Fatalf("disabled preference animation=%v, want instant", got)
	}
}

func TestPageJumpUsesAnimationOnlyForLongEnabledNavigation(t *testing.T) {
	src := strings.Repeat("paragraph for page navigation\n\n", 90)
	target := 0
	for i := 0; i < 65; i++ {
		target = strings.Index(src[target+1:], "paragraph") + target + 1
	}
	if target < 0 {
		t.Fatal("target paragraph missing")
	}
	h := newHarness(t, src)
	h.e.NavigateToOffset(target, PageJumpTopMargin)
	h.frame()
	if h.e.scroll.Scrolling() {
		t.Fatal("default-off page jump should be immediate")
	}
	if h.e.scroll.Y <= 0 {
		t.Fatalf("instant page jump did not move scroll: %v", h.e.scroll.Y)
	}
	animated := newHarness(t, src)
	opts := h.e.Options()
	opts.SmoothPageJumps = true
	animated.e.SetOptions(opts)
	animated.e.NavigateToOffset(target, PageJumpCenter)
	animated.frame()
	if !animated.e.scroll.Scrolling() {
		t.Fatal("enabled long page jump should use ScrollState animation")
	}
	animated.tt.Advance(100 * time.Millisecond)
	if animated.e.scroll.Y <= 0 {
		t.Fatalf("ScrollState did not advance during the animation: y=%v pending=%v", animated.e.scroll.Y, animated.tt.FramePending())
	}
	animated.tt.Advance(time.Second)
	if animated.e.scroll.Scrolling() || animated.e.scroll.Y <= 0 {
		t.Fatalf("page jump did not settle: y=%v scrolling=%v", animated.e.scroll.Y, animated.e.scroll.Scrolling())
	}
}

func TestNavigateToRangeSelectsAndSchedulesPageJump(t *testing.T) {
	src := strings.Repeat("match in a distant paragraph\n\n", 90)
	target := 0
	for i := 0; i < 65; i++ {
		target = strings.Index(src[target+1:], "match") + target + 1
	}
	h := newHarness(t, src)
	opts := h.e.Options()
	opts.SmoothPageJumps = true
	h.e.SetOptions(opts)
	h.e.NavigateToRange(target, target+len("match"))
	h.frame()
	if got := h.e.SelectionRange(); got != (Range{Start: target, End: target + len("match")}) {
		t.Fatalf("navigated range=%+v", got)
	}
	if !h.e.scroll.Scrolling() {
		t.Fatal("distant range navigation did not use the shared page-jump animation")
	}
}

func TestEditorScrollCaretLinePlacements(t *testing.T) {
	src := strings.Repeat("line content for viewport testing\n", 80)
	h := newHarness(t, src)
	h.caretAt(len("line content for viewport testing\n") * 35)
	caret := h.e.caret
	for _, tc := range []struct {
		placement CaretLinePlacement
		name      string
	}{{CaretLineTop, "top"}, {CaretLineCenter, "center"}, {CaretLineBottom, "bottom"}} {
		h.e.ApplyOperation(EditorOperation{Kind: OperationScrollCaretLine, Placement: tc.placement})
		h.frame()
		r, ok := h.e.caretRect(caret)
		if !ok {
			t.Fatalf("%s placement lost caret geometry", tc.name)
		}
		screenY := r.Y - h.e.scroll.Y
		switch tc.placement {
		case CaretLineTop:
			if screenY > 2 {
				t.Fatalf("top caret screenY=%v", screenY)
			}
		case CaretLineCenter:
			want := (h.e.viewH - r.H) / 2
			if d := screenY - want; d < -2 || d > 2 {
				t.Fatalf("center caret screenY=%v want=%v", screenY, want)
			}
		case CaretLineBottom:
			want := h.e.viewH - r.H
			if d := screenY - want; d < -2 || d > 2 {
				t.Fatalf("bottom caret screenY=%v want=%v", screenY, want)
			}
		}
		if h.e.caret != caret {
			t.Fatalf("%s scroll moved caret: %d want %d", tc.name, h.e.caret, caret)
		}
	}
}

func TestVimZPrefixDispatchesScrollAndFoldOperations(t *testing.T) {
	h := newHarness(t, foldTestSource())
	h.e.SetKeymap("vim")
	top := headingAtSource(t, h.e, "# Top")
	child := headingAtSource(t, h.e, "## Child")
	h.caretAt(top.lineStart)
	modalKeys(h.e, "zt")
	if h.e.scrollPlacement != CaretLineTop {
		t.Fatalf("zt placement=%v", h.e.scrollPlacement)
	}
	modalKeys(h.e, "zz")
	if h.e.scrollPlacement != CaretLineCenter {
		t.Fatalf("zz placement=%v", h.e.scrollPlacement)
	}
	modalKeys(h.e, "zb")
	if h.e.scrollPlacement != CaretLineBottom {
		t.Fatalf("zb placement=%v", h.e.scrollPlacement)
	}
	modalKeys(h.e, "zc")
	if !h.e.isFoldedHeading(top) {
		t.Fatal("zc did not close current section")
	}
	modalKeys(h.e, "zo")
	if h.e.isFoldedHeading(top) {
		t.Fatal("zo did not open current section")
	}
	modalKeys(h.e, "zC")
	if !h.e.isFoldedHeading(top) || !h.e.isFoldedHeading(child) {
		t.Fatal("zC did not recursively close current section")
	}
	modalKeys(h.e, "zO")
	if h.e.isFoldedHeading(top) || h.e.isFoldedHeading(child) {
		t.Fatal("zO did not recursively open current section")
	}
	modalKeys(h.e, "zA")
	if !h.e.isFoldedHeading(top) || !h.e.isFoldedHeading(child) {
		t.Fatal("zA did not recursively toggle closed")
	}
	modalKeys(h.e, "zM")
	if !h.e.isFoldedHeading(headingAtSource(t, h.e, "# Next")) {
		t.Fatal("zM did not close all sections")
	}
	modalKeys(h.e, "zR")
	if len(h.e.folded) != 0 {
		t.Fatal("zR did not open all sections")
	}
	modalKeys(h.e, "zv")
	if h.e.modal.sequence.Pending != "" {
		t.Fatalf("zv left prefix pending: %q", h.e.modal.sequence.Pending)
	}
}

func TestVimZPrefixCancelsTimesOutAndIsKeymapLocal(t *testing.T) {
	e := New("# Heading\n\nBody\n", Options{})
	e.SetKeymap("vim")
	e.modalText("z")
	e.modalKey(ui.KeyEscape, 0)
	if e.modal.sequence.Pending != "" {
		t.Fatal("Escape did not cancel z prefix")
	}
	e.modalText("z")
	e.modal.prefixAt = time.Now().Add(-2 * time.Second)
	e.modalText("t")
	if e.modal.sequence.Pending != "" || e.hasScrollTo {
		t.Fatal("expired z prefix dispatched an operation")
	}
	e.SetKeymap("helix")
	e.modalText("z")
	if e.modal.sequence.Pending != "" {
		t.Fatal("z prefix leaked into helix keymap")
	}
}

func TestVimFindAndReplaceBindingsStayInVimConsumer(t *testing.T) {
	e := New("abcXdef", Options{})
	e.SetKeymap("helix")
	modalKeys(e, "fXrZ;,")
	if e.modal.awaitingCharDirection != 0 || e.modal.awaitingReplacement || e.modal.findValid {
		t.Fatalf("Vim-only find/replace state leaked into Helix: %+v", e.modal)
	}
	if got := e.Source(); got != "abcXdef" {
		t.Fatalf("Vim-only key bindings changed source in Helix: %q", got)
	}
}
