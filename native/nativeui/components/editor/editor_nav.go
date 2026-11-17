package editor

import (
	"runtime"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"muse/editor/navigation"
	syntax "muse/markdown/syntax"
	tableui "muse/nativeui/components/editor/blocks/table"
)

func isSourceRuneBoundary(src string, off int) bool { return navigation.IsRuneBoundary(src, off) }

// The caret moves through the display: a grapheme, a word or a line at a
// time in what the user sees, mapped back to the source. Syntax hidden at
// the caret's old place may show at its new one, so every move ends on an
// offset the display at that offset can show (normalize).

// unit is a part of the document the caret moves through: a leaf, or a
// cell of a table.
func (e *Editor) units() []*leaf {
	if e.unitRev == e.Session.Revision() && e.unitList != nil {
		return e.unitList
	}
	out := e.unitList[:0]
	if cap(out) < len(e.leaves) {
		out = make([]*leaf, 0, len(e.leaves))
	}
	for _, l := range e.leaves {
		if e.foldHidden(l) {
			continue
		}
		if l.kind == kTable {
			for _, row := range l.tab.rows {
				out = append(out, row...)
			}
			continue
		}
		out = append(out, l)
	}
	e.unitList, e.unitRev = out, e.Session.Revision()
	return e.unitList
}

// isWordRune supports edit-side decisions unrelated to navigation tokens.
func isWordRune(r rune) bool { return navigation.IsWordRune(r) }

// unitIndex returns the index in units of the one holding the caret at
// off, or -1. A line of its own owns its offset: a leaf may end or begin
// there too, but the caret there is the line's.
func (e *Editor) unitIndex(us []*leaf, off int) int {
	var first *leaf
	if len(us) > 0 {
		first = us[0]
	}
	cache := &e.unitIndexCache
	if cache.valid && cache.rev == e.Session.Revision() && cache.folds == e.foldRevision && cache.off == off && cache.first == first && cache.length == len(us) {
		return cache.index
	}
	result := e.findUnitIndex(us, off)
	*cache = struct {
		rev, folds uint64
		off        int
		first      *leaf
		length     int
		index      int
		valid      bool
	}{e.Session.Revision(), e.foldRevision, off, first, len(us), result, true}
	return result
}

func (e *Editor) findUnitIndex(us []*leaf, off int) int {
	// One pass over the units: a line of its own may come after the leaf
	// that ends where it starts, and it wins when it does.
	line, cover := -1, -1
	for i, u := range us {
		if u.isLine() {
			if line < 0 && (off == u.start || u.kind == kBlank && off > u.start && off <= u.end) {
				line = i
			}
			continue
		}
		if cover >= 0 {
			continue
		}
		unitEnd := u.end
		if u.kind == kCode && u.fenced && e.codeFenceSourceShown(u) {
			unitEnd = blockEnd(e.Source(), u)
		}
		if off >= leafLo(e, u) && off <= unitEnd {
			if u.kind == kCode && u.fenced && off > u.infoEndOffset() && off < u.start {
				continue
			}
			cover = i
		}
	}
	if line >= 0 {
		return line
	}
	if t, right := e.tableAtEdge(off); t != nil {
		cells := t.tab.cells()
		target := cells[0]
		if right {
			target = cells[len(cells)-1]
		}
		for i, u := range us {
			if u == target {
				return i
			}
		}
	}
	return cover
}

// tableOf returns the table leaf a cell is in.
func (e *Editor) tableOf(cell *leaf) *leaf {
	if cell == nil {
		return nil
	}
	return cell.table
}

// tableAtEdge reports the virtual caret stops immediately before or after a
// table. Those stops let horizontal movement treat the table as one unit.
func (e *Editor) tableAtEdge(off int) (table *leaf, after bool) {
	if !e.tableEdgeIndexed || e.tableEdgeIndexRev != e.Session.Revision() {
		e.tableEdgeIndex = e.tableEdgeIndex[:0]
		for _, l := range e.leaves {
			if l.kind != kTable || l.tab == nil || !tableHasCells(l.tab) {
				continue
			}
			e.tableEdgeIndex = append(e.tableEdgeIndex, tableEdgePosition{
				leaf: l, start: l.lineStart, end: blockEnd(e.Source(), l),
			})
		}
		e.tableEdgeIndexRev, e.tableEdgeIndexed = e.Session.Revision(), true
	}
	i := sort.Search(len(e.tableEdgeIndex), func(i int) bool { return e.tableEdgeIndex[i].start >= off })
	if i < len(e.tableEdgeIndex) && e.tableEdgeIndex[i].start == off {
		return e.tableEdgeIndex[i].leaf, false
	}
	i = sort.Search(len(e.tableEdgeIndex), func(i int) bool { return e.tableEdgeIndex[i].end >= off })
	if i < len(e.tableEdgeIndex) && e.tableEdgeIndex[i].end == off {
		return e.tableEdgeIndex[i].leaf, true
	}
	return nil, false
}

type tableEdgePosition struct {
	leaf       *leaf
	start, end int
}

func tableHasCells(t *table) bool {
	if t == nil {
		return false
	}
	for _, row := range t.rows {
		if len(row) > 0 {
			return true
		}
	}
	return false
}

func (e *Editor) tableSideStep(t *leaf, dir int) int {
	us := e.units()
	cells := t.tab.cells()
	first, last := -1, -1
	for i, u := range us {
		if u == cells[0] {
			first = i
		}
		if u == cells[len(cells)-1] {
			last = i
		}
	}
	i := first - 1
	if dir > 0 {
		i = last + 1
	}
	for i >= 0 && i < len(us) {
		if s, ok := e.edgeStop(us[i], dir < 0); ok {
			return s
		}
		i += dir
	}
	if dir < 0 {
		return t.lineStart
	}
	return blockEnd(e.Source(), t)
}

// isCell reports whether a unit is a table's cell.
func (e *Editor) isCell(u *leaf) bool {
	return u.kind == kPara && u.conts == nil && e.tableOf(u) != nil
}

// unitView returns the view of a unit as it shows with the caret at off.
func (e *Editor) unitView(u *leaf, off int) *leafView {
	if u.view == nil || e.revealFor(u, off) != e.revealAt(u) {
		header, align := false, syntax.IncrementalAlignNone
		if t := e.tableOf(u); t != nil {
			for r, row := range t.tab.rows {
				for c, cell := range row {
					if cell == u {
						header, align = r == 0, alignAt(t.tab.align, c)
					}
				}
			}
		}
		return e.viewWith(u, u.w, header, align, e.revealFor(u, off))
	}
	return u.view
}

