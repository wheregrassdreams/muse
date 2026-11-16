package editor

import (
	"sort"

	"muse/editor/navigation"
)

// graphemeEdges is a source-only index of extended grapheme boundaries. It is
// keyed to the editor revision so selection/paint queries never rescan a long
// document for each leaf.
func (e *Editor) graphemeEdges() []int {
	if e.graphemeBoundaries != nil && e.graphemeRevision == e.Session.Revision() {
		return e.graphemeBoundaries
	}
	edges := navigation.GraphemeBoundaries(e.Source())
	e.graphemeRevision = e.Session.Revision()
	e.graphemeBoundaries = edges
	e.graphemeBuilds++
	return edges
}

func (e *Editor) nextGraphemeBoundary(off int) int {
	off = max(0, min(off, len(e.Source())))
	edges := e.graphemeEdges()
	i := sort.Search(len(edges), func(i int) bool { return edges[i] > off })
	if i == len(edges) {
		return len(e.Source())
	}
	return edges[i]
}

func (e *Editor) previousGraphemeBoundary(off int) int {
	off = max(0, min(off, len(e.Source())))
	edges := e.graphemeEdges()
	i := sort.Search(len(edges), func(i int) bool { return edges[i] >= off })
	if i == 0 {
		return 0
	}
	return edges[i-1]
}

func (e *Editor) snapGraphemeBoundary(off, dir int) int {
	off = max(0, min(off, len(e.Source())))
	edges := e.graphemeEdges()
	i := sort.Search(len(edges), func(i int) bool { return edges[i] >= off })
	if i < len(edges) && edges[i] == off {
		return off
	}
	if i == 0 {
		return 0
	}
	if i == len(edges) {
		return len(e.Source())
	}
	if dir < 0 {
		return edges[i-1]
	}
	return edges[i]
}
