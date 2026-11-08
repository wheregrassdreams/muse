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

// maskLines returns the bytes of src in [from, to) as the parser reads
// them: the part every comment has in the range blanked, its line breaks
// kept, and the no-break space that keeps a comment opening a line from
// indenting what follows it. The range must start and end at a line.
func maskLines(src string, from, to int, cmts []cmt) string {
	to = min(to, len(src))
	if from < 0 || to <= from {
		return ""
	}
	b := []byte(src[from:to])
	for _, c := range cmts {
		if c.to <= from || c.from >= to {
			continue
		}
		cf, ct := max(c.from-from, 0), min(c.to-from, len(b))
		for i := cf; i < ct; i++ {
			if b[i] != '\n' && b[i] != '\r' {
				b[i] = ' '
			}
		}
		// Only a comment that starts in the range gets the no-break space:
		// one that started above it was masked, or not, with its own line.
		if c.from >= from && cf+2 <= ct && opensContent(b, cf) && hasTextOnLine(b, ct) {
			b[cf], b[cf+1] = 0xC2, 0xA0
		}
	}
	return string(b)
}

// blankComments returns src with every comment's bytes replaced by spaces,
// its line breaks kept: what the parser reads. The length is unchanged, so
// every offset the parser reports still addresses the real source.
//
// This is maskLines over the whole source, and maskLines is how the lines
// an edit gives the parser again are masked: one rule for the whole source
// and for the part of it an edit rewrites.
func blankComments(src string, cmts []cmt) string {
	if len(cmts) == 0 {
		return src
	}
	return maskLines(src, 0, len(src), cmts)
}

// opensContent reports whether the comment at at opens the content of its
// block: only the markers of the blocks around it — a quotation's `>`, an
// item's bullet or number, a task's box, a footnote's label — and spaces
// stand before it on the line. A comment there must not leave a run of
// spaces as wide as itself, since four spaces of indentation are a block of
// code to the parser; one in the middle of the text cannot, and keeps its
// spaces.
func opensContent(b []byte, at int) bool {
	for i := at - 1; i >= 0; i-- {
		switch ch := b[i]; {
		case ch == '\n':
			return true
		case ch == ' ' || ch == '\t':
		case ch == '>' || ch == '#' || ch == '-' || ch == '*' || ch == '+' ||
			ch == '.' || ch == ')' || ch == '[' || ch == ']' || ch == '^' ||
			ch == ':' || ch == '!' || ch == '|' || ch == '~':
		case ch >= '0' && ch <= '9':
		default:
			return false
		}
	}
	return true
}

// hasTextOnLine reports whether anything but spaces and tabs stands
// between at and the end of its line.
func hasTextOnLine(b []byte, at int) bool {
	for i := at; i < len(b) && b[i] != '\n'; i++ {
		if b[i] != ' ' && b[i] != '\t' && b[i] != '\r' {
			return true
		}
	}
	return false
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
