package mdedit

import (
	"unicode"
	"unicode/utf8"
)

// #tag, as Obsidian writes it: a `#` that starts a word, then its name —
// letters, digits, `_`, and `-` or `/` for nesting. A `#` inside a word is
// not a tag (`a#b`), and neither is a lone one. A heading's own `#` never
// reaches here: the parser takes it, and the leaf draws it as chrome.

// tagAt returns where the tag starting at p ends, or -1 when no tag starts
// there. src[s:e] is the text inline being drawn, so that the character
// before the `#` can be checked.
func tagAt(src string, p, s, e int) int {
	if p >= e || src[p] != '#' {
		return -1
	}
	if p > s {
		prev, _ := utf8.DecodeLastRuneInString(src[s:p])
		if !unicode.IsSpace(prev) {
			return -1
		}
	}
	q := p + 1
	for q < e {
		r, n := utf8.DecodeRuneInString(src[q:e])
		if !tagNameRune(r) || (q == p+1 && !tagFirstRune(r)) {
			break
		}
		q += n
	}
	if q == p+1 {
		return -1
	}
	return q
}

// tagFirstRune is what a tag's name may start with: a letter, a digit or an
// underscore, never a separator, so `##` and `#-` stay as written.
func tagFirstRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// tagNameRune is what a tag's name is made of.
func tagNameRune(r rune) bool {
	return tagFirstRune(r) || r == '-' || r == '/'
}
