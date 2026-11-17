package table

import "strings"

// ShortcutPlan is the source edit for promoting a single pipe row into a
// Markdown table. Caret is relative to the insertion start.
type ShortcutPlan struct {
	Insert string
	Caret  int
}

// PlanShortcut creates the delimiter and empty body rows after a single-line
// pipe row. The caller supplies the block continuation prefix and applies the
// resulting edit as one editor transaction.
func PlanShortcut(row, continuation string) (ShortcutPlan, bool) {
	text := strings.TrimSpace(row)
	if strings.ContainsAny(text, "\r\n") || len(text) < 3 || text[0] != '|' || text[len(text)-1] != '|' {
		return ShortcutPlan{}, false
	}
	cells := splitShortcutRow(text)
	if len(cells) == 0 {
		return ShortcutPlan{}, false
	}
	delim, blank := "|", "|"
	for range cells {
		delim += " --- |"
		blank += "   |"
	}
	insert := "\n" + continuation + delim + "\n" + continuation + blank
	return ShortcutPlan{Insert: insert, Caret: len(insert) - len(blank) + 2}, true
}

func splitShortcutRow(row string) []string {
	var cells []string
	start := 1
	escaped := false
	for i := 1; i < len(row)-1; i++ {
		if escaped {
			escaped = false
			continue
		}
		switch row[i] {
		case '\\':
			escaped = true
		case '|':
			cells = append(cells, row[start:i])
			start = i + 1
		}
	}
	cells = append(cells, row[start:len(row)-1])
	return cells
}