// revealFor returns what revealAt would be with the caret at off.
func (e *Editor) revealFor(l *leaf, off int) int {
	if !e.focused {
		return -1
	}
	if s, end, ok := containerPrefixRange(l); ok && off >= s && off <= end {
		return off - l.lineStart
	}
	for _, r := range quoteSourcePrefixes(e, l) {
		if off >= r.Start && off <= r.End {
			return off - l.lineStart
		}
	}
	if off < l.lineStart || off > l.end {
		if l.kind == kCode && l.fenced && off >= l.infoStartOffset() && off <= l.infoEndOffset() {
			return off - l.lineStart
		}
		if l.kind == kCode && l.fenced && e.codeFenceSourceShown(l) && off <= blockEnd(e.Source(), l) {
			return off - l.lineStart
		}
		return -1
	}
	return off - l.lineStart
}

// codeFenceSourceShown says whether a fenced code block currently exposes
// its Markdown source, including both fence rows. Its source-backed range
// then participates in caret mapping and selection just like the code body.
func (e *Editor) codeFenceSourceShown(l *leaf) bool {
	return e.sourceOpen(l) || !isMermaid(l) && sourceRangeTargeted(e, l.lineStart, blockEnd(e.Source(), l), false)
}

// stopOK reports whether the caret can be at off, in the display of its
// unit as it would show with the caret there.
func (e *Editor) stopOK(u *leaf, off int) bool {
	if u.isLine() {
		if u.kind == kBlank && off >= u.start && off <= u.end {
			return isSourceRuneBoundary(e.Source(), off)
		}
		return off == u.start
	}
	if t := e.tableOf(u); t != nil {
		if edge, _ := e.tableAtEdge(off); edge == t {
			return true
		}
	}
	v := e.unitView(u, off)
	if v.layout == nil {
		return false
	}
	_, ok := v.disp.srcToDisp(off - u.lineStart)
	return ok
}

// normalize returns the offset the caret takes for off: off itself when
// the display can show the caret there, else the nearest offset that it
// can, in direction dir first.
func (e *Editor) normalize(off, dir int) int {
	e.layoutIfNeeded()
	// Row boundaries are real structural caret stops. Treating them as
	// source gaps advances a pointer endpoint into the next row on release.
	if _, _, _, ok := e.tableBoundaryAtOffset(off); ok {
		return off
	}
	us := e.units()
	i := e.unitIndex(us, off)
	if i < 0 {
		// Between leaves: the edge of the next one in dir, or of the one
		// the other way.
		return e.gapStop(us, off, dir)
	}
	u := us[i]
	if e.stopOK(u, off) {
		return off
	}
	v := e.unitView(u, off)
	if v.layout == nil {
		if s, ok := e.edgeStop(u, dir < 0); ok {
			return s
		}
		return off
	}
	stops := v.disp.stops()
	best, bestD := -1, 1<<30
	for _, s := range stops {
		a := s + u.lineStart
		if !e.stopOK(u, a) {
			continue
		}
		d := a - off
		if dir < 0 {
			d = -d
		}
		if d < 0 {
			d = 1<<20 - d
		}
		if d < bestD {
			best, bestD = a, d
		}
	}
	if best < 0 {
		return off
	}
	return best
}

// gapStop returns where the caret goes for an offset between leaves.
func (e *Editor) gapStop(us []*leaf, off, dir int) int {
	if dir >= 0 {
		for _, u := range us {
			if leafLo(e, u) >= off {
				if s, ok := e.edgeStop(u, false); ok {
					return s
				}
			}
		}
	}
	for i := len(us) - 1; i >= 0; i-- {
		if us[i].end <= off {
			if s, ok := e.edgeStop(us[i], true); ok {
				return s
			}
		}
	}
	for _, u := range us {
		if s, ok := e.edgeStop(u, false); ok {
			return s
		}
	}
	return off
}

// edgeStop returns the first (or, with end set, the last) offset the caret
// can take in a leaf.
func (e *Editor) edgeStop(l *leaf, end bool) (int, bool) {
	if l.kind == kTable {
		if l.tab == nil || len(l.tab.cells()) == 0 {
			return 0, false
		}
		if end {
			return blockEnd(e.Source(), l), true
		}
		return l.lineStart, true
	}
	if l.isLine() {
		if end && l.kind == kBlank && l.end > l.start {
			return l.end, true
		}
		return l.start, true
	}
	off := l.start
	if end {
		off = l.end
	}
	if l.kind == kCode && l.fenced && len(l.codeLines()) == 0 {
		// A fence without lines: its info string.
		if end {
			return l.infoEndOffset(), true
		}
		return l.infoEndOffset(), true
	}
	v := e.unitView(l, off)
	if v.layout == nil {
		return off, true
	}
	if _, ok := v.disp.srcToDisp(off - l.lineStart); ok {
		return off, true
	}
	stops := v.disp.stops()
	if len(stops) == 0 {
		return off, true
	}
	if end {
		return stops[len(stops)-1] + l.lineStart, true
	}
	return stops[0] + l.lineStart, true
}

// step returns the caret one grapheme (or word) from off, in direction
// dir, across leaves at their edges.
func (e *Editor) step(off, dir int, word bool) int {
	return e.stepWithWordMode(off, dir, word, false)
}

