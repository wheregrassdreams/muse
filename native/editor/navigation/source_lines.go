package navigation

import "strings"

// LineRange returns the half-open source range of the line containing offset.
// Lines are separated only by LF bytes; CR and other Unicode whitespace remain
// part of the line. Offset is a UTF-8 byte offset and is clamped to the source.
func LineRange(source string, offset int) Range {
	offset = min(max(offset, 0), len(source))
	start := strings.LastIndexByte(source[:offset], '\n') + 1
	end := len(source)
	if newline := strings.IndexByte(source[offset:], '\n'); newline >= 0 {
		end = offset + newline
	}
	return Range{Start: start, End: end}
}

// LineStart returns the start byte of the line containing offset. When
// firstNonSpace is true, it advances over only ASCII spaces and tabs, stopping
// at the line end if the line contains no other byte.
func LineStart(source string, offset int, firstNonSpace bool) int {
	line := LineRange(source, offset)
	start := line.Start
	if firstNonSpace {
		for start < line.End && (source[start] == ' ' || source[start] == '\t') {
			start++
		}
	}
	return start
}

// LineEnd returns the exclusive end byte of the line containing offset.
func LineEnd(source string, offset int) int {
	return LineRange(source, offset).End
}
