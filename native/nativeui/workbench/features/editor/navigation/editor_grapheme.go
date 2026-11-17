package editor

import textnav "muse/editor/navigation"

// graphemeEdges returns the revision-owned source boundary index used by
// editing interactions. Boundary construction and indexed movement live in
// the backend-neutral navigation module.
func (e *Editor) graphemeEdges() []int {
	return e.graphemeCache.Edges(e.Source(), e.Session.Revision())
}

func (e *Editor) nextGraphemeBoundary(off int) int {
	return textnav.NextBoundary(e.graphemeEdges(), len(e.Source()), off)
}

func (e *Editor) previousGraphemeBoundary(off int) int {
	return textnav.PreviousBoundary(e.graphemeEdges(), len(e.Source()), off)
}

func (e *Editor) snapGraphemeBoundary(off, dir int) int {
	return textnav.SnapBoundary(e.graphemeEdges(), len(e.Source()), off, dir)
}
