package mdedit

import (
	"sort"

	"github.com/go-text/typesetting/segmenter"
)

// graphemeEdges is a source-only index of extended grapheme boundaries. It is
// keyed to the editor revision so selection/paint queries never rescan a long
// document for each leaf.
func (e *Editor) graphemeEdges() []int {
	if e.graphemeBoundaries != nil && e.graphemeRevision == e.rev {
		return e.graphemeBoundaries
	}
	var seg segmenter.Segmenter
	_ = seg.InitWithBytes([]byte(e.src))
	edges := []int{0}
	it := seg.GraphemeIterator()
	for it.Next() {
		g := it.Grapheme()
		end := g.OffsetInBytes + g.LengthInBytes
		if end > edges[len(edges)-1] {
			edges = append(edges, end)
		}
	}
	if edges[len(edges)-1] != len(e.src) {
		edges = append(edges, len(e.src))
	}
	e.graphemeRevision = e.rev
	e.graphemeBoundaries = edges
	e.graphemeBuilds++
	return edges
}

func (e *Editor) nextGraphemeBoundary(off int) int {
	off = max(0, min(off, len(e.src)))
	edges := e.graphemeEdges()
	i := sort.Search(len(edges), func(i int) bool { return edges[i] > off })
	if i == len(edges) {
		return len(e.src)
	}
	return edges[i]
}

func (e *Editor) previousGraphemeBoundary(off int) int {
	off = max(0, min(off, len(e.src)))
	edges := e.graphemeEdges()
	i := sort.Search(len(edges), func(i int) bool { return edges[i] >= off })
	if i == 0 {
		return 0
	}
	return edges[i-1]
}

func (e *Editor) snapGraphemeBoundary(off, dir int) int {
	off = max(0, min(off, len(e.src)))
	edges := e.graphemeEdges()
	i := sort.Search(len(edges), func(i int) bool { return edges[i] >= off })
	if i < len(edges) && edges[i] == off {
		return off
	}
	if i == 0 {
		return 0
	}
	if i == len(edges) {
		return len(e.src)
	}
	if dir < 0 {
		return edges[i-1]
	}
	return edges[i]
}
