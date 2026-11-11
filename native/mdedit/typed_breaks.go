package mdedit

import (
	"encoding/binary"
	"sort"
	"strings"
)

// Typed breaks are view-local source anchors, not extra source characters.
// They preserve a line the user explicitly filled/created while keeping
// imported Markdown soft wrapping stable during all caret/focus movement.
func (e *Editor) rememberTypedBreak(at int) {
	if at < 0 || at >= len(e.src) || e.src[at] != '\n' {
		return
	}
	i := sort.SearchInts(e.typedBreaks, at)
	if i < len(e.typedBreaks) && e.typedBreaks[i] == at {
		return
	}
	e.typedBreaks = append(e.typedBreaks, 0)
	copy(e.typedBreaks[i+1:], e.typedBreaks[i:])
	e.typedBreaks[i] = at
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
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			e.rememberTypedBreak(at + i)
		}
	}
}

func (e *Editor) moveTypedBreaks(from, to int, text string) {
	out := e.typedBreaks[:0]
	for _, at := range e.typedBreaks {
		switch {
		case at < from:
			out = append(out, at)
		case at >= to:
			out = append(out, at+len(text)-(to-from))
		}
	}
	e.typedBreaks = out
	if e.preserveInsertedBreaks {
		e.rememberInsertedBreaks(from, text)
	}
}

func (e *Editor) typedBreakIn(from, to int) bool {
	if !e.opts.PreserveTypedLineBreaks {
		return false
	}
	i := sort.SearchInts(e.typedBreaks, from)
	return i < len(e.typedBreaks) && e.typedBreaks[i] < to
}

// Relative anchors let unaffected leaves reuse their shaped views after edits
// earlier in the document. Only a leaf with an actual boundary change reflows.
func (e *Editor) typedBreakKey(l *leaf) string {
	if !e.opts.PreserveTypedLineBreaks {
		return ""
	}
	i := sort.SearchInts(e.typedBreaks, l.lineStart)
	var key []byte
	for ; i < len(e.typedBreaks) && e.typedBreaks[i] < l.end; i++ {
		key = binary.AppendUvarint(key, uint64(e.typedBreaks[i]-l.lineStart))
	}
	return string(key)
}
