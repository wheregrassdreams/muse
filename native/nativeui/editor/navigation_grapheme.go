package editor

import textnav "github.com/wheregrassdreams/muse-editor/text/navigation"

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
