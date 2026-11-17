package paragraph

import (
	"strconv"
	"strings"
)

// NextMarker returns the source marker for the next item in the same list.
// The caller supplies parser-derived metadata and adds the block continuation
// prefix; task items restart unchecked.
func NextMarker(marker rune, ordered bool, number int, task bool) string {
	value := string(marker) + " "
	if ordered {
		value = strconv.Itoa(number+1) + string(marker) + " "
	} else if marker == 0 {
		value = "- "
	}
	if task {
		value += "[ ] "
	}
	return value
}

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

// ListContinuationInput contains parser-resolved item facts and source
// positions. The editor host owns selection/layout lookup and applies the
// returned edit as one session transaction.
type ListContinuationInput struct {
	Source       string
	Caret        int
	ItemStart    int
	ContentStart int
	ItemEnd      int
	Marker       rune
	Ordered      bool
	Number       int
	Task         bool
	Tight        bool
}

// ListContinuationPlan is one source edit and its resulting caret.
type ListContinuationPlan struct {
	Edit  Edit
	Caret int
}

// PlanListContinuation builds the next source item while preserving the
// existing container prefix, list looseness, and marker style.
func PlanListContinuation(in ListContinuationInput) (ListContinuationPlan, bool) {
	if in.ItemStart < 0 || in.ContentStart < in.ItemStart || in.ItemEnd < in.ContentStart ||
		!validEditRange(in.Source, in.ItemStart, in.ItemEnd) ||
		!validEditRange(in.Source, in.ContentStart, in.ItemEnd) ||
		in.Caret < in.ContentStart || in.Caret > in.ItemEnd || !validBoundary(in.Source, in.Caret) {
		return ListContinuationPlan{}, false
	}
	lineStart := strings.LastIndexByte(in.Source[:in.ItemStart], '\n') + 1
	outer := BlankListMarkers(in.Source[lineStart:in.ItemStart])
	marker := NextMarker(in.Marker, in.Ordered, in.Number, in.Task)
	separator := "\n"
	if !in.Tight {
		separator = "\n" + BlankParagraphPrefix(outer) + "\n"
	}
	from, to := in.Caret, in.Caret
	for from > in.ContentStart && in.Source[from-1] == ' ' {
		from--
	}
	for to < in.ItemEnd && in.Source[to] == ' ' {
		to++
	}
	edit := Edit{Start: from, End: to, Text: separator + outer + marker}
	return ListContinuationPlan{Edit: edit, Caret: from + len(edit.Text)}, true
}

type ListExitAction uint8

const (
	ListExitNoop ListExitAction = iota
	ListExitOutdent
	ListExitEmptyItem
	ListExitMarker
)

// ListExitInput contains source and parser facts needed when Enter leaves an
// item. The UI host still owns the phantom paragraph presentation.
type ListExitInput struct {
	Source                      string
	MarkerStart, ContentStart   int
	LineStart, LineEnd, ItemEnd int
	Caret                       int
	ItemDepth                   int
	HasPreviousLeaf             bool
}

// ListExitPlan describes a list exit without exposing editor or rendering
// state. Edits address the original source and are ordered right-to-left.
type ListExitPlan struct {
	Action        ListExitAction
	Edits         []Edit
	Caret         int
	AtDocumentEnd bool
}

// PlanListExit selects nested outdent, empty-item removal, or marker removal
// and computes the source transaction. The host translates ItemDepth from
// parser structure and manages any resulting phantom view.
func PlanListExit(in ListExitInput) (ListExitPlan, bool) {
	if in.ItemDepth < 1 || in.MarkerStart < 0 || in.ContentStart < in.MarkerStart ||
		in.LineStart < 0 || in.LineStart > in.MarkerStart || in.LineEnd < in.LineStart ||
		in.ItemEnd < in.ContentStart || in.LineEnd > len(in.Source) || in.ItemEnd > len(in.Source) ||
		!validEditRange(in.Source, in.MarkerStart, in.ContentStart) ||
		!validEditRange(in.Source, in.LineStart, in.LineEnd) ||
		!validBoundary(in.Source, in.Caret) {
		return ListExitPlan{}, false
	}
	if in.ItemDepth > 1 {
		return ListExitPlan{Action: ListExitOutdent, Caret: in.Caret}, true
	}
	if strings.TrimSpace(in.Source[in.ContentStart:in.ItemEnd]) == "" {
		if in.LineEnd < len(in.Source) {
			return ListExitPlan{
				Action: ListExitEmptyItem, Edits: []Edit{{Start: in.LineStart, End: in.LineEnd + 1}},
				Caret: in.LineStart,
			}, true
		}
		at := max(in.LineStart-1, 0)
		return ListExitPlan{
			Action: ListExitEmptyItem, Edits: []Edit{{Start: at, End: in.LineEnd}},
			Caret: at, AtDocumentEnd: true,
		}, true
	}

	plan := ListExitPlan{
		Action: ListExitMarker,
		Edits:  []Edit{{Start: in.MarkerStart, End: in.ContentStart}},
		Caret:  in.MarkerStart,
	}
	if in.LineStart > 0 && in.HasPreviousLeaf {
		prefix := BlankParagraphPrefix(BlankListMarkers(in.Source[in.LineStart:in.MarkerStart]))
		text := prefix + "\n"
		plan.Edits = append(plan.Edits, Edit{Start: in.LineStart, End: in.LineStart, Text: text})
		plan.Caret += len(text)
	}
	return plan, true
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
