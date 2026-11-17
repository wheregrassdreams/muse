package editor

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestHeadingRevealsItsHashes(t *testing.T) {
	h := newHarness(t, "## 标题\n\n正文")
	h.caretAt(3) // in the heading
	if !h.shows("## 标题") {
		t.Fatalf("the heading shows %q", h.e.leaves[0].view.disp.text)
	}
	h.caretAt(10) // in the body paragraph
	if h.shows("##") {
		t.Fatalf("the heading keeps its hashes: %q", h.e.leaves[0].view.disp.text)
	}
}

func TestHeadingMarkerRevealWaitsForPrefixSpacePosition(t *testing.T) {
	h := newHarness(t, "## 标题\n\n正文")
	opts := h.e.Options()
	opts.HeadingMarkerReveal = true
	h.e.SetOptions(opts)
	h.caretAt(3) // on the first heading character, after `## `
	if h.shows("## 标题") {
		t.Fatalf("heading markers revealed while the caret is on heading text: %q", h.e.leaves[0].view.disp.text)
	}
	h.caretAt(2) // on the separating space in the `## ` prefix
	if !h.shows("## 标题") {
		t.Fatalf("heading markers did not reveal on the prefix space: %q", h.e.leaves[0].view.disp.text)
	}
	for _, at := range []int{0, 1} { // remain navigable while moving through `##`
		h.caretAt(at)
		if !h.shows("## 标题") {
			t.Fatalf("heading prefix collapsed at source offset %d: %q", at, h.e.leaves[0].view.disp.text)
		}
		if !h.e.stopOK(h.e.leaves[0], at) {
			t.Fatalf("caret cannot enter heading prefix at source offset %d", at)
		}
	}
}

func TestHeadingMarkerRevealKeepsVimCaretOnWholeHeadingGlyph(t *testing.T) {
	h := newHarness(t, "# Home")
	opts := h.e.Options()
	opts.HeadingMarkerReveal = true
	h.e.SetOptions(opts)
	h.caretAt(2) // Vim's normal caret is on H; the prefix must stay hidden.
	if h.shows("# Home") {
		t.Fatalf("prefix revealed while caret was on H: %q", h.e.leaves[0].view.disp.text)
	}
	start, ok := h.e.caretRect(2)
	if !ok {
		t.Fatal("caret on H has no rendered position")
	}
	next, ok := h.e.caretRect(3)
	if !ok {
		t.Fatal("caret after H has no rendered position")
	}
	wantWidth := next.X - start.X
	got := h.e.blockCaretRect(start).W
	if wantWidth <= 0 || got != wantWidth {
		t.Fatalf("block caret width = %v, want full H glyph width %v", got, wantWidth)
	}
}

func TestHeadingMarkerRevealCanNavigateLeftIntoPrefix(t *testing.T) {
	h := newHarness(t, "# Home")
	h.e.SetKeymap("vim")
	opts := h.e.Options()
	opts.HeadingMarkerReveal = true
	h.e.SetOptions(opts)
	h.caretAt(2) // on H, with the marker hidden
	h.key(0, ui.KeyLeft)
	if h.e.Caret() != 1 {
		t.Fatalf("left from H landed at %d, want the separating space at 1", h.e.Caret())
	}
	if !h.shows("# Home") {
		t.Fatalf("moving left onto the prefix space did not reveal it: %q", h.e.leaves[0].view.disp.text)
	}
	h.key(0, ui.KeyLeft)
	if h.e.Caret() != 0 {
		t.Fatalf("left from the prefix space landed at %d, want # at 0", h.e.Caret())
	}
	if !h.shows("# Home") {
		t.Fatalf("heading prefix collapsed before the caret reached # : %q", h.e.leaves[0].view.disp.text)
	}
}

func TestEmptyHeadingShowsPrefixUntilTextIsTyped(t *testing.T) {
	h := newHarness(t, "## content")
	opts := h.e.Options()
	opts.HeadingMarkerReveal = true
	h.e.SetOptions(opts)
	h.caretAt(len(h.e.Source()))
	for range len("content") {
		h.key(0, ui.KeyBackspace)
	}
	if got := h.e.Source(); got != "## " {
		t.Fatalf("deleting heading text left %q, want the empty heading prefix", got)
	}
	if !h.shows("## ") {
		t.Fatalf("empty heading prefix is hidden: %q", h.e.leaves[0].view.disp.text)
	}
	h.typ("N")
	if got := h.e.Source(); got != "## N" {
		t.Fatalf("typing into the empty heading produced %q", got)
	}
	if h.shows("## ") {
		t.Fatalf("heading prefix stayed visible after text was typed: %q", h.e.leaves[0].view.disp.text)
	}
}
