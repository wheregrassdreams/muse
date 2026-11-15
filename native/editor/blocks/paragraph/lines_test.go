package paragraph

import (
	"reflect"
	"testing"
)

func TestPlanIndentLinesUsesOriginalDescendingRanges(t *testing.T) {
	plan, ok := PlanIndentLines("a\n\n  b\n", 0, 6, "> ", 0, 4, 4)
	want := LinePlan{
		Edits:  []Edit{{Start: 3, End: 3, Text: "> "}, {Start: 0, End: 0, Text: "> "}},
		Anchor: 8, Head: 8,
	}
	if !ok || !reflect.DeepEqual(plan, want) {
		t.Fatalf("PlanIndentLines() = %+v, %v; want %+v", plan, ok, want)
	}
}

func TestPlanOutdentLinesAdjustsSelectionAndKeepsDescendingRanges(t *testing.T) {
	plan, ok := PlanOutdentLines("  a\n  b", 0, 7, 2, 0, 6, 6)
	want := LinePlan{
		Edits:  []Edit{{Start: 4, End: 6}, {Start: 0, End: 2}},
		Anchor: 2, Head: 2,
	}
	if !ok || !reflect.DeepEqual(plan, want) {
		t.Fatalf("PlanOutdentLines() = %+v, %v; want %+v", plan, ok, want)
	}
}

func TestPlanUnquoteRemovesNestedQuoteMarkers(t *testing.T) {
	source := "> > first\n> > second"
	plan, ok := PlanUnquote(source, 0, len(source), 2, len(source))
	want := LinePlan{
		Edits:  []Edit{{Start: 12, End: 14}, {Start: 2, End: 4}},
		Anchor: len(source) - 4, Head: len(source) - 4,
	}
	if !ok || !reflect.DeepEqual(plan, want) {
		t.Fatalf("PlanUnquote() = %+v, %v; want %+v", plan, ok, want)
	}
}
