package paragraph

import "strings"

// LinePlan contains source edits and the selection after applying them.
// Edits are sorted from right to left and use the original source offsets.
type LinePlan struct {
	Edits  []Edit
	Anchor int
	Head   int
}

// PlanIndentLines inserts pad at column col of each nonblank line in [start,end].
func PlanIndentLines(source string, start, end int, pad string, col int, anchor, head int) (LinePlan, bool) {
	if !validEditRange(source, start, end) || pad == "" || !validBoundary(source, anchor) || !validBoundary(source, head) {
		return LinePlan{}, false
	}
	plan := LinePlan{Anchor: anchor, Head: head}
	for _, line := range lineStarts(source, start, end) {
		lineEnd := sourceLineEnd(source, line)
		if strings.TrimSpace(source[line:lineEnd]) == "" {
			continue
		}
		at := min(line+max(col, 0), lineEnd)
		plan.Edits = append(plan.Edits, Edit{Start: at, End: at, Text: pad})
		if anchor >= at {
			plan.Anchor += len(pad)
		}
		if head >= at {
			plan.Head += len(pad)
		}
	}
	reverseEdits(plan.Edits)
	return plan, true
}

// PlanOutdentLines removes up to count spaces at column col from each line.
func PlanOutdentLines(source string, start, end, count, col int, anchor, head int) (LinePlan, bool) {
	if !validEditRange(source, start, end) || count < 0 || !validBoundary(source, anchor) || !validBoundary(source, head) {
		return LinePlan{}, false
	}
	plan := LinePlan{Anchor: anchor, Head: head}
	for _, line := range lineStarts(source, start, end) {
		lineEnd := sourceLineEnd(source, line)
		at := min(line+max(col, 0), lineEnd)
		remove := 0
		for remove < count && at+remove < lineEnd && source[at+remove] == ' ' {
			remove++
		}
		if remove == 0 {
			continue
		}
		plan.Edits = append(plan.Edits, Edit{Start: at, End: at + remove})
		if anchor > at {
			plan.Anchor -= min(remove, anchor-at)
		}
		if head > at {
			plan.Head -= min(remove, head-at)
		}
	}
	reverseEdits(plan.Edits)
	return plan, true
}

// PlanUnquote removes the depth-th quote marker from each line in a quoted
// block. It preserves the caller's existing unquote behavior of collapsing
// the selection to the adjusted head.
func PlanUnquote(source string, start, end, depth, head int) (LinePlan, bool) {
	if !validEditRange(source, start, end) || depth <= 0 || !validBoundary(source, head) {
		return LinePlan{}, false
	}
	plan := LinePlan{Anchor: head, Head: head}
	starts := lineStarts(source, max(start, 0), end)
	for i := len(starts) - 1; i >= 0; i-- {
		line := starts[i]
		lineEnd := sourceLineEnd(source, line)
		seen, marker := 0, -1
		for at := line; at < lineEnd; at++ {
			if source[at] == '>' {
				seen++
				if seen == depth {
					marker = at
					break
				}
			} else if source[at] != ' ' && source[at] != '\t' {
				break
			}
		}
		if marker < 0 {
			continue
		}
		remove := 1
		if marker+1 < lineEnd && source[marker+1] == ' ' {
			remove = 2
		}
		plan.Edits = append(plan.Edits, Edit{Start: marker, End: marker + remove})
		if plan.Head > marker {
			plan.Head -= min(remove, plan.Head-marker)
		}
		plan.Anchor = plan.Head
	}
	return plan, true
}

func lineStarts(source string, start, end int) []int {
	start = min(max(start, 0), len(source))
	end = min(max(end, start), len(source))
	line := strings.LastIndexByte(source[:start], '\n') + 1
	starts := make([]int, 0, 4)
	for line <= end {
		starts = append(starts, line)
		lineEnd := sourceLineEnd(source, line)
		if lineEnd >= end || lineEnd >= len(source) {
			break
		}
		line = lineEnd + 1
	}
	return starts
}

func sourceLineEnd(source string, at int) int {
	if at < 0 {
		at = 0
	}
	if at > len(source) {
		at = len(source)
	}
	if n := strings.IndexByte(source[at:], '\n'); n >= 0 {
		return at + n
	}
	return len(source)
}

func reverseEdits(edits []Edit) {
	for left, right := 0, len(edits)-1; left < right; left, right = left+1, right-1 {
		edits[left], edits[right] = edits[right], edits[left]
	}
}
