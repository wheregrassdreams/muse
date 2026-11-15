package paragraph

import "strings"

type ListAction uint8

const (
	ListNoop ListAction = iota
	ListLeave
	ListEdit
)

type ListInput struct {
	Source       string
	Caret        int
	Marker       string
	Item         bool
	Ordered      bool
	Task         bool
	EligibleText bool
	MarkerStart  int
	ContentStart int
	InsertAt     int
}

type ListPlan struct {
	Action ListAction
	Edit   Edit
	Caret  int
}

// PlanListToggle decides whether a list item should be left, have its marker
// changed, or whether a paragraph should receive a new list marker.
func PlanListToggle(in ListInput) (ListPlan, bool) {
	if in.Marker == "" || !validBoundary(in.Source, in.Caret) {
		return ListPlan{}, false
	}
	if in.Item {
		if !validEditRange(in.Source, in.MarkerStart, in.ContentStart) {
			return ListPlan{}, false
		}
		wantOrdered := strings.HasSuffix(in.Marker, ".")
		wantTask := strings.Contains(in.Marker, "[")
		if in.Ordered == wantOrdered && in.Task == wantTask {
			return ListPlan{Action: ListLeave, Caret: in.Caret}, true
		}
		return ListPlan{
			Action: ListEdit,
			Edit:   Edit{Start: in.MarkerStart, End: in.ContentStart, Text: in.Marker + " "},
			Caret:  in.Caret + len(in.Marker) + 1 - (in.ContentStart - in.MarkerStart),
		}, true
	}
	if !in.EligibleText {
		return ListPlan{Action: ListNoop, Caret: in.Caret}, true
	}
	if !validBoundary(in.Source, in.InsertAt) {
		return ListPlan{}, false
	}
	return ListPlan{
		Action: ListEdit,
		Edit:   Edit{Start: in.InsertAt, End: in.InsertAt, Text: in.Marker + " "},
		Caret:  in.Caret + len(in.Marker) + 1,
	}, true
}
