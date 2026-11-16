package inline

import (
	"reflect"
	"testing"
)

func TestWrapSelectionPlansSourceEdits(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		selection Range
		word      Range
		useWord   bool
		want      FormatPlan
	}{
		{
			name:   "collapsed caret inserts markers",
			source: "abc", selection: Range{Start: 1, End: 1},
			want: FormatPlan{Edits: []Edit{{Start: 1, End: 1, Text: "****"}}, Selection: Range{Start: 3, End: 3}},
		},
		{
			name:   "selection is wrapped",
			source: "abc", selection: Range{Start: 0, End: 3},
			want: FormatPlan{Edits: []Edit{{Start: 3, End: 3, Text: "**"}, {Start: 0, End: 0, Text: "**"}}, Selection: Range{Start: 2, End: 5}},
		},
		{
			name:   "edge spaces stay outside",
			source: " a ", selection: Range{Start: 0, End: 3},
			want: FormatPlan{Edits: []Edit{{Start: 2, End: 2, Text: "**"}, {Start: 1, End: 1, Text: "**"}}, Selection: Range{Start: 3, End: 4}},
		},
		{
			name:   "caret uses eligible word",
			source: "hello world", selection: Range{Start: 2, End: 2}, word: Range{Start: 0, End: 5}, useWord: true,
			want: FormatPlan{Edits: []Edit{{Start: 5, End: 5, Text: "**"}, {Start: 0, End: 0, Text: "**"}}, Selection: Range{Start: 2, End: 7}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := WrapSelection(test.source, test.selection, test.word, test.useWord, "**", "**")
			if !ok || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("plan = %+v, %v; want %+v", got, ok, test.want)
			}
		})
	}
}

func TestMarkerLengthsForVariableDelimiters(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		span        Range
		kind        MarkerKind
		open, close string
		wantOpen    int
		wantClose   int
	}{
		{"strong", "***word***", Range{0, 10}, MarkerStrong, "**", "**", 2, 2},
		{"code run", "``code``", Range{0, 8}, MarkerCode, "`", "`", 2, 2},
		{"strike run", "~~~word~~~", Range{0, 10}, MarkerStrikethrough, "~~", "~~", 2, 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			open, close, ok := MarkerLengths(test.source, test.span, test.kind, test.open, test.close)
			if !ok || open != test.wantOpen || close != test.wantClose {
				t.Fatalf("marker lengths = %d,%d,%v; want %d,%d,true", open, close, ok, test.wantOpen, test.wantClose)
			}
		})
	}
}

func TestUnwrapSelectionPlansDelimiterRemovalAndMapsSelection(t *testing.T) {
	for _, test := range []struct {
		name      string
		selection Range
		want      Range
	}{
		{name: "whole span", selection: Range{Start: 0, End: 9}, want: Range{Start: 0, End: 5}},
		{name: "caret in content", selection: Range{Start: 5, End: 5}, want: Range{Start: 3, End: 3}},
		{name: "partial content", selection: Range{Start: 3, End: 6}, want: Range{Start: 1, End: 4}},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan, ok := UnwrapSelection("**hello**", test.selection, Range{Start: 0, End: 9}, MarkerStrong, "**", "**")
			if !ok || plan.Selection != test.want || len(plan.Edits) != 2 ||
				plan.Edits[0] != (Edit{Start: 7, End: 9}) || plan.Edits[1] != (Edit{Start: 0, End: 2}) {
				t.Fatalf("UnwrapSelection() = %+v, %v; want selection %+v", plan, ok, test.want)
			}
		})
	}
}

func TestPlanLinkCreatesDestinationCaretOrPreservesSelectedURL(t *testing.T) {
	plan, ok := PlanLink("visit docs", Range{Start: 6, End: 10})
	if !ok || !reflect.DeepEqual(plan.Edits, []Edit{{Start: 10, End: 10, Text: "]()"}, {Start: 6, End: 6, Text: "["}}) || plan.Selection != (Range{Start: 13, End: 13}) {
		t.Fatalf("selected text link plan = %+v, %v", plan, ok)
	}
	plan, ok = PlanLink("https://example.test", Range{Start: 0, End: len("https://example.test")})
	if !ok || len(plan.Edits) != 1 || plan.Edits[0].Text != "[https://example.test](https://example.test)" ||
		plan.Selection != (Range{Start: 1, End: 21}) {
		t.Fatalf("selected URL link plan = %+v, %v", plan, ok)
	}
}

func TestWrapSelectionRejectsSplitRuneRanges(t *testing.T) {
	source := "甲乙"
	for _, selection := range []Range{{Start: 1, End: 3}, {Start: 0, End: 1}} {
		if _, ok := WrapSelection(source, selection, Range{}, false, "**", "**"); ok {
			t.Fatalf("WrapSelection(%+v) accepted a split UTF-8 rune", selection)
		}
	}
	if _, _, ok := MarkerLengths("**甲**", Range{Start: 3, End: 4}, MarkerStrong, "**", "**"); ok {
		t.Fatal("MarkerLengths accepted a split UTF-8 rune")
	}
}
