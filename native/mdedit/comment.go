package mdedit

import "github.com/egoist/mygo/ui/markdown/parser"

// Obsidian's comment: %%…%% is hidden in the view, but stays in the file.
// The parser reads it as text, so the editor pairs the %%s itself, among
// the Text inlines of one block (a block comment may span lines), and
// hides what is between them. The caret in the comment shows it again, as
// every other construct's markers do.

// cmt is one %%…%% span, from the opening %% to the end of the closing one.
type cmt struct{ from, to int }

// comments returns the comments among the Text inlines from i to end of
// ins, siblings, in order. An unclosed %% is left as text: an editor must
// not hide the rest of a note because a marker was half typed, and a
// message that is only %%%% is a comment like any other.
func comments(src string, ins []parser.Inline, i, end int) []cmt {
	var out []cmt
	open := -1
	for k := i; k < end && k < len(ins); k = max(ins[k].Next, k+1) {
		in := &ins[k]
		if in.Kind != parser.Text {
			continue
		}
		for p := in.Start; p+1 < in.End; {
			if src[p] != '%' || src[p+1] != '%' || escapedAt(src, p) {
				p++
				continue
			}
			if open < 0 {
				open = p
			} else {
				out = append(out, cmt{from: open, to: p + 2})
				open = -1
			}
			p += 2
		}
	}
	return out
}

// commentAt returns the comment holding p, if any.
func commentAt(cmts []cmt, p int) (cmt, bool) {
	for _, c := range cmts {
		if c.from <= p && p < c.to {
			return c, true
		}
	}
	return cmt{}, false
}
