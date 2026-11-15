package editor

import (
	"fmt"
	"strings"
	"testing"
)

// railDoc returns a document long enough to scroll, with a heading every
// few paragraphs: `heads` headings, each followed by 12 lines of text.
func railDoc(heads int) string {
	var b strings.Builder
	for i := 0; i < heads; i++ {
		level := i%6 + 1
		fmt.Fprintf(&b, "%s Heading %d\n\n", strings.Repeat("#", level), i)
		for k := 0; k < 12; k++ {
			fmt.Fprintf(&b, "Paragraph %d.%d: the quick brown fox jumps over the lazy dog.\n\n", i, k)
		}
	}
	return b.String()
}

// TestRailMarksHeadings checks that the rail has one tick per heading, in
// order, with the heading's level, and no tick for a heading still being
// typed.
func TestRailMarksHeadings(t *testing.T) {
	// The lone marker at the end is a heading still being typed: it shows
	// as text and is no heading yet, so the rail has no tick for it.
	src := "# One\n\ntext\n\n## Two\n\ntext\n\n### Three\n\ntext\n\n#"
	h := newHarness(t, src)
	if got, want := len(h.e.railLeaves), 3; got != want {
		t.Fatalf("rail has %d ticks, want %d", got, want)
	}
	for i, want := range []int{1, 2, 3} {
		if got := h.e.railLeaves[i].level; got != want {
			t.Fatalf("tick %d is a level %d heading, want %d", i, got, want)
		}
	}
	ys := h.e.railPositions(h.e.viewH)
	if len(ys) != 3 {
		t.Fatalf("rail positions: %d, want 3", len(ys))
	}
	for i := 1; i < len(ys); i++ {
		if ys[i] <= ys[i-1] {
			t.Fatalf("ticks %d and %d are at %.1f and %.1f, want them in order", i-1, i, ys[i-1], ys[i])
		}
	}
	for i, y := range ys {
		if y < 0 || y > h.e.viewH {
			t.Fatalf("tick %d at %.1f is outside the rail of %.1f", i, y, h.e.viewH)
		}
	}
}

// TestRailHiddenWhenShort checks that a document that does not scroll has
// no rail: there is nothing to navigate.
func TestRailHiddenWhenShort(t *testing.T) {
	h := newHarness(t, "# One\n\ntext\n\n## Two\n\ntext")
	if h.e.docH > h.e.viewH {
		t.Fatalf("the document is %.0f tall and scrolls in %.0f", h.e.docH, h.e.viewH)
	}
	if r := h.e.railRect; r.W != 0 || r.H != 0 {
		t.Fatalf("the rail was drawn over a document that does not scroll: %+v", r)
	}
}

// TestRailPositionsDense checks that many headings keep their order and
// stay within the rail, evenly spaced and centered, squeezed closer
// together as they must.
func TestRailPositionsDense(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&b, "## Heading %d\n\ntext\n\n", i)
	}
	h := newHarness(t, b.String())
	const railH float32 = 400
	ys := h.e.railPositions(railH)
	if len(ys) != 200 {
		t.Fatalf("rail positions: %d, want 200", len(ys))
	}
	for i := 1; i < len(ys); i++ {
		if ys[i] <= ys[i-1] {
			t.Fatalf("tick %d at %.1f is not below tick %d at %.1f", i, ys[i], i-1, ys[i-1])
		}
	}
	if ys[0] < 0 || ys[len(ys)-1] > railH {
		t.Fatalf("ticks %.1f..%.1f are outside the rail of %.1f", ys[0], ys[len(ys)-1], railH)
	}
	if mid := (ys[0] + ys[len(ys)-1]) / 2; mid < railH/2-2 || mid > railH/2+2 {
		t.Fatalf("the ticks' middle is at %.1f, want the rail's middle %.1f", mid, railH/2)
	}
	pitch := ys[1] - ys[0]
	if pitch > railItemHeight {
		t.Fatalf("the ticks are %.1f apart, more than the %.1f of a tick's row", pitch, railItemHeight)
	}
	if last := ys[len(ys)-1] - ys[len(ys)-2]; last < pitch-0.01 || last > pitch+0.01 {
		t.Fatalf("the last gap is %.2f, want the even %.2f", last, pitch)
	}
}