func (e *Editor) stepWithWordMode(off, dir int, word, bigWord bool) int {
	e.layoutIfNeeded()
	if t, row, right, ok := e.tableBoundaryAtOffset(off); ok {
		if !right && dir > 0 {
			return t.tab.rows[row][0].start
		}
		if right && dir < 0 {
			cells := t.tab.rows[row]
			return cells[len(cells)-1].end
		}
		if !right && dir < 0 {
			if row > 0 {
				return e.tableRowBoundary(t, row-1, true)
			}
			return e.tableSideStep(t, dir)
		}
		if right && dir > 0 {
			if row+1 < len(t.tab.rows) {
				return e.tableRowBoundary(t, row+1, false)
			}
			return e.tableSideStep(t, dir)
		}
	}
	if t, after := e.tableAtEdge(off); t != nil {
		if dir > 0 && !after {
			return blockEnd(e.Source(), t)
		}
		if dir < 0 && after {
			return t.lineStart
		}
		return e.tableSideStep(t, dir)
	}
	us := e.units()
	i := e.unitIndex(us, off)
	if i < 0 {
		return e.gapStop(us, off, dir)
	}
	u := us[i]
	if t := e.tableOf(u); t != nil {
		if _, row, col := cellAt(t, off); row >= 0 && row < len(t.tab.rows) {
			if dir < 0 && col == 0 {
				if edge, ok := e.edgeStop(u, false); ok && off == edge {
					return e.tableRowBoundary(t, row, false)
				}
			}
			if dir > 0 && col == len(t.tab.rows[row])-1 {
				if edge, ok := e.edgeStop(u, true); ok && off == edge {
					return e.tableRowBoundary(t, row, true)
				}
			}
		}
		cells := t.tab.cells()
		if dir < 0 && u == cells[0] {
			if edge, ok := e.edgeStop(u, false); ok && off == edge {
				return t.lineStart
			}
		}
		if dir > 0 && u == cells[len(cells)-1] {
			if edge, ok := e.edgeStop(u, true); ok && off == edge {
				return blockEnd(e.Source(), t)
			}
		}
	}
	if u.kind == kCode && u.fenced && dir < 0 && off == u.start && e.revealFor(u, off) >= 0 && !(off >= u.infoStartOffset() && off <= u.infoEndOffset()) {
		// From the start of the code to its fence's info string.
		return u.infoEndOffset()
	}
	if u.isLine() {
		// Out of a line of its own: the edge of the neighbour it faces. That
		// edge may be the phantom's own offset — the same source shown
		// on the neighbour's line — so the phantom goes either way.
		// With no neighbour the caret stays where it is.
		for j := i + dir; j >= 0 && j < len(us); j += dir {
			if s, ok := e.edgeStop(us[j], dir < 0); ok {
				e.dropPhantom()
				return s
			}
		}
		return off
	}
	if word {
		var s int
		var ok bool
		if bigWord {
			s, ok = e.bigWordStepIn(u, off, dir)
		} else {
			s, ok = e.wordStepIn(u, off, dir)
		}
		if ok {
			return s
		}
	} else if s, ok := e.stopStepIn(u, off, dir); ok {
		// One stop at a time, in the source's own offsets: an object is one
		// unit, and the positions on either side of it are two — the display
		// puts them in the same column, so moving in the display would step
		// over both (see docs/editor-model.md, atomic objects).
		return s
	}
	for j := i + dir; j >= 0 && j < len(us); j += dir {
		if t := e.tableOf(us[j]); t != nil {
			cells := t.tab.cells()
			if dir > 0 && us[j] == cells[0] {
				return t.lineStart
			}
			if dir < 0 && us[j] == cells[len(cells)-1] {
				return blockEnd(e.Source(), t)
			}
		}
		if s, ok := e.edgeStop(us[j], dir < 0); ok {
			return s
		}
	}
	return off
}

// stopStepIn returns the next stop of a unit from off, in direction dir:
// the caret's positions of the display, which leave out the inside of an
// object.
func (e *Editor) stopStepIn(u *leaf, off, dir int) (int, bool) {
	viewOff := off
	if dir < 0 {
		if _, end, ok := e.editablePrefixRange(u); ok && off == end {
			// Reveal the source prefix while stepping left from the first
			// rendered character. At the boundary itself list markers stay
			// compact, preserving task checkboxes as controls.
			viewOff = end - 1
		}
	}
	stops := e.unitView(u, viewOff).disp.stops()
	best := -1
	for _, s := range stops {
		at := s + u.lineStart
		if dir > 0 && at > off || dir < 0 && at < off {
			// Display stops can belong to zero-source decorations such as an
			// external-link icon. Ignore those and keep searching in this unit
			// for the next stop that has a real source mapping.
			if !e.stopOK(u, at) {
				continue
			}
			if best < 0 || dir > 0 && at < best || dir < 0 && at > best {
				best = at
			}
		}
	}
	if best < 0 {
		return 0, false
	}
	return e.normalize(best, dir), true
}

// wordStepIn returns the offset a word move takes from off within a unit.
func (e *Editor) wordStepIn(u *leaf, off, dir int) (int, bool) {
	v := e.unitView(u, off)
	if v.layout == nil || v.empty {
		return 0, false
	}
	d, ok := v.disp.srcToDisp(off - u.lineStart)
	if !ok {
		return 0, false
	}
	w := e.wordsForDisplay(v.disp.text)
	b := w.byteAtUTF16(d, false)
	var target int
	var found bool
	for range w.index.TokenCount() + 1 {
		if dir > 0 {
			target, found = w.index.NextStart(b)
		} else {
			target, found = w.index.PrevStart(b)
		}
		if !found {
			return 0, false
		}
		tok, ok := w.index.WordAt(target)
		if !ok {
			return 0, false
		}
		nd := w.utf16AtByte(target, dir > 0)
		end := w.utf16AtByte(tok.End, true)
		if displayRangeHasSource(&v.disp, nd, end) {
			break
		}
		// NextStart is strict: resume at this decoration's start so a real
		// token immediately following its end is still considered.
		b = tok.Start
	}
	nd := w.utf16AtByte(target, dir > 0)
	c := v.disp.dispToSrc(nd, dir > 0) + u.lineStart
	if dir > 0 && c <= off || dir < 0 && c >= off {
		return 0, false
	}
	return e.normalize(c, dir), true
}

func (e *Editor) bigWordStepIn(u *leaf, off, dir int) (int, bool) {
	v := e.unitView(u, off)
	if v.layout == nil || v.empty {
		return 0, false
	}
	d, ok := v.disp.srcToDisp(off - u.lineStart)
	if !ok {
		return 0, false
	}
	w := e.wordsForDisplay(v.disp.text)
	b := w.byteAtUTF16(d, false)
	var target int
	found := false
	for range len(w.bigWords) + 1 {
		r, exists := w.bigWordStart(b, dir)
		if !exists {
			return 0, false
		}
		start, end := w.utf16AtByte(r.Start, false), w.utf16AtByte(r.End, true)
		if displayRangeHasSource(&v.disp, start, end) {
			target, found = start, true
			break
		}
		b = r.Start
	}
	if !found {
		return 0, false
	}
	c := v.disp.dispToSrc(target, dir > 0) + u.lineStart
	if dir > 0 && c <= off || dir < 0 && c >= off {
		return 0, false
	}
	return e.normalize(c, dir), true
}

