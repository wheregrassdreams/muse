package paragraph

import (
	"reflect"
	"testing"
)

func TestNextMarkerPreservesBulletAndResetsTask(t *testing.T) {
	for _, tc := range []struct {
		marker  rune
		ordered bool
		number  int
		task    bool
		want    string
	}{
		{'-', false, 0, false, "- "},
		{'*', false, 0, true, "* [ ] "},
		{'.', true, 9, false, "10. "},
		{')', true, 2, true, "3) [ ] "},
	} {
		if got := NextMarker(tc.marker, tc.ordered, tc.number, tc.task); got != tc.want {
			t.Errorf("NextMarker(%q, %v, %d, %v) = %q, want %q", tc.marker, tc.ordered, tc.number, tc.task, got, tc.want)
		}
	}
}

func TestMarkerWidthUsesMarkdownContinuationColumns(t *testing.T) {
	for _, tc := range []struct {
		source string
		at     int
		want   int
	}{
		{"- item\n", 0, 2},
		{"123. item", 0, 5},
		{"*    item", 0, 2},
		{"*", 0, 2},
		{"x", -1, 0},
	} {
		if got := MarkerWidth(tc.source, tc.at); got != tc.want {
			t.Errorf("MarkerWidth(%q, %d) = %d, want %d", tc.source, tc.at, got, tc.want)
		}
	}
}

func TestPlanListContinuationPreservesMarkerAndContainers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		in    ListContinuationInput
		want  Edit
		caret int
	}{
		{
			name: "bullet",
			in:   ListContinuationInput{Source: "- alpha beta", Caret: 7, ItemStart: 0, ContentStart: 2, ItemEnd: 12, Marker: '-', Tight: true},
			want: Edit{Start: 7, End: 8, Text: "\n- "}, caret: 10,
		},
		{
			name: "quote task",
			in:   ListContinuationInput{Source: "> - task", Caret: 8, ItemStart: 2, ContentStart: 4, ItemEnd: 8, Marker: '-', Task: true, Tight: true},
			want: Edit{Start: 8, End: 8, Text: "\n> - [ ] "}, caret: 17,
		},
		{
			name: "ordered loose list",
			in:   ListContinuationInput{Source: "3. alpha", Caret: 8, ItemStart: 0, ContentStart: 3, ItemEnd: 8, Marker: '.', Ordered: true, Number: 3},
			want: Edit{Start: 8, End: 8, Text: "\n\n4. "}, caret: 13,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := PlanListContinuation(tc.in)
			if !ok || got.Edit != tc.want || got.Caret != tc.caret {
				t.Fatalf("PlanListContinuation() = %+v, %v; want %+v at %d", got, ok, tc.want, tc.caret)
			}
		})
	}
}

func TestPlanListContinuationRejectsInvalidSourcePositions(t *testing.T) {
	in := ListContinuationInput{Source: "- item", Caret: 3, ItemStart: 0, ContentStart: 2, ItemEnd: 6, Marker: '-', Tight: true}
	in.Caret = 1 // inside the list marker
	if _, ok := PlanListContinuation(in); ok {
		t.Fatal("PlanListContinuation accepted a caret outside item content")
	}
	in.Caret = 3
	in.ItemEnd++
	if _, ok := PlanListContinuation(in); ok {
		t.Fatal("PlanListContinuation accepted an item range beyond the source")
	}
}

func TestContainerContinuationAndBlankPrefixes(t *testing.T) {
	if got, want := ContinuationPrefix([]ContainerKind{QuoteContainer, ListContainer, QuoteContainer}), ">   > "; got != want {
		t.Fatalf("ContinuationPrefix = %q, want %q", got, want)
	}
	if got, want := BlankListMarkers("> - [x] "), ">       "; got != want {
		t.Fatalf("BlankListMarkers = %q, want %q", got, want)
	}
	if got, want := BlankParagraphPrefix(">   >   "), ">   >"; got != want {
		t.Fatalf("BlankParagraphPrefix = %q, want %q", got, want)
	}
}

func TestPlanListExitPlansNestedEmptyAndNonemptyItems(t *testing.T) {
	t.Run("nested item requests outdent", func(t *testing.T) {
		plan, ok := PlanListExit(ListExitInput{
			Source: "- item", MarkerStart: 0, ContentStart: 2, LineStart: 0, LineEnd: 6,
			ItemEnd: 6, Caret: 6, ItemDepth: 2,
		})
		if !ok || plan.Action != ListExitOutdent || len(plan.Edits) != 0 {
			t.Fatalf("PlanListExit() = %+v, %v; want outdent without source edits", plan, ok)
		}
	})

	t.Run("empty item leaves a placeholder before following source", func(t *testing.T) {
		plan, ok := PlanListExit(ListExitInput{
			Source: "- \nnext", MarkerStart: 0, ContentStart: 2, LineStart: 0, LineEnd: 2,
			ItemEnd: 2, Caret: 2, ItemDepth: 1,
		})
		if !ok || plan.Action != ListExitEmptyItem || plan.AtDocumentEnd || plan.Caret != 0 ||
			len(plan.Edits) != 1 || plan.Edits[0] != (Edit{Start: 0, End: 3}) {
			t.Fatalf("PlanListExit() = %+v, %v; want removal of the empty list line", plan, ok)
		}
	})

	t.Run("nonempty item removes marker and inserts paragraph separation", func(t *testing.T) {
		plan, ok := PlanListExit(ListExitInput{
			Source: "- prev\n- next", MarkerStart: 7, ContentStart: 9, LineStart: 7, LineEnd: 13,
			ItemEnd: 13, Caret: 9, ItemDepth: 1, HasPreviousLeaf: true,
		})
		want := []Edit{{Start: 7, End: 9}, {Start: 7, End: 7, Text: "\n"}}
		if !ok || plan.Action != ListExitMarker || plan.Caret != 8 || !reflect.DeepEqual(plan.Edits, want) {
			t.Fatalf("PlanListExit() = %+v, %v; want edits %+v and caret 8", plan, ok, want)
		}
	})
}

func TestPlanListExitRejectsInvalidRanges(t *testing.T) {
	if _, ok := PlanListExit(ListExitInput{Source: "- item", MarkerStart: 4, ContentStart: 2, LineStart: 0, LineEnd: 6, ItemEnd: 6, Caret: 3, ItemDepth: 1}); ok {
		t.Fatal("PlanListExit accepted an inverted marker/content range")
	}
}
