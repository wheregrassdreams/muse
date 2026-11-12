package mdedit

import (
	"sort"
	"unicode/utf8"

	"muse/textnav"
)

// displayWordIndex adapts a display string's UTF-16 coordinates to the
// UTF-8 byte coordinates owned by textnav. The cache is keyed by exact display
// text so repeated motion/selection queries never re-tokenize a frame.
type displayWordIndex struct {
	text        string
	index       *textnav.Index
	byteOffsets []int
	u16Offsets  []int
	cost        int
}

const (
	wordIndexCacheEntries = 8
	wordIndexCacheBudget  = 256 << 10
)

func (e *Editor) wordsForDisplay(text string) *displayWordIndex {
	for n, cached := range e.wordIndexCache {
		if cached.text == text {
			copy(e.wordIndexCache[1:n+1], e.wordIndexCache[0:n])
			e.wordIndexCache[0] = cached
			return cached
		}
	}
	w := &displayWordIndex{text: text, index: textnav.New(text)}
	w.byteOffsets = make([]int, 1, utf8.RuneCountInString(text)+1)
	w.u16Offsets = make([]int, 1, utf8.RuneCountInString(text)+1)
	byteOff, unitOff := 0, 0
	for byteOff < len(text) {
		r, size := utf8.DecodeRuneInString(text[byteOff:])
		unitSize := 1
		if r > 0xffff {
			unitSize = 2
		}
		byteOff += size
		unitOff += unitSize
		w.byteOffsets = append(w.byteOffsets, byteOff)
		w.u16Offsets = append(w.u16Offsets, unitOff)
	}
	w.cost = len(text) + w.index.TokenCount()*24 + len(w.byteOffsets)*16
	if w.cost > wordIndexCacheBudget {
		// A single large paragraph is worth retaining because movement queries
		// revisit it on every key. Keep it alone; small indexes replace it.
		e.wordIndexCache = []*displayWordIndex{w}
		e.wordIndexCacheCost = w.cost
		return w
	}
	e.wordIndexCache = append([]*displayWordIndex{w}, e.wordIndexCache...)
	e.wordIndexCacheCost += w.cost
	for len(e.wordIndexCache) > wordIndexCacheEntries || e.wordIndexCacheCost > wordIndexCacheBudget {
		last := len(e.wordIndexCache) - 1
		e.wordIndexCacheCost -= e.wordIndexCache[last].cost
		e.wordIndexCache = e.wordIndexCache[:last]
	}
	return w
}

func (w *displayWordIndex) byteAtUTF16(off int, end bool) int {
	off = max(0, off)
	n := sort.Search(len(w.u16Offsets), func(n int) bool { return w.u16Offsets[n] >= off })
	if n == len(w.u16Offsets) {
		return w.byteOffsets[len(w.byteOffsets)-1]
	}
	if w.u16Offsets[n] == off || end || n == 0 {
		return w.byteOffsets[n]
	}
	return w.byteOffsets[n-1]
}

func (w *displayWordIndex) utf16AtByte(off int, end bool) int {
	off = max(0, off)
	n := sort.Search(len(w.byteOffsets), func(n int) bool { return w.byteOffsets[n] >= off })
	if n == len(w.byteOffsets) {
		return w.u16Offsets[len(w.u16Offsets)-1]
	}
	if w.byteOffsets[n] == off || end || n == 0 {
		return w.u16Offsets[n]
	}
	return w.u16Offsets[n-1]
}

// The keymaps translate keys into these editor operations. Geometry, source
// ranges, and history remain owned by the editor rather than by a keymap.
func (e *Editor) nativeHorizontal(dir int, extend bool) {
	if e.tableVisualLineActive() {
		e.moveTableVisualLineHorizontal(dir)
		return
	}
	e.hasGoal = false
	e.upstream = false
	e.moveHorizontal(dir, extend)
	if e.selectionInclusive && e.tableSelection == nil {
		if off := e.snapGraphemeBoundary(e.caret, dir); off != e.caret {
			e.setCaret(off, extend)
		}
	}
}

func (e *Editor) nativeVertical(dir int, extend bool) {
	e.layoutIfNeeded()
	from := e.unitAt(e.caret)
	targetRow := -1
	if from != nil && from.table == nil {
		v := e.unitView(from, e.caret)
		if v != nil && v.layout != nil && !v.empty {
			d, _ := v.disp.srcToDisp(e.caret - from.lineStart)
			row := lineOf(v.layout.Lines(), d, e.upstream) + dir
			if row >= 0 && row < len(v.layout.Lines()) {
				targetRow = row
			}
		}
	}
	e.vertical(dir, extend)
	if !e.hasGoal || dir == 0 {
		return
	}
	// Revealing source can reflow a soft-wrapped line. Keep the destination
	// display row fixed while refining X: following each candidate's new row
	// can walk back to the original line. If the reveal toggles between two
	// layouts, retain the last landing that actually belongs to the target row.
	e.layoutIfNeeded()
	u := e.unitAt(e.caret)
	if u == nil || u.table != nil {
		return
	}
	v := e.unitView(u, e.caret)
	if v == nil || v.layout == nil || v.empty {
		return
	}
	d, _ := v.disp.srcToDisp(e.caret - u.lineStart)
	row := lineOf(v.layout.Lines(), d, e.upstream)
	if u == from && targetRow >= 0 && targetRow < len(v.layout.Lines()) {
		row = targetRow
	}
	seen := map[int]bool{e.caret: true}
	for range 3 {
		if row >= len(v.layout.Lines()) {
			return // a reveal may have removed the last soft-wrapped row
		}
		ln := v.layout.Lines()[row]
		previous, upstream := e.caret, e.upstream
		d, _ := v.disp.srcToDisp(previous - u.lineStart)
		previousOnRow := lineOf(v.layout.Lines(), d, upstream) == row
		off := e.hitUnit(u, v, e.goalX, u.y+v.ty+ln.Rect.Y+ln.Rect.H/2)
		if off == e.caret {
			return
		}
		if seen[off] {
			e.upstream = upstream
			return
		}
		seen[off] = true
		e.setCaretKeepGoal(off, extend)
		e.layoutIfNeeded()
		v = e.unitView(u, e.caret)
		d, ok := v.disp.srcToDisp(e.caret - u.lineStart)
		if previousOnRow && (!ok || lineOf(v.layout.Lines(), d, e.upstream) != row) {
			e.setCaretKeepGoal(previous, extend)
			e.upstream = upstream
			return
		}
	}
}

