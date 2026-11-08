package mdedit

import "testing"

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