// TestRailHoverAndClick checks that hovering a tick previews that
// heading's text and section, and clicking it brings the heading to the
// middle of the view.
func TestRailHoverAndClick(t *testing.T) {
	h := newHarness(t, railDoc(6))
	h.frame()
	r := h.e.railRect
	if r.W <= 0 || r.H <= 0 {
		t.Fatalf("the rail was not drawn: %+v", r)
	}
	ys := h.e.railPositions(r.H)
	if len(ys) != 6 {
		t.Fatalf("rail positions: %d, want 6", len(ys))
	}

	x := r.X + r.W/2
	h.tt.Move(x, r.Y+ys[4])
	h.frame()
	if h.e.railHover != 4 {
		t.Fatalf("hovering tick 4 made %d hover", h.e.railHover)
	}
	if got := h.e.railHeadingText(4); got != "Heading 4" {
		t.Fatalf("preview text is %q, want %q", got, "Heading 4")
	}
	if got := h.e.railHeadingDesc(4); !strings.Contains(got, "Paragraph 4.0") {
		t.Fatalf("preview description is %q, want the section's first paragraph", got)
	}

	h.tt.ClickAt(x, r.Y+ys[4])
	head := h.e.railLeaves[4]
	for i := 0; i < 3; i++ {
		h.frame()
	}
	if h.e.caret != head.start {
		t.Fatalf("caret at %d after clicking tick 4, want the heading at %d", h.e.caret, head.start)
	}
	if h.e.scroll.Y <= 0 {
		t.Fatal("the document did not scroll to the clicked heading")
	}
	if y := head.y - h.e.scroll.Y; y < h.e.viewH*0.25 || y > h.e.viewH*0.75 {
		t.Fatalf("the clicked heading is at y=%.0f of %.0f, want it near the middle", y, h.e.viewH)
	}
}

// TestRailScalesByDistance checks the rail's lens: the tick of the
// highlighted heading is full width, and the ones further from it
// narrower, by distance; with nothing hovered, the heading the view is in
// is the one highlighted.
func TestRailScalesByDistance(t *testing.T) {
	h := newHarness(t, railDoc(8))
	h.frame()
	r := h.e.railRect
	ys := h.e.railPositions(r.H)

	h.tt.Move(r.X+r.W/2, r.Y+ys[4])
	for i := 0; i < 40; i++ {
		h.frame()
	}
	want := []float32{0.25, 0.25, 0.44, 0.68, 1, 0.68, 0.44, 0.25}
	for i, w := range want {
		if got := h.e.railScales[i]; got < w-0.02 || got > w+0.02 {
			t.Fatalf("tick %d is %.2f wide, want %.2f", i, got, w)
		}
	}

	// With the pointer off the rail, the heading the view is in (the first,
	// at the top) takes the full width.
	h.tt.Move(10, 10)
	for i := 0; i < 40; i++ {
		h.frame()
	}
	want = []float32{1, 0.68, 0.44, 0.25, 0.25, 0.25, 0.25, 0.25}
	for i, w := range want {
		if got := h.e.railScales[i]; got < w-0.02 || got > w+0.02 {
			t.Fatalf("tick %d is %.2f wide with no hover, want %.2f", i, got, w)
		}
	}
}

// TestRailCurrentFollowsViewport checks that the tick of the heading the
// reader is in is the one highlighted.
func TestRailCurrentFollowsViewport(t *testing.T) {
	h := newHarness(t, railDoc(6))
	h.frame()
	if cur := h.e.railCurrent(); cur != 0 {
		t.Fatalf("the current tick is %d at the top of the document, want 0", cur)
	}
	// Scroll well into the fourth heading's section.
	head := h.e.railLeaves[4]
	h.e.ScrollToOffset(head.start)
	for i := 0; i < 3; i++ {
		h.frame()
	}
	if cur := h.e.railCurrent(); cur != 4 {
		t.Fatalf("the current tick is %d after scrolling to heading 4, want 4", cur)
	}
}

func TestDenseRailMergesOnlyContiguousMatchingTicks(t *testing.T) {
	ys := []float32{0, 0.5, 1, 1.5, 2, 2.5, 3}
	scales := []float32{0.25, 0.25, 1, 0.25, 0.25, 0.25, 0.25}
	if end := railRunEnd(ys, scales, 0, 2, -1); end != 2 {
		t.Fatalf("first dense tick run ends at %d, want 2", end)
	}
	if end := railRunEnd(ys, scales, 2, 2, -1); end != 3 {
		t.Fatalf("highlighted tick run ends at %d, want 3", end)
	}
	if end := railRunEnd(ys, scales, 3, 2, -1); end != len(ys) {
		t.Fatalf("last dense tick run ends at %d, want %d", end, len(ys))
	}
	if end := railRunEnd([]float32{0, 3}, []float32{0.25, 0.25}, 0, -1, -1); end != 1 {
		t.Fatalf("separated ticks were merged through a gap: end=%d", end)
	}
}
