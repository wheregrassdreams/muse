package editor

import (
	"image/color"
	"sort"
	"strings"

	"muse/editor/layout"
	"muse/nativeui/editor/navigationrail"
)

// railEdge is how near the document's start or end the view must be for the
// first or last heading to be the current one.
const railEdge = float32(56)

// HeadingRailSnapshot returns the detached heading list and active section
// for a navigation rail. The item slice remains valid until the document
// revision changes. Hover, animation and paint state belong to the UI adapter.
func (e *Editor) HeadingRailSnapshot(viewport navigationrail.ViewportSize) navigationrail.Frame {
	frame := navigationrail.Frame{
		Active: -1, Revision: e.Session.Revision(),
		MarkerColor: color.RGBA{R: e.lk.marker.R, G: e.lk.marker.G, B: e.lk.marker.B, A: e.lk.marker.A},
		ActiveColor: color.RGBA{R: e.lk.text.R, G: e.lk.text.G, B: e.lk.text.B, A: e.lk.text.A},
		HoverColor:  color.RGBA{R: e.lk.muted.R, G: e.lk.muted.G, B: e.lk.muted.B, A: e.lk.muted.A},
	}
	if !navigationrail.ShouldShow(len(e.railLeaves), e.docH, e.viewH, viewport.Width, viewport.Height) {
		return frame
	}
	frame.Visible = true
	frame.Active = e.railCurrent()
	if e.railItems == nil || e.railItemsRev != e.Session.Revision() {
		e.railItems = e.railItems[:0]
		for _, leaf := range e.railLeaves {
			e.railItems = append(e.railItems, navigationrail.Item{Offset: leaf.start, Level: leaf.level})
		}
		e.railItemsRev = e.Session.Revision()
	}
	frame.Items = append([]navigationrail.Item(nil), e.railItems...)
	return frame
}

// HeadingRailPreview returns the title and a short excerpt for one item in
// the current snapshot. It is intended to run only for the hovered heading,
// so large documents do not project every section on each frame.
func (e *Editor) HeadingRailPreview(index int) navigationrail.Preview {
	if index < 0 || index >= len(e.railLeaves) {
		return navigationrail.Preview{}
	}
	e.railPreviewText(index)
	return navigationrail.Preview{Title: e.railText, Excerpt: e.railDesc, Level: e.railLeaves[index].level}
}

// JumpToHeading navigates to an exact heading source offset from a snapshot.
// It returns false if the offset no longer identifies a heading.
func (e *Editor) JumpToHeading(revision uint64, offset int) bool {
	if revision != e.Session.Revision() {
		return false
	}
	for i, leaf := range e.railLeaves {
		if leaf.start == offset {
			e.railJump(i)
			return true
		}
	}
	return false
}

// railCurrent returns the heading whose section contains the center of the
// viewport, with the first and last headings selected near document edges.
func (e *Editor) railCurrent() int {
	n := len(e.railLeaves)
	if n == 0 {
		return -1
	}
	if e.scroll.Y <= railEdge {
		return 0
	}
	if maxOff := e.docH - e.viewH; maxOff > 0 && e.scroll.Y >= maxOff-railEdge {
		return n - 1
	}
	line := e.scroll.Y + e.viewH/2
	i := sort.Search(n, func(i int) bool { return e.virtualYFor(e.railLeaves[i]) > line })
	return max(i-1, 0)
}

func (e *Editor) railJump(i int) {
	if i < 0 || i >= len(e.railLeaves) {
		return
	}
	e.NavigateToOffset(e.railLeaves[i].start, layout.PageJumpCenter)
	e.Focus()
}

// railPreviewText reads a heading and its section excerpt once per revision.
func (e *Editor) railPreviewText(i int) {
	if i < 0 || i >= len(e.railLeaves) {
		e.railText, e.railDesc = "", ""
		return
	}
	if e.railTextIdx == i && e.railTextRev == e.Session.Revision() {
		return
	}
	leaf := e.railLeaves[i]
	e.railText = strings.TrimSpace(e.plainText(e.p.Parser().Inlines(leaf.id)))
	e.railDesc = ""
	if i < len(e.railNext) && e.railNext[i] != nil && e.railNext[i].id >= 0 {
		e.railDesc = navigationrail.Excerpt(e.plainText(e.p.Parser().Inlines(e.railNext[i].id)))
	}
	e.railTextIdx, e.railTextRev = i, e.Session.Revision()
}
