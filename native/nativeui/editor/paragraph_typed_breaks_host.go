package editor

import "github.com/wheregrassdreams/muse-editor/markdown/paragraph"

// Typed breaks are view-local source anchors, not extra source characters.
// They preserve a line the user explicitly filled/created while keeping
// imported Markdown soft wrapping stable during all caret/focus movement.
func (e *Editor) rememberTypedBreak(at int) {
	e.typedBreaks.Remember(e.Source(), at)
}

func (e *Editor) trackBlankLineInput(text string, kind stepKind) {
	input := paragraph.BlankInput{
		Enabled:            e.opts.PreserveTypedLineBreaks,
		CollapseSoftBreaks: e.opts.SoftBreaksAsSpaces,
		CollapsedSelection: e.Session.Selection().Anchor == e.Caret(),
		PhantomParagraph:   e.phantom != nil,
		Source:             e.Source(),
		Caret:              e.Caret(),
		Text:               text,
	}
	// Avoid forcing a layout/unit lookup for ordinary input. Policy still lives
	// in paragraph; this is only a cheap gate before supplying its parser fact.
	if !paragraph.ShouldProcessInput(input) {
		return
	}
	blankLine := false
	if e.phantom == nil {
		start := lineStartAt(e.Source(), e.Caret())
		l := e.unitAt(start)
		blankLine = l != nil && l.kind == kBlank
	}
	input.BlankSourceLine = blankLine
	plan := paragraph.PlanBlankInput(input)
	if !plan.BeginHistory {
		return
	}
	e.begin(kind)
	for _, at := range plan.RememberBreaks {
		e.rememberTypedBreak(at)
	}
}

func (e *Editor) rememberInsertedBreaks(at int, text string) {
	if !e.opts.PreserveTypedLineBreaks {
		return
	}
	e.typedBreaks.RememberInserted(at, text)
}

func (e *Editor) moveTypedBreaks(from, to int, text string) {
	e.typedBreaks.ApplyEdit(from, to, text, e.preserveInsertedBreaks)
}

// HasTypedBreak is the narrow paragraph UI capability for determining whether
// a rendered source break was explicitly introduced or preserved by editing.
func (e *Editor) HasTypedBreak(from, to int) bool {
	if !e.opts.PreserveTypedLineBreaks {
		return false
	}
	return e.typedBreaks.Contains(from, to)
}

// TypedBreakCacheKey returns the relative break projection used in a paragraph
// view's cache identity.
func (e *Editor) TypedBreakCacheKey(lineStart, lineEnd int) string {
	if !e.opts.PreserveTypedLineBreaks {
		return ""
	}
	return e.typedBreaks.Key(lineStart, lineEnd)
}
