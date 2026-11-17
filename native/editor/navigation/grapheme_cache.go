package navigation

import "sort"

// GraphemeBoundaryCache retains one source revision's grapheme index for
// repeated movement and selection queries. The cache is owned by an editor
// session; callers provide its monotonically increasing source revision.
type GraphemeBoundaryCache struct {
	revision   uint64
	boundaries []int
	builds     uint64
}

// Edges returns the cached extended-grapheme boundaries for source. The
// returned slice is read-only and remains owned by the cache.
func (c *GraphemeBoundaryCache) Edges(source string, revision uint64) []int {
	if c.boundaries != nil && c.revision == revision {
		return c.boundaries
	}
	c.boundaries = GraphemeBoundaries(source)
	c.revision = revision
	c.builds++
	return c.boundaries
}

// BuildCount reports how many source revisions have been indexed.
func (c *GraphemeBoundaryCache) BuildCount() uint64 { return c.builds }

// NextBoundary returns the first indexed boundary strictly after offset.
func NextBoundary(boundaries []int, sourceLength, offset int) int {
	offset = min(max(offset, 0), sourceLength)
	i := sort.Search(len(boundaries), func(i int) bool { return boundaries[i] > offset })
	if i == len(boundaries) {
		return sourceLength
	}
	return boundaries[i]
}

// PreviousBoundary returns the last indexed boundary strictly before offset.
func PreviousBoundary(boundaries []int, sourceLength, offset int) int {
	offset = min(max(offset, 0), sourceLength)
	i := sort.Search(len(boundaries), func(i int) bool { return boundaries[i] >= offset })
	if i == 0 {
		return 0
	}
	return boundaries[i-1]
}

// SnapBoundary snaps offset to the previous boundary for negative direction,
// or the next boundary otherwise.
func SnapBoundary(boundaries []int, sourceLength, offset, direction int) int {
	offset = min(max(offset, 0), sourceLength)
	i := sort.Search(len(boundaries), func(i int) bool { return boundaries[i] >= offset })
	if i < len(boundaries) && boundaries[i] == offset {
		return offset
	}
	if i == 0 {
		return 0
	}
	if i == len(boundaries) || direction >= 0 {
		return boundaries[min(i, len(boundaries)-1)]
	}
	return boundaries[i-1]
}
