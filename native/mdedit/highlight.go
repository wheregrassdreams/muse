package mdedit

import (
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/markdown/parser"
)

// Highlight, as Typora writes it: text between == and == is marked. The
// parser does not read it, so the editor pairs the ==s among the text of
// the inlines next to each other, as it pairs the tildes of
// strikethrough: one opens before text and closes after it, and not
// inside a word of Latin letters and digits, so that a==b && c==d marks
// nothing.

// hlMark is an == of highlight: its offset, whether it opens, and the
// range of its pair, from the opener's start to the closer's end.
type hlMark struct {
	at       int
	open     bool
	from, to int
}

// highlights returns the ==s that highlight among the Text inlines from i
// to end of ins, siblings, in order: each opener pairs with the next ==
// that closes.
func highlights(src string, ins []parser.Inline, i, end int) []hlMark {
	var out []hlMark
	pending := -1
	for k := i; k < end && k < len(ins); k = max(ins[k].Next, k+1) {
		in := &ins[k]
		if in.Kind != parser.Text {
			continue
		}
		for p := in.Start; p < in.End; {
			if src[p] != '=' {
				p++
				continue
			}
			q := p
			for q < in.End && src[q] == '=' {
				q++
			}
			if q-p == 2 && !escapedAt(src, p) {
				before, after := ' ', ' '
				if p > 0 {
					before, _ = utf8.DecodeLastRuneInString(src[:p])
				}
				if q < len(src) {
					after, _ = utf8.DecodeRuneInString(src[q:])
				}
				canOpen := !unicode.IsSpace(after) && !isLatinAlnum(before)
				canClose := !unicode.IsSpace(before) && !isLatinAlnum(after)
				switch {
				case pending >= 0 && canClose:
					out = append(out, hlMark{at: pending, open: true, from: pending, to: q}, hlMark{at: p, from: pending, to: q})
					pending = -1
				case canOpen:
					pending = p
				}
			}
			p = q
		}
	}
	return out
}

// allHighlights returns the ==s that highlight among all the inlines from
// i to end, at every depth.
func allHighlights(src string, ins []parser.Inline, i, end int) []hlMark {
	out := highlights(src, ins, i, end)
	for k := i; k < end && k < len(ins); k = max(ins[k].Next, k+1) {
		if ins[k].Next > k+1 {
			out = append(out, allHighlights(src, ins, k+1, ins[k].Next)...)
		}
	}
	return out
}

func isLatinAlnum(r rune) bool {
	return r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

// ToggleHighlight marks the selection with ==, or takes the marks away
// from the highlight it is in.
func (e *Editor) ToggleHighlight() {
	e.layoutIfNeeded()
	a, z := e.selection()
	if u := e.unitAt(e.caret); u != nil && u.id >= 0 && u.isText() {
		ins := e.p.doc.Inlines(u.id)
		best := hlMark{from: -1}
		for _, m := range allHighlights(e.src, ins, 0, len(ins)) {
			if m.open && m.from <= a && z <= m.to && (best.from < 0 || m.to-m.from < best.to-best.from) {
				best = m
			}
		}
		if best.from >= 0 {
			e.begin(stepEdit)
			e.replace(best.to-2, best.to, "")
			e.replace(best.from, best.from+2, "")
			e.commit(max(a-2, best.from), max(min(z, best.to-2)-2, best.from))
			return
		}
	}
	e.wrapInline("==", "==")
}
