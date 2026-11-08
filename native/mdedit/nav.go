package mdedit

import (
	"runtime"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
)

// The caret moves through the display: a grapheme, a word or a line at a
// time in what the user sees, mapped back to the source. Syntax hidden at
// the caret's old place may show at its new one, so every move ends on an
// offset the display at that offset can show (normalize).

// unit is a part of the document the caret moves through: a leaf, or a
// cell of a table.
func (e *Editor) units() []*leaf {
	if e.unitRev == e.rev && e.unitList != nil {
		return e.unitList
	}
	out := e.unitList[:0]
	if cap(out) < len(e.leaves) {
		out = make([]*leaf, 0, len(e.leaves))
	}
	for _, l := range e.leaves {
		if l.kind == kTable {
			for _, row := range l.tab.rows {
				out = append(out, row...)
			}
			continue
		}
		out = append(out, l)
	}
	e.unitList, e.unitRev = out, e.rev
	return e.unitList
}

// unitIndex returns the index in units of the one holding the caret at
// off, or -1. A line of its own owns its offset: a leaf may end or begin
// there too, but the caret there is the line's.
func (e *Editor) unitIndex(us []*leaf, off int) int {
	// One pass over the units: a line of its own may come after the leaf
	// that ends where it starts, and it wins when it does.
	line, cover := -1, -1
	for i, u := range us {
		if u.isLine() {
			if line < 0 && off == u.start {
				line = i
			}
			continue
		}
		if cover >= 0 {
			continue
		}
		if off >= leafLo(u) && off <= u.end {
			if u.kind == kCode && u.fenced && off > u.infoEnd && off < u.start {
				continue
			}
			cover = i
		}
	}
	if line >= 0 {
		return line
	}
	return cover
}

// tableOf returns the table leaf a cell is in.
func (e *Editor) tableOf(cell *leaf) *leaf { return cell.table }

// isCell reports whether a unit is a table's cell.
func (e *Editor) isCell(u *leaf) bool {
	return u.kind == kPara && u.conts == nil && e.tableOf(u) != nil
}