// nativePrepareInsert transfers a structural or text selection to one source
// caret before a text command starts an ordinary editing transaction.
func (e *Editor) nativePrepareInsert() int {
	focus := e.selectionFocus()
	e.clearTableSelection()
	e.caret, e.anchor = focus, focus
	e.selectionInclusive = false
	e.selectionLinewise = false
	e.hasGoal = false
	e.upstream = false
	e.wantReveal = true
	e.invalidate()
	return focus
}

func (e *Editor) nativeSetCaret(off int, extend bool) {
	if e.tableVisualLineMode() && e.moveTableVisualLineCaret(off) {
		return
	}
	e.hasGoal = false
	e.upstream = false
	e.setCaret(off, extend)
}

func (e *Editor) sourceLineRange(off int) (int, int) {
	a := 0
	if off > 0 {
		for i := off - 1; i >= 0; i-- {
			if e.src[i] == '\n' {
				a = i + 1
				break
			}
		}
	}
	for i := off; i < len(e.src); i++ {
		if e.src[i] == '\n' {
			return a, i
		}
	}
	return a, len(e.src)
}

func (e *Editor) nativeLineStart(off int, firstNonSpace bool) int {
	a, _ := e.sourceLineRange(off)
	if firstNonSpace {
		for a < len(e.src) && (e.src[a] == ' ' || e.src[a] == '\t') {
			a++
		}
	}
	return a
}

func (e *Editor) nativeSelectLine(focus int) {
	if e.startTableVisualLine(focus) {
		return
	}
	e.anchor, e.caret = focus, focus
	e.selectionInclusive = false
	e.selectionLinewise = true
	e.finishNativeLineSelection()
}

func (e *Editor) finishNativeLineSelection() {
	e.hasGoal = false
	e.upstream = false
	e.wantReveal = true
	e.invalidate()
}

// nativeMoveLineSelection extends a linewise selection by source lines. Vim's
// V mode selects logical Markdown lines; ordinary j/k continue to use the
// displayed layout and its geometric column.
func (e *Editor) nativeMoveLineSelection(dir, count int) {
	if count < 1 {
		count = 1
	}
	if !e.tableVisualLineActive() && e.restoreTableVisualLineFromMixed(dir) {
		count--
		if count == 0 {
			return
		}
	}
	if e.tableVisualLineActive() {
		remaining := count
		for remaining > 0 && e.tableSelection != nil {
			t := e.tableSelectionLeaf()
			oldCaret := e.caret
			if !e.moveStructuralRow(dir, true) {
				break
			}
			if e.tableSelection != nil && t != nil {
				e.caret = e.tableVisualCaretAtRow(t, e.tableSelection.active, oldCaret)
			}
			remaining--
		}
		if e.tableSelection != nil || remaining == 0 {
			if e.tableSelection == nil {
				e.selectionLinewise = true
			}
			return
		}
		e.selectionLinewise = true
		// A counted move can leave the table and continue through ordinary
		// source lines; the table structure itself was handled row by row.
		count = remaining
	}
	focus := e.caret
	for range count {
		_, z := e.sourceLineRange(focus)
		if dir > 0 {
			if z >= len(e.src) {
				break
			}
			focus = z + 1
		} else {
			a, _ := e.sourceLineRange(focus)
			if a == 0 {
				break
			}
			focus = a - 1
		}
		focus = e.nextVisibleSourceOffset(focus, dir)
	}
	targetStart, _ := e.sourceLineRange(focus)
	e.caret = targetStart
	e.finishNativeLineSelection()
}

func (e *Editor) nativeLineRemainder() (int, int, bool) {
	_, z := e.sourceLineRange(e.caret)
	return e.caret, z, z > e.caret
}

func (e *Editor) nativeOpenLine(before bool) {
	a, z := e.sourceLineRange(e.caret)
	if before {
		e.begin(stepEdit)
		e.replace(a, a, "\n")
		e.commit(a, a)
	} else {
		e.begin(stepEdit)
		e.replace(z, z, "\n")
		e.commit(z+1, z+1)
	}
}

func (e *Editor) nativeSubstituteRange(count int, backwards bool) (int, int, bool) {
	if count < 1 {
		count = 1
	}
	a, z := e.caret, e.caret
	lineStart, lineEnd := e.sourceLineRange(e.caret)
	for range count {
		if backwards {
			if a <= lineStart {
				break
			}
			a = e.step(a, -1, false)
		} else {
			if z >= lineEnd {
				break
			}
			z = e.step(z, 1, false)
		}
	}
	return a, z, a != z
}

func (e *Editor) nativeLineEndAt(off int) int {
	_, z := e.sourceLineRange(off)
	return z
}
