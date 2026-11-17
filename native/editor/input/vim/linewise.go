package vim

import "muse/editor/navigation"

// LinewiseOperatorRange returns the half-open source range acted on by a
// linewise operator. Count includes the caret's current line. Delete and yank
// include the selected line's trailing LF when present; deleting a final line
// also consumes its preceding LF. Change leaves the surrounding line breaks
// in place so inserted text replaces only the selected line contents.
func LinewiseOperatorRange(source string, caret, count int, operator string) navigation.Range {
	line := navigation.LineRange(source, caret)
	start, end := line.Start, line.End
	for i := 1; i < max(count, 1); i++ {
		if end < len(source) {
			end++
			end = navigation.LineEnd(source, end)
		}
	}
	if operator == "c" {
		if end > start && source[end-1] == '\n' {
			end--
		}
	} else if end < len(source) {
		end++
	} else if operator == "d" && start > 0 && start < len(source) {
		start--
	}
	return navigation.Range{Start: start, End: end}
}
