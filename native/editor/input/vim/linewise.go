package vim

import "muse/editor/navigation"

// LinewiseOperatorRange returns the half-open source range selected by a
// linewise operator. Count includes the caret's current line; repeated lines
// include the LF between them. The change operator trims a trailing LF when
// the selected range ends immediately after one.
func LinewiseOperatorRange(source string, caret, count int, operator string) navigation.Range {
	line := navigation.LineRange(source, caret)
	start, end := line.Start, line.End
	for i := 1; i < max(count, 1); i++ {
		if end < len(source) {
			end++
			end = navigation.LineEnd(source, end)
		}
	}
	if operator == "c" && end > start && source[end-1] == '\n' {
		end--
	}
	return navigation.Range{Start: start, End: end}
}