// unitView returns the view of a unit as it shows with the caret at off.
func (e *Editor) unitView(u *leaf, off int) *leafView {
	if u.view == nil || e.revealFor(u, off) != e.revealAt(u) {
		header, align := false, parser.AlignNone
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
	if off < l.lineStart || off > l.end {
		if l.kind == kCode && l.fenced && off >= l.infoStart && off <= l.infoEnd {
			return off - l.lineStart
		}
		return -1
	}
	return off - l.lineStart
}

// stopOK reports whether the caret can be at off, in the display of its
// unit as it would show with the caret there.
func (e *Editor) stopOK(u *leaf, off int) bool {
	if u.isLine() {
		return off == u.start
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
			if leafLo(u) >= off {
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
		cells := l.tab.cells()
		if len(cells) == 0 {
			return 0, false
		}
		if end {
			return e.edgeStop(cells[len(cells)-1], true)
		}
		return e.edgeStop(cells[0], false)
	}
	if l.isLine() {
		return l.start, true
	}
	off := l.start
	if end {
		off = l.end
	}
	if l.kind == kCode && l.fenced && len(l.lines) == 0 {
		// A fence without lines: its info string.
		if end {
			return l.infoEnd, true
		}
		return l.infoEnd, true
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
	e.layoutIfNeeded()
	us := e.units()
	i := e.unitIndex(us, off)
	if i < 0 {
		return e.gapStop(us, off, dir)
	}
	u := us[i]
	if u.kind == kCode && u.fenced && dir < 0 && off == u.start && e.revealFor(u, off) >= 0 && !(off >= u.infoStart && off <= u.infoEnd) {
		// From the start of the code to its fence's info string.
		return u.infoEnd
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
		if s, ok := e.wordStepIn(u, off, dir); ok {
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
	stops := e.unitView(u, off).disp.stops()
	best := -1
	for _, s := range stops {
		at := s + u.lineStart
		if dir > 0 && at > off && (best < 0 || at < best) {
			best = at
		}
		if dir < 0 && at < off && (best < 0 || at > best) {
			best = at
		}
	}
	if best < 0 {
		return 0, false
	}
	if !e.stopOK(u, best) {
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
	nd := wordStep(v.disp.text, d, dir)
	if nd == d {
		return 0, false
	}
	c := v.disp.dispToSrc(nd, dir > 0) + u.lineStart
	if dir > 0 && c <= off || dir < 0 && c >= off {
		return 0, false
	}
	return e.normalize(c, dir), true
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

// wordStep returns the display offset of the next word's end (dir > 0) or
// the previous word's start, from d: an ideograph or a kana is a word of
// its own.
func wordStep(text string, d, dir int) int {
	rs, us := runesWithUnits(text)
	// k is the rune at display offset d.
	k := 0
	for k < len(rs) && us[k] < d {
		k++
	}
	if dir > 0 {
		for k < len(rs) && !isWordRune(rs[k]) {
			k++
		}
		if k < len(rs) && isIdeograph(rs[k]) {
			return us[k+1]
		}
		for k < len(rs) && isWordRune(rs[k]) && !isIdeograph(rs[k]) {
			k++
		}
		return us[k]
	}
	for k > 0 && !isWordRune(rs[k-1]) {
		k--
	}
	if k > 0 && isIdeograph(rs[k-1]) {
		return us[k-1]
	}
	for k > 0 && isWordRune(rs[k-1]) && !isIdeograph(rs[k-1]) {
		k--
	}
	return us[k]
}

// wordAround returns the display range of the word, or run of other
// characters, at display offset d.
func wordAround(text string, d int) (int, int) {
	rs, us := runesWithUnits(text)
	if len(rs) == 0 {
		return 0, 0
	}
	k := 0
	for k < len(rs) && us[k+1] <= d {
		k++
	}
	if k >= len(rs) {
		k = len(rs) - 1
	}
	if isIdeograph(rs[k]) {
		return us[k], us[k+1]
	}
	cls := func(r rune) int {
		switch {
		case r == '\n':
			return 3
		case isWordRune(r) && !isIdeograph(r):
			return 0
		case unicode.IsSpace(r):
			return 1
		}
		return 2
	}
	c := cls(rs[k])
	a, z := k, k+1
	for a > 0 && cls(rs[a-1]) == c && c != 3 && !isIdeograph(rs[a-1]) {
		a--
	}
	for z < len(rs) && cls(rs[z]) == c && c != 3 && !isIdeograph(rs[z]) {
		z++
	}
	return us[a], us[z]
}

// runesWithUnits returns the runes of s and the UTF-16 offset of each,
// with one more for the end.
func runesWithUnits(s string) ([]rune, []int) {
	rs := make([]rune, 0, len(s))
	us := make([]int, 0, len(s)+1)
	u := 0
	for _, r := range s {
		rs = append(rs, r)
		us = append(us, u)
		if r >= 0x10000 {
			u += 2
		} else {
			u++
		}
	}
	us = append(us, u)
	return rs, us
}

func isWordRune(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

func isIdeograph(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r)
}

// ---------------------------------------------------------------------------
// Geometry

// caretRect returns the caret's box at off, in the element's coordinates.
func (e *Editor) caretRect(off int) (ui.Rect, bool) {
	e.layoutIfNeeded()
	us := e.units()
	i := e.unitIndex(us, off)
	if i < 0 {
		return ui.Rect{}, false
	}
	u := us[i]
	v := e.unitView(u, off)
	if v.layout == nil {
		return ui.Rect{X: u.x, Y: u.y, W: 1, H: u.h}, true
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
	r.X += u.x + v.tx
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

// vertical moves the caret up (dir < 0) or down lines, keeping its x.
func (e *Editor) vertical(dir int, extend bool) {
	e.layoutIfNeeded()
	r, ok := e.caretRect(e.caret)
	if !ok {
		return
	}
	if !e.hasGoal {
		e.goalX, e.hasGoal = r.X, true
	}
	us := e.units()
	i := e.unitIndex(us, e.caret)
	if i < 0 {
		return
	}
	u := us[i]
	v := e.unitView(u, e.caret)
	// Within the unit: the line above or below.
	if v.layout != nil && !v.empty {
		d, _ := v.disp.srcToDisp(e.caret - u.lineStart)
		lines := v.layout.Lines()
		k := lineOf(lines, d, e.upstream)
		if k+dir >= 0 && k+dir < len(lines) {
			ln := lines[k+dir]
			off := e.hitUnit(u, v, e.goalX, u.y+v.ty+ln.Rect.Y+ln.Rect.H/2)
			e.setCaretKeepGoal(off, extend)
			return
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
	if li+dir < 0 || li+dir >= len(e.leaves) {
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
	e.enterLeaf(li+dir, dir, extend)
}

// enterLeaf moves the caret into leaf li from above (dir > 0) or below, at
// the goal's x: its first line or its last.
func (e *Editor) enterLeaf(li, dir int, extend bool) {
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
		if strings.HasSuffix(e.src, "\n") {
			// The source already ends with a line to type on.
			return false
		}
		end := blockEnd(e.src, l)
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
	d := v.layout.IndexAt(ui.Point{X: x - u.x - v.tx, Y: y - u.y - v.ty})
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
		return e.hitUnit(best, e.unitView(best, e.caret), x, y)
	}
	v := e.unitView(l, e.caret)
	if l.kind == kCode && v.label != "" {
		// The label of the language: to the info string.
	}
	return e.hitUnit(l, v, x, y)
}

// leafAtPoint returns the leaf the point's y is on: the last one starting
// above it, or the nearer of two when the point falls between them.
func (e *Editor) leafAtPoint(y float32) *leaf {
	if len(e.leaves) == 0 {
		return nil
	}
	li := 0
	for k, l := range e.leaves {
		if y >= l.y {
			li = k
		}
	}
	// Between two leaves, the nearer.
	if li+1 < len(e.leaves) {
		l, n := e.leaves[li], e.leaves[li+1]
		if y > l.y+l.h && n.y-y < y-(l.y+l.h) {
			li++
		}
	}
	return e.leaves[li]
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
		if p.kind != pAtom || p.obj == nil || p.s1 <= p.s0 {
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
		if l.isLine() || l.view == nil {
			continue
		}
		if y >= l.y && y <= l.y+l.h {
			return 0, false
		}
		if l.y+l.h < y {
			prev = l
		} else if next == nil && l.y > y {
			next = l
		}
	}
	if prev == nil || next == nil || next.start <= prev.end {
		return 0, false
	}
	n := strings.Count(e.src[prev.end:next.start], "\n")
	if n < 2 {
		return 0, false
	}
	lineH := e.lk.size * e.lk.lh
	k := int((y - (prev.y + prev.h)) / lineH)
	k = max(0, min(k, n-2))
	at := prev.end + 1 + k
	if at >= next.start {
		return 0, false
	}
	return at, true
}

func (e *Editor) setCaretAtPoint(off int, y float32, extend bool) {
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
	a, z := wordAround(v.disp.text, d)
	return v.disp.dispToSrc(a, false) + u.lineStart, v.disp.dispToSrc(z, true) + u.lineStart
}

// blockRangeAt returns the source range of the leaf (or cell) at off.
func (e *Editor) blockRangeAt(off int) (int, int) {
	// A block that draws as one thing — a table, a fence, a picture of its
	// own — is one unit of the source too: the whole of its lines, markers
	// and all, and the break that ends the last of them. Its content's edges
	// would leave the pipe of a table's last row behind, and a block is not
	// half deleted.
	if l := e.leafAt(off); l != nil && e.wholeBlock(l) {
		z := blockEnd(e.src, l)
		if z < len(e.src) {
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
	if v := l.view; v != nil {
		// A paragraph that is one picture: the picture is the block.
		if n := len(v.disp.pieces); n == 1 && v.disp.pieces[0].obj != nil {
			return true
		}
	}
	return false
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
	}
	return ui.Rect{X: x, Y: y, W: w, H: h}
}

// gridBlock reports whether a block draws as a grid, or as the source of
// one: the blocks that have a source control.
func (e *Editor) gridBlock(l *leaf) bool {
	if l.kind == kTable {
		return true
	}
	return e.sourceOK && l.lineStart == e.sourceAt
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
	if e.sourceOK && l.lineStart == e.sourceAt {
		return true
	}
	return e.hoverBlock == l.lineStart
}

// toggleSource shows a block as its source, or hides it again, and puts the
// caret in it: the block is what the caret then edits.
func (e *Editor) toggleSource(l *leaf) {
	if e.sourceOK && e.sourceAt == l.lineStart {
		e.sourceOK = false
	} else {
		e.sourceOK, e.sourceAt = true, l.lineStart
	}
	e.setCaret(l.start, false)
	e.rev++
	e.parsed = 0
	e.invalidate()
}
