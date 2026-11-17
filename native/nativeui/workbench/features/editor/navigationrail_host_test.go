package editor

import (
	"fmt"
	"strings"
	"testing"

	"muse/nativeui/workbench/features/editor/navigationrail"
)

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

func TestHeadingRailSnapshotOwnsOnlyDetachedNavigationData(t *testing.T) {
	h := newHarness(t, railDoc(6))
	h.frame()
	frame := h.e.HeadingRailSnapshot(navigationrail.ViewportSize{Width: 400, Height: 400})
	if !frame.Visible || len(frame.Items) != 6 {
		t.Fatalf("rail snapshot visible=%v items=%d, want visible with six headings", frame.Visible, len(frame.Items))
	}
	for i, item := range frame.Items {
		if item.Offset != h.e.railLeaves[i].start || item.Level != h.e.railLeaves[i].level {
			t.Fatalf("item %d = %+v, want source heading offset/level", i, item)
		}
	}
	if frame.Active != 0 {
		t.Fatalf("active heading at document top = %d, want 0", frame.Active)
	}
	frame.Items[0].Offset = -1
	if h.e.railItems[0].Offset < 0 {
		t.Fatal("navigation snapshot aliases the editor's cached heading list")
	}
	if narrow := h.e.HeadingRailSnapshot(navigationrail.ViewportSize{Width: 52, Height: 400}); narrow.Visible {
		t.Fatal("rail snapshot visible in a viewport too narrow for the component")
	}

	h.e.ScrollToOffset(frame.Items[4].Offset)
	for i := 0; i < 3; i++ {
		h.frame()
	}
	if got := h.e.HeadingRailSnapshot(navigationrail.ViewportSize{Width: 400, Height: 400}).Active; got != 4 {
		t.Fatalf("active heading after scrolling = %d, want 4", got)
	}
}

func TestHeadingRailPreviewProjectsOnlyRequestedSection(t *testing.T) {
	h := newHarness(t, railDoc(6))
	h.frame()
	preview := h.e.HeadingRailPreview(4)
	if preview.Title != "Heading 4" || preview.Level != 5 || !strings.Contains(preview.Excerpt, "Paragraph 4.0") {
		t.Fatalf("heading preview = %+v", preview)
	}
	if invalid := h.e.HeadingRailPreview(-1); invalid != (navigationrail.Preview{}) {
		t.Fatalf("invalid preview = %+v, want empty value", invalid)
	}
}

func TestJumpToHeadingNavigatesToCurrentSnapshotOffset(t *testing.T) {
	h := newHarness(t, railDoc(6))
	h.frame()
	frame := h.e.HeadingRailSnapshot(navigationrail.ViewportSize{Width: 400, Height: 400})
	target := frame.Items[4].Offset
	if !h.e.JumpToHeading(frame.Revision, target) {
		t.Fatal("jump rejected a heading offset from the current snapshot")
	}
	if h.e.JumpToHeading(frame.Revision+1, target) {
		t.Fatal("jump accepted a stale navigation snapshot revision")
	}
	for i := 0; i < 3; i++ {
		h.frame()
	}
	if h.e.Caret() != target {
		t.Fatalf("caret after jump = %d, want heading at %d", h.e.Caret(), target)
	}
	if h.e.scroll.Y <= 0 {
		t.Fatal("jump did not scroll the heading into view")
	}
	if h.e.JumpToHeading(frame.Revision, target+1) {
		t.Fatal("jump accepted an offset that is not a heading")
	}
}

func TestHeadingRailSnapshotHiddenWithoutScrollableContent(t *testing.T) {
	h := newHarness(t, "# One\n\ntext\n\n## Two\n\ntext")
	h.frame()
	frame := h.e.HeadingRailSnapshot(navigationrail.ViewportSize{Width: 400, Height: 400})
	if frame.Visible {
		t.Fatal("rail snapshot visible when the document does not scroll")
	}
}
