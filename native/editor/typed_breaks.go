package editor

import "strings"

// Typed breaks are view-local source anchors, not extra source characters.
// They preserve a line the user explicitly filled/created while keeping
// imported Markdown soft wrapping stable during all caret/focus movement.
func (e *Editor) rememberTypedBreak(at int) {
	e.typedBreaks.Remember(e.src, at)
}

func (e *Editor) trackBlankLineInput(text string, kind stepKind) {
	if !e.opts.PreserveTypedLineBreaks || !e.opts.SoftBreaksAsSpaces || e.anchor != e.caret || strings.TrimSpace(text) == "" {
		return
	}
	if e.phantom != nil {
		e.begin(kind)
		return
	}
	start, end := lineStartAt(e.src, e.caret), lineEndAt(e.src, e.caret)
	l := e.unitAt(start)
	if l == nil || l.kind != kBlank {
		return
	}
	e.begin(kind)
	e.rememberTypedBreak(start - 1)
	e.rememberTypedBreak(end)
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

func (e *Editor) typedBreakIn(from, to int) bool {
	if !e.opts.PreserveTypedLineBreaks {
		return false
	}
	return e.typedBreaks.Contains(from, to)
}

// Relative anchors let unaffected leaves reuse their shaped views after edits
// earlier in the document. Only a leaf with an actual boundary change reflows.
func (e *Editor) typedBreakKey(l *leaf) string {
	if !e.opts.PreserveTypedLineBreaks {
		return ""
	}
	return e.typedBreaks.Key(l.lineStart, l.end)
}
