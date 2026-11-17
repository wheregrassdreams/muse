package layout

import (
	"math"
	"testing"
)

func near(got, want float32) bool { return math.Abs(float64(got-want)) < .001 }

func TestContainerIndentAppliesCalloutAndFootnotePolicy(t *testing.T) {
	if got := ContainerIndent(QuoteContainer, 20, 0, false); got != 24 {
		t.Fatalf("quote indent = %v, want 24", got)
	}
	if got := ContainerIndent(QuoteContainer, 20, 0, true); got != 28 {
		t.Fatalf("callout indent = %v, want 28", got)
	}
	if got := ContainerIndent(FootnoteContainer, 20, 50, false); got != 50 {
		t.Fatalf("wide footnote indent = %v, want 50", got)
	}
	if got := ContainerIndent(FootnoteContainer, 20, 20, false); got != 35 {
		t.Fatalf("minimum footnote indent = %v, want 35", got)
	}
}

func TestBlockMarginsResolveEditorLayoutCategories(t *testing.T) {
	if top, bottom := BlockMargins(HeadingBlock, 20, 31, false, false); top != 31 || bottom != 7 {
		t.Fatalf("heading margins = %v/%v, want 31/7", top, bottom)
	}
	if top, bottom := BlockMargins(HeadingBlock, 20, 31, true, false); top != 10 || bottom != 10 {
		t.Fatalf("pending heading margins = %v/%v, want 10/10", top, bottom)
	}
	if top, bottom := BlockMargins(ParagraphBlock, 20, 0, false, true); top != 14 || bottom != 14 {
		t.Fatalf("callout margins = %v/%v, want 14/14", top, bottom)
	}
	if top, bottom := BlockMargins(BlankBlock, 20, 0, false, false); top != 0 || bottom != 0 {
		t.Fatalf("blank margins = %v/%v, want 0/0", top, bottom)
	}
}

func TestVirtualizationPolicyUsesCountAndImageThresholds(t *testing.T) {
	for _, test := range []struct {
		count  int
		images bool
		want   bool
	}{{63, false, false}, {64, false, true}, {15, true, false}, {16, true, true}, {16, false, false}} {
		if got := ShouldVirtualize(test.count, test.images); got != test.want {
			t.Errorf("ShouldVirtualize(%d, %t) = %t, want %t", test.count, test.images, got, test.want)
		}
	}
}

func TestShapePolicyUsesViewportOverscanAndCaretOwner(t *testing.T) {
	query := ShapeQuery{Virtualized: true, Y: 110, Height: 20, ViewportTop: 100, ViewportBottom: 120, Overscan: 16, Caret: 500, SourceStart: 0, SourceEnd: 10}
	if !ShouldShapeBlock(query) {
		t.Fatal("block intersecting the viewport band was deferred")
	}
	query.Y, query.Height = 200, 8
	if ShouldShapeBlock(query) {
		t.Fatal("offscreen block outside overscan was shaped")
	}
	query.Caret = 5
	if !ShouldShapeBlock(query) {
		t.Fatal("caret-owning block was deferred")
	}
	query.Virtualized = false
	query.Caret = 500
	if !ShouldShapeBlock(query) {
		t.Fatal("whole-document layout skipped an offscreen block")
	}
}

func TestEstimateCodeTextAndTableGeometry(t *testing.T) {
	if got := EstimateCodeLines(4, 500, 240, 16, false, true); got <= 0 {
		t.Fatalf("code estimate = %v", got)
	}
	if got := EstimateText(0, 300, 16, 1.7, false); got != 27.2 {
		t.Fatalf("empty prose estimate = %v, want 27.2", got)
	}
	if got := EstimateTableRow(500, 3, 300, 16, 1.7); !near(got, 1432) {
		t.Fatalf("table row estimate = %v, want about 1432", got)
	}
}
