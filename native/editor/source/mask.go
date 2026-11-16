package source

import (
	"bytes"
	"slices"
)

func editMask(mask []byte, src string, from, to, frontEnd int, comments []Comment, transform func(string) string) (next []byte, mFrom, mTo int, replacement string, ok bool) {
	if from < 0 || from > to || to > len(mask) || len(src) < len(mask)-(to-from) || from == 0 {
		return nil, 0, 0, "", false
	}
	if frontEnd > 0 && (from <= frontEnd || to <= frontEnd) {
		return nil, 0, 0, "", false
	}
	lineStart := lineStartBytes(mask, from)
	lineEnd := lineEndBytes(mask, to)
	newEnd := to + len(src) - len(mask)
	if newEnd < from || newEnd > len(src) {
		return nil, 0, 0, "", false
	}
	newLineStart, newLineEnd := lineStartAt(src, from), lineEndAt(src, newEnd)
	replacement = maskLines(src, newLineStart, newLineEnd, comments)
	if transform != nil {
		replacement = transform(replacement)
	}
	next = slices.Replace(mask, lineStart, lineEnd, []byte(replacement)...)
	return next, lineStart, lineEnd, replacement, true
}

func lineStartBytes(b []byte, off int) int {
	off = max(0, min(off, len(b)))
	return bytes.LastIndexByte(b[:off], '\n') + 1
}

func lineEndBytes(b []byte, off int) int {
	off = max(0, min(off, len(b)))
	if i := bytes.IndexByte(b[off:], '\n'); i >= 0 {
		return off + i
	}
	return len(b)
}
