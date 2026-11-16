package navigation

import "sort"

// BigWordStart returns a Vim WORD range strictly after or before byteOffset.
// The caller owns construction and lifetime of the ordered, non-overlapping
// source ranges.
func BigWordStart(ranges []Range, byteOffset int, direction Direction) (Range, bool) {
	if direction > 0 {
		i := sort.Search(len(ranges), func(i int) bool { return ranges[i].End > byteOffset })
		if i < len(ranges) && ranges[i].Start <= byteOffset {
			i++
		}
		if i < len(ranges) {
			return ranges[i], true
		}
		return Range{}, false
	}
	i := sort.Search(len(ranges), func(i int) bool { return ranges[i].Start >= byteOffset })
	if i > 0 {
		return ranges[i-1], true
	}
	return Range{}, false
}

// BigWordEnd returns the current or next Vim WORD range by its exclusive
// display-byte end. When next is true, a range containing byteOffset is
// skipped; gaps naturally resolve to the next range.
func BigWordEnd(ranges []Range, byteOffset int, next bool) (Range, bool) {
	i := sort.Search(len(ranges), func(i int) bool { return ranges[i].End > byteOffset })
	if next && i < len(ranges) && ranges[i].Start <= byteOffset {
		i++
	}
	if i < len(ranges) {
		return ranges[i], true
	}
	return Range{}, false
}
