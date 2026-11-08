package mdedit

import (
	"bytes"
	"slices"
)

// editMask advances the parser-facing source across one edit. mask is the
// masked source before the edit, src the source after it, and from/to the
// replaced range in the source before it. The mask keeps the source's
// length and line breaks, and what it changes on a line depends on that
// line alone, so an edit only needs the lines it touches masked again;
// the rest of the document is kept as it was.
//
// It returns the updated mask, the range the parser must replace, the
// replacement text, and whether the edit could be applied this way. ok is
// false when an edit can change the front matter, because front matter is
// blanked in the mask and a change to it is not line-independent.
func editMask(mask []byte, src string, from, to, fm int, cmts []cmt) (next []byte, mFrom, mTo int, repl string, ok bool) {
	if from < 0 || from > to || to > len(mask) || len(src) < len(mask)-(to-from) {
		return nil, 0, 0, "", false
	}
	// The first bytes decide whether the document has front matter. An
	// edit there can add or remove it, so the mask starts over.
	if from == 0 {
		return nil, 0, 0, "", false
	}
	// Front matter is blanked in the mask. An edit inside it, or one that
	// reaches across its end, has to rebuild the mask.
	if fm > 0 && (from <= fm || to <= fm) {
		return nil, 0, 0, "", false
	}
	// The lines the edit replaces in the mask, and the lines those become
	// in the new source. The edit starts and ends inside them, so masking
	// whole lines keeps the parser's offsets on line boundaries.
	ls := lineStartBytes(mask, from)
	le := lineEndBytes(mask, to)
	// The text inserted has length (len(src)-len(mask)) + (to-from); it
	// ends at end, which is where the new lines are looked up.
	end := to + len(src) - len(mask)
	if end < from || end > len(src) {
		return nil, 0, 0, "", false
	}
	nls := lineStartAt(src, from)
	nle := lineEndAt(src, end)
	// The lines the parser reads again are masked again: a comment's bytes
	// are spaces to it, and the no-break space that keeps a comment opening
	// a line from indenting what follows is part of the mask too.
	repl = parserText(maskLines(src, nls, nle, cmts))
	next = slices.Replace(mask, ls, le, []byte(repl)...)
	return next, ls, le, repl, true
}

// lineStartBytes is lineStartAt for a byte slice: the offset of the start
// of the line holding off.
func lineStartBytes(b []byte, off int) int {
	off = max(0, min(off, len(b)))
	return bytes.LastIndexByte(b[:off], '\n') + 1
}

// lineEndBytes is lineEndAt for a byte slice: the offset of the end of the
// line holding off, before its newline.
func lineEndBytes(b []byte, off int) int {
	off = max(0, min(off, len(b)))
	if i := bytes.IndexByte(b[off:], '\n'); i >= 0 {
		return off + i
	}
	return len(b)
}