func displayRangeHasSource(d *display, start, end int) bool {
	if end <= start {
		return false
	}
	for _, p := range d.pieces {
		if p.kind == pDeco && start < p.d1 && p.d0 < end {
			return false
		}
	}
	return d.dispToSrc(end, true) > d.dispToSrc(start, false)
}

// nearestStop returns the stop at or before rel, or the first one after it.
func nearestStop(stops []int, rel int) (int, bool) {
	if len(stops) == 0 {
		return 0, false
	}
	best := stops[0]
	for _, s := range stops {
		if s <= rel {
			best = s
		}
	}
	return best, true
}

// ---------------------------------------------------------------------------
// Geometry

// caretRect returns the caret's box at off, in the element's coordinates.
func (e *Editor) caretRect(off int) (ui.Rect, bool) {
	e.layoutIfNeeded()
	if e.phantom != nil && e.leafIsPhantom(off) {
		u := e.leafAt(off)
		return ui.Rect{X: u.x, Y: u.y, W: 1, H: u.h}, true
	}
	if t, row, right, ok := e.tableBoundaryAtOffset(off); ok && row+1 < len(t.geom.rowY) {
		x := tableContentX(t)
		if right {
			x += t.geom.w
		}
		return ui.Rect{X: x, Y: t.y + t.geom.rowY[row], W: 1, H: t.geom.rowY[row+1] - t.geom.rowY[row]}, true
	}
	if t, after := e.tableAtEdge(off); t != nil && t.geom != nil && len(t.geom.rowY) > 0 {
		x := tableContentX(t)
		row := 0
		if after {
			x += t.geom.w
			row = len(t.geom.rowY) - 2
		}
		row = max(0, min(row, len(t.geom.rowY)-2))
		return ui.Rect{X: x, Y: t.y + t.geom.rowY[row], W: 1, H: t.geom.rowY[row+1] - t.geom.rowY[row]}, true
	}
	us := e.units()
	i := e.unitIndex(us, off)
	if i < 0 {
		return ui.Rect{}, false
	}
	u := us[i]
	if r, ok := e.vimImageRowCaretRect(u); ok {
		return r, true
	}
	v := e.unitView(u, off)
	if v.layout == nil {
		return ui.Rect{X: u.x - blockScrollOffset(u), Y: u.y, W: 1, H: u.h}, true
	}
	d, ok := v.disp.srcToDisp(off - u.lineStart)
	if !ok {
		// An offset the display has no place for — the inside of an object,
		// which is one unit — draws the caret at the nearest place it does,
		// so that the caret is never invisible.
		at, sok := nearestStop(v.disp.stops(), off-u.lineStart)
		if !sok {
			return ui.Rect{}, false
		}
		d, _ = v.disp.srcToDisp(at)
	}
	if v.empty {
		d = 0
	}
	aff := ui.TextDownstream
	if e.upstream {
		aff = ui.TextUpstream
	}
	r := v.layout.CaretAt(ui.TextCaretPosition{Index: d, Affinity: aff})
	if lines := v.layout.Lines(); len(lines) > 0 && v.font.Size > 0 {
		// As tall as the text, on its baseline, not the line's leading.
		ln := lines[lineOf(lines, d, e.upstream)]
		m := v.font.Metrics()
		r.Y, r.H = max(ln.Baseline-m.Ascent, ln.Rect.Y), min(m.Ascent+m.Descent, ln.Rect.H)
	}
	r.X += unitTextX(u, v)
	r.Y += u.y + v.ty
	return r, true
}

// lineOf returns the line of a layout holding display offset d.
func lineOf(lines []ui.TextLine, d int, upstream bool) int {
	for k, ln := range lines {
		if d < ln.Start {
			return max(k-1, 0)
		}
		if d < ln.End || (d == ln.End && (upstream || k == len(lines)-1 || lines[k+1].Start > ln.End)) {
			return k
		}
	}
	return len(lines) - 1
}

// layoutLineHasCaretStop reports whether a rendered row has a caret stop
// backed by source text. Projection-only rows (such as a link icon) have
// display geometry but no independent source position, so vertical motion
// must step over them. Atoms count because their source-backed edges are
// valid stops even when their display text is an object or replacement.
func layoutLineHasCaretStop(d display, ln ui.TextLine) bool {
	for i := range d.pieces {
		p := &d.pieces[i]
		if p.kind == pDeco || p.s1 <= p.s0 {
			continue
		}
		if ln.Start == ln.End {
			if ln.Start >= p.d0 && ln.Start <= p.d1 {
				return true
			}
			continue
		}
		if p.d0 < ln.End && p.d1 > ln.Start {
			return true
		}
	}
	return false
}

