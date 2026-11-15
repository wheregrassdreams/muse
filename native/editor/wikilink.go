package mdedit

import "strings"

// Wiki links, as Obsidian writes them: [[target]], [[target|label]],
// [[target#heading]] and the embeds ![[target]]. The parser reads none of
// them — they arrive as plain Text — so the editor pairs the brackets
// itself and draws the label where the source has the whole construct, the
// way highlight.go pairs ==.
//
// The source stays what it is: only the view hides the brackets, and the
// caret at the construct shows them again, so the link can be edited like
// any other text.

// wikiLink is one [[…]]: the source range from the `[[` (or the `!` of an
// embed) to the closing `]]`, the target it opens, and the range of the
// text drawn in its place.
type wikiLink struct {
	from, to           int
	labelFrom, labelTo int
	target             string
	embed              bool
}

// wikiLinks returns the wiki links in src[from:to], in order.
//
// `[[` opens one and the next `]]` closes it; a link never crosses a line
// and an empty target is not a link, so `[[]]` and a half-typed `[[note`
// stay as written. `[[a|b]]` shows b and opens a; the pipes of a table's
// row are the table's, so a target with a `\|` keeps the escaped pipe.
func wikiLinks(src string, from, to int) []wikiLink {
	var out []wikiLink
	for p := from; p+1 < to; {
		i := strings.Index(src[p:to], "[[")
		if i < 0 {
			return out
		}
		i += p
		if i > from && src[i-1] == '\\' {
			p = i + 2
			continue
		}
		j := strings.Index(src[i+2:to], "]]")
		if j < 0 {
			return out
		}
		j += i + 2
		inner := src[i+2 : j]
		if strings.ContainsAny(inner, "\n\r") || strings.TrimSpace(inner) == "" {
			p = i + 2
			continue
		}
		start := i
		embed := i > from && src[i-1] == '!'
		if embed {
			start = i - 1
		}
		ls, le := i+2, j
		target := inner
		if k := strings.IndexByte(inner, '|'); k >= 0 {
			target = inner[:k]
			ls = i + 2 + k + 1
		}
		out = append(out, wikiLink{
			from:      start,
			to:        j + 2,
			labelFrom: ls,
			labelTo:   le,
			target:    strings.TrimSpace(strings.ReplaceAll(target, `\|`, "|")),
			embed:     embed,
		})
		p = j + 2
	}
	return out
}

// wiki links of src starting or ending at off, as linkAt finds them.
func wikiAt(src string, from, to, off int) (string, bool) {
	for _, w := range wikiLinks(src, from, to) {
		if w.from <= off && off <= w.to {
			target := strings.TrimSpace(w.target)
			if target == "" {
				return "", false
			}
			return target, true
		}
	}
	return "", false
}
