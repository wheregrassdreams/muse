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