// vertical moves the caret up (dir < 0) or down lines, keeping its x.
func (e *Editor) vertical(dir int, extend bool) {
	if e.moveVimImageRow(dir, extend) {
		return
	}
	_, _ = e.tableUI().Dispatch(tableui.Action{Kind: tableui.ResetNavigation})
	focus := e.selectionFocus()
	if e.tableSelection != nil {
		e.tableSelection = nil
		e.setSessionSelection(focus, focus)
	}
	e.layoutIfNeeded()
	e.tableBridge = nil
	if t, row, right, ok := e.tableBoundaryAtOffset(e.Caret()); ok {
		// A table embedded in a document selection is one semantic unit.
		if extend && (e.Session.Selection().Anchor < t.lineStart || e.Session.Selection().Anchor > blockEnd(e.Source(), t)) {
			e.leaveTableBoundary(t, dir, true)
			return
		}
		next := row + dir
		if next < 0 || next >= len(t.tab.rows) {
			r, _ := e.caretRect(e.Caret())
			if !e.hasGoal {
				e.goalX, e.hasGoal = r.X, true
			}
			ti := -1
			for i, l := range e.leaves {
				if l == t {
					ti = i
					break
				}
			}
			if ti < 0 {
				return
			}
			if !extend && (ti+dir < 0 || ti+dir >= len(e.leaves)) && e.phantomAtEdge(ti, dir) {
				return
			}
			e.enterLeaf(ti+dir, dir, extend)
			return
		}
		lines := e.tableLines(t)
		li := next
		if next > 0 {
			li++
		}
		if li < len(lines) {
			off := lines[li][0]
			if right {
				off = lines[li][1]
			}
			e.setCaret(off, extend)
			e.hasGoal = false
			return
		}
	}
	r, ok := e.caretRect(e.Caret())
	if !ok {
		return
	}
	if !e.hasGoal {
		e.goalX, e.hasGoal = r.X, true
	}
	if t, after := e.tableAtEdge(e.Caret()); t != nil {
		for i, l := range e.leaves {
			if l != t {
				continue
			}
			if dir > 0 && !after || dir < 0 && after {
				e.enterLeaf(i, dir, extend)
			} else {
				e.enterLeaf(i+dir, dir, extend)
			}
			return
		}
	}
	us := e.units()
	i := e.unitIndex(us, e.Caret())
	if i < 0 {
		return
	}
	u := us[i]
	v := e.unitView(u, e.Caret())
	// Within the unit: the line above or below.
	if v.layout != nil && !v.empty {
		d, _ := v.disp.srcToDisp(e.Caret() - u.lineStart)
		lines := v.layout.Lines()
		k := lineOf(lines, d, e.upstream)
		for target := k + dir; target >= 0 && target < len(lines); target += dir {
			ln := lines[target]
			if !layoutLineHasCaretStop(v.disp, ln) {
				continue
			}
			off := e.hitUnit(u, v, e.goalX, u.y+v.ty+ln.Rect.Y+ln.Rect.H/2)
			// A display row may contain source text, but nearest-hit mapping
			// can still land on a neighboring projection edge. Only consume
			// this row when the resulting offset really belongs to it.
			mapped, ok := v.disp.srcToDisp(off - u.lineStart)
			if ok && lineOf(lines, mapped, false) == target {
				e.setCaretKeepGoal(off, extend)
				return
			}
		}
	}
	// A cell: the cell above or below in the same column.
	if t := e.tableOf(u); t != nil {
		r0, c0 := -1, -1
		for ri, row := range t.tab.rows {
			for ci, cell := range row {
				if cell == u {
					r0, c0 = ri, ci
				}
			}
		}
		if nr := r0 + dir; nr >= 0 && nr < len(t.tab.rows) && c0 < len(t.tab.rows[nr]) {
			cell := t.tab.rows[nr][c0]
			cv := e.unitView(cell, cell.start)
			y := cell.y + cv.ty
			if dir < 0 {
				y = cell.y + cv.h - 1
			}
			e.setCaretKeepGoal(e.hitUnit(cell, cv, e.goalX, y), extend)
			return
		}
		// Out of the table: to the leaf after or before it.
		ti := -1
		for k, l := range e.leaves {
			if l == t {
				ti = k
			}
		}
		e.enterLeaf(ti+dir, dir, extend)
		return
	}
	li := -1
	for k, l := range e.leaves {
		if l == u {
			li = k
		}
	}
	if li < 0 {
		return
	}
	next := li + dir
	for next >= 0 && next < len(e.leaves) && e.foldHidden(e.leaves[next]) {
		next += dir
	}
	if next < 0 || next >= len(e.leaves) {
		// Past the first or last line: to the start or end, as text views
		// go, unless a block of code, math or a table ends the document,
		// where a new paragraph comes for the caret.
		if !extend && e.phantomAtEdge(li, dir) {
			return
		}
		if dir < 0 {
			e.setCaretKeepGoal(e.firstStop(), extend)
		} else {
			e.setCaretKeepGoal(e.lastStop(), extend)
		}
		return
	}
	e.enterLeaf(next, dir, extend)
}

// enterLeaf moves the caret into leaf li from above (dir > 0) or below, at
// the goal's x: its first line or its last.
func (e *Editor) enterLeaf(li, dir int, extend bool) {
	for li >= 0 && li < len(e.leaves) && e.foldHidden(e.leaves[li]) {
		li += dir
	}
	if li < 0 || li >= len(e.leaves) {
		if dir < 0 {
			e.setCaretKeepGoal(e.firstStop(), extend)
		} else {
			e.setCaretKeepGoal(e.lastStop(), extend)
		}
		return
	}
	l := e.leaves[li]
	if l.kind == kTable {
		if extend && (e.Session.Selection().Anchor < l.lineStart || e.Session.Selection().Anchor > blockEnd(e.Source(), l) || e.phantom != nil && e.leafIsPhantom(e.Caret())) {
			// Enter the table as a whole structure, keeping the outside anchor.
			off := blockEnd(e.Source(), l)
			if e.Session.Selection().Anchor >= off {
				off = l.lineStart
			}
			e.dropPhantom()
			e.setCaretKeepGoal(off, true)
			return
		}
		row := l.tab.rows[0]
		if dir < 0 {
			row = l.tab.rows[len(l.tab.rows)-1]
		}
		best := row[0]
		for _, cell := range row {
			if cell.x <= e.goalX {
				best = cell
			}
		}
		v := e.unitView(best, best.start)
		y := best.y + v.ty
		if dir < 0 {
			y = best.y + v.h - 1
		}
		e.setCaretKeepGoal(e.hitUnit(best, v, e.goalX, y), extend)
		return
	}
	if l.isLine() {
		e.setCaretKeepGoal(l.start, extend)
		return
	}
	if e.enterVimImageRow(l, dir, extend) {
		return
	}
	// The caret entering a block may show its source: lay it out as it
	// shows with the caret at its edge, and take the line at that edge.
	edge, ok := e.edgeStop(l, dir < 0)
	if !ok {
		return
	}
	v := e.unitView(l, edge)
	if v.layout == nil || v.empty {
		e.setCaretKeepGoal(edge, extend)
		return
	}
	lines := v.layout.Lines()
	ln := lines[0]
	if dir < 0 {
		ln = lines[len(lines)-1]
	}
	if l.kind == kCode && dir > 0 && v.reveal && len(lines) > 1 {
		// Coming down into code, past the info string shown above it.
		ln = lines[1]
	}
	e.setCaretKeepGoal(e.hitUnit(l, v, e.goalX, l.y+v.ty+ln.Rect.Y+ln.Rect.H/2), extend)
}

