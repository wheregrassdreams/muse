package editor

import "testing"

func TestPaintVisibleSpanIsOrderedAndExcludesFoldedLeaves(t *testing.T) {
	h := newHarness(t, foldTestSource())
	h.frame()
	clickFoldChevron(h, headingAtSource(t, h.e, "# Top"))
	h.e.layoutIfNeeded()

	for i, l := range h.e.visibleLeaves {
		if h.e.foldHidden(l) {
			t.Fatalf("visible layout index contains folded leaf at %d: %q", i, h.e.Source()[l.lineStart:l.end])
		}
		if i > 0 && l.y < h.e.visibleLeaves[i-1].y {
			t.Fatalf("visible layout index is not y-sorted at %d: %v < %v", i, l.y, h.e.visibleLeaves[i-1].y)
		}
	}

	next := headingAtSource(t, h.e, "# Next")
	span := h.e.paintVisibleSpan(next.y, next.y+next.h)
	if len(span) == 0 {
		t.Fatal("viewport span omitted the visible next heading")
	}
	found := false
	for _, l := range span {
		if l == next {
			found = true
		}
		if l.y+l.h < next.y-40 || l.y > next.y+next.h+40 {
			t.Fatalf("viewport span included a distant leaf: y=%v h=%v target=%v..%v", l.y, l.h, next.y, next.y+next.h)
		}
	}
	if !found {
		t.Fatal("viewport span did not contain its target leaf")
	}
}

func TestPaintVisibleSpanKeepsLeavesInsideViewportSpanningQuote(t *testing.T) {
	src := "> [!NOTE] Long note\n>\n" +
		"> first paragraph in the callout.\n>\n" +
		"> second paragraph in the callout.\n>\n" +
		"> third paragraph in the callout.\n\n" +
		"Outside paragraph.\n"
	h := newHarness(t, src)
	h.frame()

	var quote []*leaf
	for _, l := range h.e.visibleLeaves {
		for _, c := range l.conts {
			if isCalloutContainer(c) {
				quote = append(quote, l)
				break
			}
		}
	}
	if len(quote) < 3 {
		t.Fatalf("expected a multi-leaf callout, got %d leaves", len(quote))
	}
	middle := quote[len(quote)/2]
	span := h.e.paintVisibleSpan(middle.y, middle.y+middle.h)
	seen := false
	for _, l := range span {
		for _, c := range l.conts {
			if isCalloutContainer(c) && c.id == quote[0].conts[0].id {
				seen = true
			}
		}
	}
	if !seen {
		t.Fatal("viewport span omitted callout content while its container crosses the viewport")
	}
}
