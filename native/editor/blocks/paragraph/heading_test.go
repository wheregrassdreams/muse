package paragraph

import (
	"reflect"
	"testing"
)

func TestPlanHeading(t *testing.T) {
	tests := []struct {
		name  string
		input HeadingInput
		want  HeadingPlan
	}{
		{
			name:  "paragraph becomes heading after continuation prefix",
			input: HeadingInput{Source: "> text", Caret: 4, Level: 2, InsertAt: 2},
			want:  HeadingPlan{Edits: []Edit{{Start: 2, End: 2, Text: "## "}}, Caret: 7},
		},
		{
			name:  "ATX level changes replace existing prefix",
			input: HeadingInput{Source: "## title", Caret: 5, Level: 3, Shape: ATXHeading, PrefixFrom: 0, PrefixTo: 3},
			want:  HeadingPlan{Edits: []Edit{{Start: 0, End: 3, Text: "### "}}, Caret: 6},
		},
		{
			name:  "setext heading removes underline and inserts ATX prefix",
			input: HeadingInput{Source: "title\n---\n", Caret: 2, Level: 1, Shape: SetextHeading, PrefixFrom: 0, SetextFrom: 5, SetextTo: 9},
			want:  HeadingPlan{Edits: []Edit{{Start: 5, End: 9}, {Start: 0, End: 0, Text: "# "}}, Caret: 4},
		},
		{
			name:  "heading level zero removes setext underline",
			input: HeadingInput{Source: "title\n---\n", Caret: 2, Shape: SetextHeading, PrefixFrom: 0, SetextFrom: 5, SetextTo: 9},
			want:  HeadingPlan{Edits: []Edit{{Start: 5, End: 9}}, Caret: 2},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := PlanHeading(test.input)
			if !ok || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("PlanHeading() = %+v, %v; want %+v", got, ok, test.want)
			}
		})
	}
}

func TestShiftHeadingLevel(t *testing.T) {
	for _, test := range []struct {
		level, direction int
		want             int
		changed          bool
	}{
		{0, 1, 6, true},
		{0, -1, 0, false},
		{1, 1, 1, false},
		{2, 1, 1, true},
		{5, -1, 6, true},
		{6, -1, 0, true},
		{6, 1, 5, true},
		{7, 1, 7, false},
		{2, 0, 2, false},
	} {
		got, changed := ShiftHeadingLevel(test.level, test.direction)
		if got != test.want || changed != test.changed {
			t.Errorf("ShiftHeadingLevel(%d, %d) = %d, %v; want %d, %v", test.level, test.direction, got, changed, test.want, test.changed)
		}
	}
}

func TestPlanHeadingRejectsInvalidRangesAndLevels(t *testing.T) {
	for _, input := range []HeadingInput{
		{Source: "x", Caret: 2, Level: 1, InsertAt: 0},
		{Source: "x", Caret: 0, Level: 7, InsertAt: 0},
		{Source: "x", Caret: 0, Level: 1, Shape: ATXHeading, PrefixFrom: 1, PrefixTo: 0},
		{Source: "甲", Caret: 1, Level: 1, InsertAt: 0},
	} {
		if _, ok := PlanHeading(input); ok {
			t.Fatalf("PlanHeading(%+v) unexpectedly succeeded", input)
		}
	}
}