// leaveTableBoundary moves vertically from a table into its neighboring leaf.
func (e *Editor) leaveTableBoundary(t *leaf, dir int, extend bool) {
	if t == nil || dir == 0 {
		return
	}
	for i, l := range e.leaves {
		if l != t {
			continue
		}
		if !extend && (i+dir < 0 || i+dir >= len(e.leaves)) && e.phantomAtEdge(i, dir) {
			return
		}
		e.enterLeaf(i+dir, dir, extend)
		return
	}
}

// phantomAtEdge puts a new paragraph below the last leaf (dir > 0) or above
// the first when it is a block the caret cannot type a paragraph after:
// code, math, a table, a rule or HTML. It reports whether it did.
func (e *Editor) phantomAtEdge(li, dir int) bool {
	l := e.leaves[li]
	switch l.kind {
	case kCode, kMath, kTable, kRule, kHTML:
	default:
		return false
	}
	if len(l.conts) > 0 || l.whole {
		return false
	}
	if dir > 0 {
		if strings.HasSuffix(e.Source(), "\n") {
			// The source already ends with a line to type on.
			return false
		}
		end := blockEnd(e.Source(), l)
		e.setPhantom(&phantom{at: end, pre: "\n"})
	} else {
		e.setPhantom(&phantom{at: l.lineStart, post: "\n", before: true})
	}
	return true
}

// setCaretKeepGoal moves the caret as a vertical move does, keeping the
// goal's x for the next.
func (e *Editor) setCaretKeepGoal(off int, extend bool) {
	g := e.goalX
	e.setCaret(off, extend)
	e.goalX, e.hasGoal = g, true
}

// hitUnit returns the offset at x, y (element coordinates) in a unit.
func (e *Editor) hitUnit(u *leaf, v *leafView, x, y float32) int {
	if u.isLine() {
		return u.start
	}
	if v.layout == nil {
		if s, ok := e.edgeStop(u, false); ok {
			return s
		}
		return u.start
	}
	if v.empty {
		if s, ok := e.edgeStop(u, false); ok {
			return s
		}
	}
	d := v.layout.IndexAt(ui.Point{X: x - unitTextX(u, v), Y: y - u.y - v.ty})
	d = min(d, ui.UTF16Len(v.disp.text))
	off := v.disp.dispToSrc(d, true) + u.lineStart
	e.upstream = false
	if lines := v.layout.Lines(); len(lines) > 1 {
		k := lineOf(lines, d, false)
		ln := lines[k]
		if k > 0 && d == ln.Start && lines[k-1].End == d && y-u.y-v.ty < ln.Rect.Y {
			e.upstream = true
		}
	}
	return e.normalize(off, 1)
}

// hitTest returns the offset at x, y in the element: in the leaf, or the
// cell of a table, nearest to it.
func (e *Editor) hitTest(x, y float32) int {
	e.layoutIfNeeded()
	if len(e.leaves) == 0 {
		return 0
	}
	l := e.leafAtPoint(y)
	if l == nil {
		return 0
	}
	if l.kind == kTable {
		if l.geom != nil {
			if x < l.x {
				row := tableRowIndexAt(l, y)
				return e.tableRowBoundary(l, row, false)
			}
			if x > l.x+tableViewportWidth(l) {
				row := tableRowIndexAt(l, y)
				return e.tableRowBoundary(l, row, true)
			}
		}
		var best *leaf
		bestD := float32(1 << 30)
		for _, cell := range l.tab.cells() {
			dx := max(cell.x-x, x-(cell.x+cell.w), 0)
			dy := max(cell.y-y, y-(cell.y+cell.h), 0)
			if d := dx + dy; d < bestD {
				best, bestD = cell, d
			}
		}
		if best == nil {
			return l.start
		}
		return e.hitUnit(best, e.unitView(best, e.Caret()), x, y)
	}
	v := e.unitView(l, e.Caret())
	if l.kind == kCode && v.label != "" {
		// The label of the language: to the info string.
	}
	return e.hitUnit(l, v, x, y)
}

// leafAtPoint returns the leaf the point's y is on: the last one starting
// above it, or the nearer of two when the point falls between them.
func (e *Editor) leafAtPoint(y float32) *leaf {
	visible := make([]*leaf, 0, len(e.leaves))
	for _, l := range e.leaves {
		if !e.foldHidden(l) {
			visible = append(visible, l)
		}
	}
	if len(visible) == 0 {
		return nil
	}
	li := 0
	for k, l := range visible {
		if y >= e.virtualYFor(l) {
			li = k
		}
	}
	// Between two leaves, the nearer.
	if li+1 < len(visible) {
		l, n := visible[li], visible[li+1]
		ly, ny := e.virtualYFor(l), e.virtualYFor(n)
		if y > ly+l.h && ny-y < y-(ly+l.h) {
			li++
		}
	}
	return visible[li]
}

// objectAt returns the source range of the object the pointer at x hit: the
// picture, formula or embed whose edge is at off. An object that shows its
// source is not one — its text is what the pointer is on.
func (e *Editor) objectAt(off int) (int, int, bool) {
	l := e.leafAt(off)
	if l == nil || l.view == nil {
		return 0, 0, false
	}
	rel := off - l.lineStart
	for _, p := range l.view.disp.pieces {
		if p.kind != pAtom || (p.obj == nil && !e.isInlineMathRange(l.lineStart+p.s0, l.lineStart+p.s1)) || p.s1 <= p.s0 {
			continue
		}
		// A pointer on the object's box lands on one of its two edges: the
		// whole range is the object, and the two offsets are where the
		// caret can stand beside it.
		if p.s0 == rel || p.s1 == rel {
			return l.lineStart + p.s0, l.lineStart + p.s1, true
		}
	}
	return 0, 0, false
}

// onALine reports whether y falls on a leaf the document holds — its text,
// or one of its blank lines — rather than in the space between two of them.
func (e *Editor) onALine(y float32) bool {
	l := e.leafAtPoint(y)
	return l != nil && y >= l.y && y <= l.y+l.h
}

