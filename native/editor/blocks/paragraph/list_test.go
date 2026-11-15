package paragraph

import "testing"

func TestPlanListToggle(t *testing.T) {
	tests := []struct {
		name  string
		input ListInput
		want  ListPlan
	}{
		{
			name:  "same marker asks host to leave list",
			input: ListInput{Source: "- text", Caret: 4, Marker: "-", Item: true, MarkerStart: 0, ContentStart: 2},
			want:  ListPlan{Action: ListLeave, Caret: 4},
		},
		{
			name:  "item marker changes as one source edit",
			input: ListInput{Source: "- [ ] text", Caret: 8, Marker: "1.", Item: true, Task: true, MarkerStart: 0, ContentStart: 6},
			want:  ListPlan{Action: ListEdit, Edit: Edit{Start: 0, End: 6, Text: "1. "}, Caret: 5},
		},
		{
			name:  "paragraph receives marker at parser offset",
			input: ListInput{Source: "> text", Caret: 4, Marker: "-", EligibleText: true, InsertAt: 2},
			want:  ListPlan{Action: ListEdit, Edit: Edit{Start: 2, End: 2, Text: "- "}, Caret: 6},
		},
		{
			name:  "ineligible block is unchanged",
			input: ListInput{Source: "text", Caret: 2, Marker: "-"},
			want:  ListPlan{Action: ListNoop, Caret: 2},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := PlanListToggle(test.input)
			if !ok || got != test.want {
				t.Fatalf("PlanListToggle() = %+v, %v; want %+v", got, ok, test.want)
			}
		})
	}
}

func TestPlanListToggleRejectsInvalidRanges(t *testing.T) {
	for _, input := range []ListInput{
		{Source: "x", Caret: 2, Marker: "-", EligibleText: true, InsertAt: 0},
		{Source: "x", Caret: 0, Marker: "-", Item: true, MarkerStart: 1, ContentStart: 0},
		{Source: "x", Caret: 0, Marker: ""},
	} {
		if _, ok := PlanListToggle(input); ok {
			t.Fatalf("PlanListToggle(%+v) unexpectedly succeeded", input)
		}
	}
}
