package mdedit

import "strings"

// Obsidian's comment: %%…%% is hidden, but stays in the file. A comment is a
// BLOCK construct — it may span lines and wrap other markup — so the editor
// handles it before the parser sees it, in `blankComments`: the parser reads
// spaces there, which keeps the comment's markdown from becoming structure
// (a `**bold**` inside one never becomes emphasis, a blank line inside one
// never becomes a paragraph of its own). The view then hides the bytes with
// the ranges `commentRegions` returns.
//
// An unclosed %% is left as text: an editor must not hide the rest of a note
// because a marker was half typed.

// cmt is one %%…%% span, from the opening %% to the end of the closing one.
type cmt struct{ from, to int }

// commentRegions returns every comment of the source, in order, skipping the
// ones inside fenced code and escaped `\%`.
func commentRegions(src string) []cmt {
	var out []cmt
	open := -1
	inFence := false
	for ls := 0; ls <= len(src); {
		le := strings.IndexByte(src[ls:], '\n')
		if le < 0 {
			le = len(src)
		} else {
			le += ls
		}
		line := src[ls:le]
		if trim := strings.TrimLeft(line, " \t"); strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~") {
			inFence = !inFence
		} else if !inFence {
			for p := ls; p+1 < le; {
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
		if le >= len(src) {
			break
		}
		ls = le + 1
	}
	return out
}

// blankComments returns src with every comment's bytes replaced by spaces,
// its line breaks kept: what the parser reads. The length is unchanged, so
// every offset the parser reports still addresses the real source.
func blankComments(src string, cmts []cmt) string {
	if len(cmts) == 0 {
		return src
	}
	b := []byte(src)
	for _, c := range cmts {
		for i := c.from; i < c.to && i < len(b); i++ {
			if b[i] != '\n' && b[i] != '\r' {
				b[i] = ' '
			}
		}
	}
	return string(b)
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