// setCaretAtPoint moves the caret to the offset a pointer at y hit. The
// phantom stays only when the point is on the phantom's own line: the leaf
// before it ends at the phantom's offset too, and a click at that line's
// end means that leaf, not an empty paragraph below.
// gapPhantomAt returns the offset of the line at y when no leaf covers it:
// the lines of a comment, which the parse reads as blank and which the
// editor shows once the caret is in them. Every other line of the document
// is a leaf — a block or a blank line — and is hit through the leaves.
func (e *Editor) gapPhantomAt(y float32) (int, bool) {
	e.layoutIfNeeded()
	var prev, next *leaf
	for _, l := range e.leaves {
		// Tables draw through geom rather than a text view, but still own
		// the document edge used to measure and map the following source gap.
		if l.isLine() || l.view == nil && l.kind != kTable {
			continue
		}
		ly := e.virtualYFor(l)
		if y >= ly && y <= ly+l.h {
			return 0, false
		}
		if ly+l.h < y {
			prev = l
		} else if next == nil && ly > y {
			next = l
		}
	}
	if prev == nil || next == nil {
		return 0, false
	}
	prevEnd := blockEnd(e.Source(), prev)
	nextStart := next.lineStart
	if nextStart <= prevEnd {
		return 0, false
	}
	n := strings.Count(e.Source()[prevEnd:nextStart], "\n")
	if n < 2 {
		return 0, false
	}
	lineH := e.lk.size * e.lk.lh
	k := int((y - (e.virtualYFor(prev) + prev.h)) / lineH)
	k = max(0, min(k, n-2))
	at := prevEnd + 1 + k
	if at >= nextStart {
		return 0, false
	}
	return at, true
}

func (e *Editor) setCaretAtPoint(off int, y float32, extend bool) {
	e.setSessionInclusive(false)
	e.setSessionLinewise(false)
	if l := e.leafAtPoint(y); e.phantom != nil && off == e.phantom.at && l != nil && l.kind != kPhantom {
		e.dropPhantom()
	}
	e.setCaret(off, extend)
}

// lineEdges returns the offsets of the start and end of the visual line
// the caret is on.
func (e *Editor) lineEdges(off int) (int, int) {
	e.layoutIfNeeded()
	us := e.units()
	i := e.unitIndex(us, off)
	if i < 0 {
		return off, off
	}
	u := us[i]
	v := e.unitView(u, off)
	if v.layout == nil || v.empty {
		return off, off
	}
	d, ok := v.disp.srcToDisp(off - u.lineStart)
	if !ok {
		return off, off
	}
	lines := v.layout.Lines()
	ln := lines[lineOf(lines, d, e.upstream)]
	end := ln.End
	// A line ending in a newline ends before it.
	s := e.normalize(v.disp.dispToSrc(ln.Start, false)+u.lineStart, 1)
	z := e.normalize(v.disp.dispToSrc(end, true)+u.lineStart, -1)
	return s, z
}

// wordRangeAt returns the source range of the word at off.
func (e *Editor) wordRangeAt(off int) (int, int) {
	e.layoutIfNeeded()
	us := e.units()
	i := e.unitIndex(us, off)
	if i < 0 {
		return off, off
	}
	u := us[i]
	v := e.unitView(u, off)
	if v.layout == nil || v.empty {
		return off, off
	}
	d, ok := v.disp.srcToDisp(off - u.lineStart)
	if !ok {
		return off, off
	}
	w := e.wordsForDisplay(v.disp.text)
	r, ok := w.index.WordAt(w.byteAtUTF16(d, false))
	if !ok {
		return off, off
	}
	a, z := w.utf16AtByte(r.Start, false), w.utf16AtByte(r.End, true)
	if !displayRangeHasSource(&v.disp, a, z) {
		return off, off
	}
	return v.disp.dispToSrc(a, false) + u.lineStart, v.disp.dispToSrc(z, true) + u.lineStart
}

func (e *Editor) wordEndAt(off int) int {
	return e.wordEndAtMode(off, false, false)
}

func (e *Editor) wordEndAtMode(off int, bigWord, next bool) int {
	e.layoutIfNeeded()
	us := e.units()
	i := e.unitIndex(us, off)
	if i < 0 {
		return off
	}
	u := us[i]
	v := e.unitView(u, off)
	if v.layout == nil || v.empty {
		return off
	}
	d, ok := v.disp.srcToDisp(off - u.lineStart)
	if !ok {
		return off
	}
	w := e.wordsForDisplay(v.disp.text)
	byteOff := w.byteAtUTF16(d, false)
	var z int
	var endOK bool
	if !bigWord {
		z, endOK = displayWordEnd(w, &v.disp, byteOff, next)
	} else {
		for range len(w.bigWords) + 1 {
			r, found := w.bigWordEnd(byteOff, next)
			if !found {
				return off
			}
			start, end := w.utf16AtByte(r.Start, false), w.utf16AtByte(r.End, true)
			if displayRangeHasSource(&v.disp, start, end) {
				z, endOK = end, true
				break
			}
			byteOff, next = r.End, true
		}
	}
	if !endOK {
		return off
	}
	return v.disp.dispToSrc(z, true) + u.lineStart
}

func (e *Editor) nextWordEndAt(off int) int {
	return e.wordEndAtMode(off, false, true)
}

func (e *Editor) nextWordEndAcrossUnitsMode(off int, bigWord bool) int {
	if z := e.wordEndAtMode(off, bigWord, true); z > off {
		return z
	}
	candidate := off
	for range len(e.units()) + 1 {
		next := e.stepWithWordMode(candidate, 1, true, bigWord)
		if next <= candidate {
			return off
		}
		candidate = next
		if z := e.wordEndAtMode(candidate, bigWord, false); z > candidate {
			return z
		}
	}
	return off
}

// nextWordEndAcrossUnits skips empty and structural leaves when an `e` motion
// needs the next word end beyond the current rendered text unit.
func displayWordEnd(w *displayWordIndex, d *display, off int, next bool) (int, bool) {
	var end int
	var ok bool
	if next {
		end, ok = w.index.NextEnd(off)
	} else {
		end, ok = w.index.WordEnd(off)
	}
	for n := 0; ok && n < w.index.TokenCount(); n++ {
		if end <= 0 {
			return 0, false
		}
		r, found := w.index.WordAt(end - 1)
		if !found {
			return 0, false
		}
		startU16, endU16 := w.utf16AtByte(r.Start, false), w.utf16AtByte(r.End, true)
		if displayRangeHasSource(d, startU16, endU16) {
			return endU16, true
		}
		// r.End already lies past the rejected decoration. WordEnd includes
		// a real token beginning exactly there; NextEnd would skip it.
		end, ok = w.index.WordEnd(r.End)
	}
	return 0, false
}

