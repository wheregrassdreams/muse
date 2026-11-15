package editor

// foldSection describes the source range hidden by one heading fold.
type foldSection struct {
	heading  *leaf
	from, to int
	content  bool
}

func (e *Editor) resetFolds() {
	e.folded = nil
	e.foldRevision++
	e.foldSectionsCache = nil
	e.foldCacheRevision = 0
	e.unitRev = 0
	e.unitList = nil
}

func (e *Editor) foldSections() []foldSection {
	e.sync()
	if e.foldCacheRevision == e.rev && e.foldSectionsCache != nil {
		return e.foldSectionsCache
	}
	headings := make([]*leaf, 0, len(e.railLeaves))
	for _, l := range e.leaves {
		if l.kind == kHeading && !l.pending {
			headings = append(headings, l)
		}
	}
	sections := make([]foldSection, 0, len(headings))
	for i, h := range headings {
		to := len(e.src)
		for _, next := range headings[i+1:] {
			if next.level <= h.level {
				to = next.lineStart
				break
			}
		}
		from := blockEnd(e.src, h)
		if from < len(e.src) {
			if e.src[from] == '\r' {
				from++
			}
			if from < len(e.src) && e.src[from] == '\n' {
				from++
			}
		}
		content := false
		for _, l := range e.leaves {
			if l.lineStart < from || l.lineStart >= to || l.isLine() {
				continue
			}
			content = true
			break
		}
		sections = append(sections, foldSection{heading: h, from: from, to: to, content: content})
	}
	e.foldSectionsCache, e.foldCacheRevision = sections, e.rev
	return sections
}

func (e *Editor) foldSectionAtHeading(h *leaf) (foldSection, bool) {
	if h == nil || h.kind != kHeading || h.pending {
		return foldSection{}, false
	}
	for _, section := range e.foldSections() {
		if section.heading.lineStart == h.lineStart {
			return section, section.content
		}
	}
	return foldSection{}, false
}

func (e *Editor) isFoldedHeading(h *leaf) bool {
	return h != nil && e.folded[h.lineStart]
}

func (e *Editor) foldHidden(l *leaf) bool {
	if l == nil || len(e.folded) == 0 {
		return false
	}
	for _, section := range e.foldSections() {
		if e.folded[section.heading.lineStart] && l != section.heading && l.lineStart >= section.from && l.lineStart < section.to {
			return true
		}
	}
	return false
}

// isFoldedSourceOffset reports whether a source offset belongs to hidden
// content. Source selection can remain intact while the presentation stays
// folded; only explicit navigation to the offset expands its parents.
func (e *Editor) isFoldedSourceOffset(off int) bool {
	for _, section := range e.foldSections() {
		if e.folded[section.heading.lineStart] && off >= section.from && off < section.to {
			return true
		}
	}
	return false
}

// nextVisibleSourceOffset advances from a hidden source offset to the next
// visible logical block in dir, without changing the document or fold state.
func (e *Editor) nextVisibleSourceOffset(off, dir int) int {
	if dir == 0 || !e.isFoldedSourceOffset(off) {
		return off
	}
	for _, l := range e.leaves {
		if e.foldHidden(l) {
			continue
		}
		if dir > 0 && l.lineStart > off {
			if s, ok := e.edgeStop(l, false); ok {
				return s
			}
		}
	}
	for i := len(e.leaves) - 1; i >= 0; i-- {
		l := e.leaves[i]
		if e.foldHidden(l) {
			continue
		}
		if dir < 0 && l.lineStart < off {
			if s, ok := e.edgeStop(l, true); ok {
				return s
			}
		}
	}
	for _, section := range e.foldSections() {
		if e.folded[section.heading.lineStart] && off >= section.from && off < section.to {
			return section.heading.end
		}
	}
	return off
}

