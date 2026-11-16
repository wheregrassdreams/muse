package inline

import (
	"unicode"
	"unicode/utf8"
)

// TagAt returns the byte offset just after a tag beginning at at, or -1 when
// no tag starts there. The input range is half-open UTF-8 source coordinates.
func TagAt(source string, at, start, end int) int {
	if start < 0 || at < start || at >= end || end > len(source) || source[at] != '#' {
		return -1
	}
	if at > start {
		prev, _ := utf8.DecodeLastRuneInString(source[start:at])
		if !unicode.IsSpace(prev) {
			return -1
		}
	}
	if at+1 >= end {
		return -1
	}
	first, size := utf8.DecodeRuneInString(source[at+1 : end])
	if !tagFirstRune(first) {
		return -1
	}
	endOffset := at + 1 + size
	for endOffset < end {
		r, size := utf8.DecodeRuneInString(source[endOffset:end])
		if !tagNameRune(r) {
			break
		}
		endOffset += size
	}
	return endOffset
}

func tagFirstRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

func tagNameRune(r rune) bool { return tagFirstRune(r) || r == '-' || r == '/' }