// blockRangeAt returns the source range of the leaf (or cell) at off.
func (e *Editor) blockRangeAt(off int) (int, int) {
	if t, _ := e.tableAtEdge(off); t != nil {
		return t.lineStart, blockEnd(e.Source(), t)
	}
	// A block that draws as one thing — a table, a fence, a picture of its
	// own — is one unit of the source too: the whole of its lines, markers
	// and all, and the break that ends the last of them. Its content's edges
	// would leave the pipe of a table's last row behind, and a block is not
	// half deleted.
	if l := e.leafAt(off); l != nil && e.wholeBlock(l) {
		z := blockEnd(e.Source(), l)
		if z < len(e.Source()) {
			z++
		}
		return l.lineStart, z
	}
	us := e.units()
	i := e.unitIndex(us, off)
	if i < 0 {
		return off, off
	}
	u := us[i]
	s, _ := e.edgeStop(u, false)
	z, _ := e.edgeStop(u, true)
	return s, z
}

// wholeBlock reports whether a leaf draws as one thing the caret does not
// stand inside of: a table, a fence of code or math, a block of HTML, a
// rule, a picture in a paragraph of its own.
func (e *Editor) wholeBlock(l *leaf) bool {
	switch l.kind {
	case kTable, kCode, kMath, kHTML, kRule:
		return true
	}
	return e.pictureBlock(l)
}

// pictureBlock reports whether a leaf is one picture and nothing else: its
// source is the picture's, and the display it makes is the object's place
// alone.
func (e *Editor) pictureBlock(l *leaf) bool {
	if l.kind != kPara {
		return false
	}
	if l.id >= 0 && e.p.Parser() != nil {
		return aloneImage(e.viewInlines(l), e.Source())
	}
	v := l.view
	return v != nil && len(v.disp.pieces) == 1 && v.disp.pieces[0].obj != nil
}

// isMac reports whether the keys are macOS's.
var isMac = runtime.GOOS == "darwin"

// blockEnd returns where the source of a leaf ends: after a fence's
// closing line, a heading's underline, a table's last row.
func blockEnd(src string, l *leaf) int {
	end := lineEndAt(src, max(l.end, l.lineStart))
	switch l.kind {
	case kCode, kMath:
		if l.kind == kCode && !l.fenced {
			return end
		}
		// The closing fence: the line after the last line of content.
		if end < len(src) {
			next := lineEndAt(src, end+1)
			if isClosingFence(src[end+1:next], l) {
				return next
			}
		}
	case kHeading:
		if l.setext && end < len(src) {
			return lineEndAt(src, end+1)
		}
	}
	return end
}

// isClosingFence reports whether a line closes a leaf's fence.
func isClosingFence(line string, l *leaf) bool {
	t := trimPrefixChars(line)
	if l.kind == kMath {
		return len(t) >= 2 && (t[:2] == "$$" || t[:2] == `\]`)
	}
	return len(t) >= 3 && (t[:3] == "```" || t[:3] == "~~~")
}

// trimPrefixChars trims the spaces and quotation markers a line of a
// container starts with.
func trimPrefixChars(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '>') {
		s = s[1:]
	}
	return s
}

// sourceChip returns the box of a block's source control: the little button
// at its top right corner that shows the block as the Markdown it is written
// as. It rides above the block's first line, as a block's tools do.
func (e *Editor) sourceChip(l *leaf) ui.Rect {
	h := e.lk.size * 1.5
	w := h * 1.7
	x := l.x + l.w - w - 2
	y := l.y + 3
	if e.gridBlock(l) {
		// A table's own box starts with its header row: the control goes
		// above it, where it covers nothing.
		y = l.y - h + 2
		if l.kind == kTable && l.geom != nil {
			// The top border belongs to column handles. Keep the source
			// control in the outer corner instead of covering the last one.
			x = l.x + tableViewportWidth(l) + 6
		}
	}
	return ui.Rect{X: x, Y: y, W: w, H: h}
}

// controlsBlock reports whether a block has a source control: a table, or
// a picture of its own — the blocks that draw as one thing.
func (e *Editor) controlsBlock(l *leaf) bool {
	return l.kind == kTable || e.pictureBlock(l)
}

// gridBlock reports whether the source control of a block rides above it:
// it does for a table's header row and for a picture's box.
func (e *Editor) gridBlock(l *leaf) bool {
	if e.controlsBlock(l) {
		return true
	}
	// A block of code has its own control at the same corner — the label
	// that reads "源码" or "图表" — so it does not get a tag as well.
	return l.kind != kCode && e.sourceOpen(l)
}

// sourceChipAt returns the block whose source control is at x, y.
func (e *Editor) sourceChipAt(x, y float32) *leaf {
	for _, l := range e.leaves {
		if !e.gridBlock(l) || !e.showsChip(l) {
			continue
		}
		if e.sourceChip(l).Contains(x, y) {
			return l
		}
	}
	return nil
}

// showsChip reports whether a block shows its source control: while the
// pointer is on the block, or while the block shows its source.
func (e *Editor) showsChip(l *leaf) bool {
	if e.tableHandleDrag != nil {
		return false
	}
	if e.sourceOK && l.lineStart == e.sourceAt {
		return true
	}
	// Rendered pictures have their own controls inside the actual image box.
	// Keep this older block chip only as the exit from an explicit source view.
	if e.pictureBlock(l) {
		return false
	}
	return e.hoverBlock == l.lineStart
}

// sourceOpen reports whether a block is showing its source: its control was
// used on it, and the caret is still in it (see Editor.sync).
func (e *Editor) sourceOpen(l *leaf) bool {
	return e.sourceOK && l.lineStart == e.sourceAt
}

// toggleSource shows a block as its source, or hides it again. It does not
// touch the caret: a control on a block is not the text, and using it twice
// leaves everything as it was found.
func (e *Editor) toggleSource(l *leaf) {
	e.actCaretStill(func() {
		if e.sourceOpen(l) {
			e.sourceOK = false
		} else {
			e.sourceOK, e.sourceAt = true, l.lineStart
		}
	})
	e.parsed = invalidParsedRevision
	e.leavesLaid = false
	e.invalidate()
}