func (e *Editor) toggleFold(h *leaf) {
	_, ok := e.foldSectionAtHeading(h)
	if !ok {
		return
	}
	if e.folded == nil {
		e.folded = make(map[int]bool)
	}
	if e.folded[h.lineStart] {
		delete(e.folded, h.lineStart)
	} else {
		e.folded[h.lineStart] = true
		if e.isFoldedSourceOffset(e.selectionFocus()) {
			e.setCaret(h.end, false)
		}
		e.clearTableSelection()
	}
	e.foldRevision++
	e.unitRev = 0
	e.unitList = nil
	e.leavesLaid = false
	e.invalidate()
}

func (e *Editor) expandFoldsAt(off int) bool {
	changed := false
	for _, section := range e.foldSections() {
		if e.folded[section.heading.lineStart] && off >= section.from && off < section.to {
			delete(e.folded, section.heading.lineStart)
			changed = true
		}
	}
	if changed {
		e.foldRevision++
		e.unitRev = 0
		e.unitList = nil
		e.leavesLaid = false
		e.invalidate()
	}
	return changed
}

func (e *Editor) expandFoldsForRange(start, end int) {
	if end < start {
		start, end = end, start
	}
	changed := false
	for _, section := range e.foldSections() {
		if e.folded[section.heading.lineStart] && start < section.to && end >= section.from {
			delete(e.folded, section.heading.lineStart)
			changed = true
		}
	}
	if changed {
		e.foldRevision++
		e.unitRev = 0
		e.unitList = nil
		e.leavesLaid = false
		e.invalidate()
	}
}

func (e *Editor) foldChevronRect(h *leaf) (x, y, size float32, ok bool) {
	if _, ok := e.foldSectionAtHeading(h); !ok || e.foldHidden(h) || h.view == nil || h.view.layout == nil {
		return 0, 0, 0, false
	}
	size = e.lk.size * 0.9
	x = h.x - e.lk.size*1.15
	lines := h.view.layout.Lines()
	if len(lines) == 0 {
		return 0, 0, 0, false
	}
	y = h.y + h.view.ty + lines[0].Rect.Y + (lines[0].Rect.H-size)/2
	return x, y, size, true
}

// foldHeadingAt returns the visible foldable heading under the pointer. The
// heading's laid out text lines and the adjacent left-side chevron corridor
// form one continuous hover target.
func (e *Editor) foldHeadingAt(x, y float32) *leaf {
	e.layoutIfNeeded()
	py := y
	for i := len(e.leaves) - 1; i >= 0; i-- {
		h := e.leaves[i]
		if h.kind != kHeading || e.foldHidden(h) || h.view == nil || h.view.layout == nil {
			continue
		}
		cx, cy, size, ok := e.foldChevronRect(h)
		if !ok {
			continue
		}
		for lineIndex, line := range h.view.layout.Lines() {
			ly := h.y + h.view.ty + line.Rect.Y
			if py < ly || py > ly+line.Rect.H {
				continue
			}
			left := h.x + h.view.tx + line.Rect.X
			right := left + line.Rect.W
			if lineIndex == 0 {
				left = min(left, cx-size*0.2)
				right = max(right, cx+size*1.2)
			}
			if x >= left && x <= right {
				return h
			}
		}
		// A small gap between the first title glyphs and the chevron should
		// remain in the same hover corridor as the icon itself.
		if py >= cy-size*0.1 && py <= cy+size*1.2 && x >= cx-size*0.2 && x <= cx+size*1.2 {
			return h
		}
	}
	return nil
}

func (e *Editor) foldChevronAt(x, y float32) *leaf {
	e.layoutIfNeeded()
	py := y
	for i := len(e.leaves) - 1; i >= 0; i-- {
		h := e.leaves[i]
		if h.kind != kHeading || e.foldHidden(h) || e.foldHover != h.lineStart || e.foldHeadingAt(x, y) != h {
			continue
		}
		cx, cy, size, ok := e.foldChevronRect(h)
		if ok && x >= cx-size*0.2 && x <= cx+size*1.2 && py >= cy-size*0.1 && py <= cy+size*1.2 {
			return h
		}
	}
	return nil
}
