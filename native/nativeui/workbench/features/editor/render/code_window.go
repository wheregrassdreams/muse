package render

import "fmt"

const (
	CodeLineBudgetBytes  = 4 << 10
	CodeBlockBudgetBytes = 16 << 10
)

// CodeLine is a source-backed code line. End is exclusive and ByteLength
// counts the line's content bytes, excluding any line break.
type CodeLine struct {
	Start      int
	End        int
	ByteLength int
}

// CodeRange is a half-open source interval selected for shaping.
type CodeRange struct {
	Start int
	End   int
}

// CodeWindowResult is a compact immutable value containing at most two source
// intervals. It avoids exposing a mutable slice across the render boundary.
type CodeWindowResult struct {
	first, second CodeRange
	count         int
	compact       bool
}

func (w CodeWindowResult) RangeCount() int { return w.count }

func (w CodeWindowResult) RangeAt(index int) (CodeRange, bool) {
	switch index {
	case 0:
		if w.count > 0 {
			return w.first, true
		}
	case 1:
		if w.count > 1 {
			return w.second, true
		}
	}
	return CodeRange{}, false
}

func (w CodeWindowResult) Compact() bool { return w.compact }

// CodeWindow chooses the source intervals to shape for a code block. A
// compact block is centered on a caret inside the block; without an in-block
// caret it retains equal budgets from both ends.
func CodeWindow(start, end, caret, maxBytes int) CodeWindowResult {
	if end <= start || maxBytes <= 0 || end-start <= maxBytes {
		return CodeWindowResult{first: CodeRange{Start: start, End: end}, count: 1}
	}
	half := maxBytes / 2
	if caret >= start && caret <= end {
		windowStart := max(start, caret-half)
		windowEnd := min(end, windowStart+maxBytes)
		windowStart = max(start, windowEnd-maxBytes)
		return CodeWindowResult{first: CodeRange{Start: windowStart, End: windowEnd}, count: 1, compact: true}
	}
	return CodeWindowResult{
		first:  CodeRange{Start: start, End: start + half},
		second: CodeRange{Start: end - half, End: end},
		count:  2, compact: true,
	}
}

// CodeLineInRanges reports whether a source line overlaps one of the shaping
// intervals. Empty lines are included when their source position is inside an
// interval, matching the editor's previous preview behavior.
func CodeLineInRanges(line CodeLine, window CodeWindowResult) bool {
	for i := 0; i < window.count; i++ {
		r, _ := window.RangeAt(i)
		if line.Start < r.End && line.End > r.Start ||
			line.ByteLength == 0 && line.Start >= r.Start && line.Start < r.End {
			return true
		}
	}
	return false
}

// CodeOmittedLabel formats the source-byte count represented by an omitted
// code interval.
func CodeOmittedLabel(byteCount int) string {
	return fmt.Sprintf("… %d source bytes omitted …", byteCount)
}
