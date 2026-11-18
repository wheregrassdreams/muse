package paragraph

import (
	"strings"
)

// MarkerWidth returns the source indentation occupied by one list marker and
// its following spaces, using Markdown's four-column continuation rule.
func MarkerWidth(source string, at int) int {
	if at < 0 || at >= len(source) {
		return 0
	}
	end := at
	for end < len(source) && source[end] != '\n' && source[end] != '\r' {
		end++
	}
	p := at
	for p < end && source[p] != ' ' && source[p] != '\t' {
		p++
	}
	spaces := 0
	for p+spaces < end && source[p+spaces] == ' ' && spaces < 4 {
		spaces++
	}
	if spaces == 0 || spaces == 4 || p+spaces == end {
		spaces = 1
	}
	return p - at + spaces
}

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

// ContainerKind is a parser-derived Markdown container surrounding a
// paragraph or list item. The editor adapter translates its private parser
// containers into these values before asking for source continuation rules.
type ContainerKind uint8

const (
	ListContainer ContainerKind = iota
	QuoteContainer
)

// ContinuationPrefix returns the prefix for a fresh line in nested Markdown
// containers. List containers contribute indentation; quote containers keep
// their visible marker.
func ContinuationPrefix(containers []ContainerKind) string {
	var prefix strings.Builder
	for _, container := range containers {
		if container == QuoteContainer {
			prefix.WriteString("> ")
		} else {
			prefix.WriteString("  ")
		}
	}
	return prefix.String()
}

// BlankListMarkers converts an existing container prefix to the indentation
// of a blank continuation line, preserving blockquote markers.
func BlankListMarkers(prefix string) string {
	b := []byte(prefix)
	for i, c := range b {
		if c != '>' && c != ' ' && c != '\t' {
			b[i] = ' '
		}
	}
	return string(b)
}

// BlankParagraphPrefix keeps only the outer blockquote prefix for a blank
// paragraph that leaves a list container.
func BlankParagraphPrefix(prefix string) string {
	if i := strings.LastIndexByte(prefix, '>'); i >= 0 {
		return prefix[:i+1]
	}
	return ""
}

// List continuation and exit source edits live in muse-editor/markdown/edit.

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
