package editor

import (
	"muse/editor/input/vim"
	"github.com/wheregrassdreams/muse-editor/text/navigation"
)

// The keymaps translate keys into these editor operations. Geometry, source
// ranges, and history remain owned by the editor rather than by a keymap.
func (e *Editor) nativeHorizontal(dir int, extend bool) {
	e.clearVimImageRow()
	if e.tableVisualLineActive() {
		e.moveTableVisualLineHorizontal(dir)
		return
	}
	e.hasGoal = false
	e.upstream = false
	e.moveHorizontal(dir, extend)
	if e.Session.Selection().Inclusive && e.tableSelection == nil {
		if off := e.snapGraphemeBoundary(e.Caret(), dir); off != e.Caret() {
			e.setCaret(off, extend)
		}
	}
	if e.block != nil && e.vimController().Mode() == vim.ModeBlockVisual {
		e.blockChanged()
	}
}

func (e *Editor) nativeVertical(dir int, extend bool) {
	blockSelection := e.block != nil && e.vimController().Mode() == vim.ModeBlockVisual
	if blockSelection {
		defer func() {
			if e.hasGoal {
				e.blockChangedVertical(e.goalX)
			} else {
				e.blockChanged()
			}
		}()
	}
	e.layoutIfNeeded()
	from := e.unitAt(e.Caret())
	targetRow := -1
	if from != nil && from.table == nil {
		v := e.unitView(from, e.Caret())
		if v != nil && v.layout != nil && !v.empty {
			d, _ := v.disp.srcToDisp(e.Caret() - from.lineStart)
			row := lineOf(v.layout.Lines(), d, e.upstream) + dir
			if row >= 0 && row < len(v.layout.Lines()) {
				targetRow = row
			}
		}
	}
	e.vertical(dir, extend)
	if _, ok := e.vimImageRowState(); ok {
		return
	}
	if !e.hasGoal || dir == 0 {
		return
	}
	// Revealing source can reflow a soft-wrapped line. Keep the destination
	// display row fixed while refining X: following each candidate's new row
	// can walk back to the original line. If the reveal toggles between two
	// layouts, retain the last landing that actually belongs to the target row.
	e.layoutIfNeeded()
	u := e.unitAt(e.Caret())
	if u == nil || u.table != nil {
		return
	}
	v := e.unitView(u, e.Caret())
	if v == nil || v.layout == nil || v.empty {
		return
	}
	d, _ := v.disp.srcToDisp(e.Caret() - u.lineStart)
	row := lineOf(v.layout.Lines(), d, e.upstream)
	if u == from && targetRow >= 0 && targetRow < len(v.layout.Lines()) {
		row = targetRow
	}
	seen := map[int]bool{e.Caret(): true}
	for range 3 {
		if row >= len(v.layout.Lines()) {
			return // a reveal may have removed the last soft-wrapped row
		}
		ln := v.layout.Lines()[row]
		previous, upstream := e.Caret(), e.upstream
		d, _ := v.disp.srcToDisp(previous - u.lineStart)
		previousOnRow := lineOf(v.layout.Lines(), d, upstream) == row
		off := e.hitUnit(u, v, e.goalX, u.y+v.ty+ln.Rect.Y+ln.Rect.H/2)
		if off == e.Caret() {
			return
		}
		if seen[off] {
			e.upstream = upstream
			return
		}
		seen[off] = true
		e.setCaretKeepGoal(off, extend)
		e.layoutIfNeeded()
		v = e.unitView(u, e.Caret())
		d, ok := v.disp.srcToDisp(e.Caret() - u.lineStart)
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
	e.clearVimImageRow()
	focus := e.selectionFocus()
	e.clearTableSelectionSession()
	e.setSessionSelection(focus, focus)
	e.setSessionInclusive(false)
	e.setSessionLinewise(false)
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
	line := navigation.LineRange(e.Source(), off)
	return line.Start, line.End
}

func (e *Editor) nativeLineStart(off int, firstNonSpace bool) int {
	return navigation.LineStart(e.Source(), off, firstNonSpace)
}

func (e *Editor) nativeSelectLine(focus int) {
	if e.startTableVisualLine(focus) {
		return
	}
	e.setSessionSelection(focus, focus)
	e.setSessionInclusive(false)
	e.setSessionLinewise(true)
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
			oldCaret := e.Caret()
			if !e.moveStructuralRow(dir, true) {
				break
			}
			if e.tableSelection != nil && t != nil {
				e.setSessionHead(e.tableVisualCaretAtRow(t, e.tableSelection.active, oldCaret))
			}
			remaining--
		}
		if e.tableSelection != nil || remaining == 0 {
			if e.tableSelection == nil {
				e.setSessionLinewise(true)
			}
			return
		}
		e.setSessionLinewise(true)
		// A counted move can leave the table and continue through ordinary
		// source lines; the table structure itself was handled row by row.
		count = remaining
	}
	focus := e.Caret()
	for range count {
		_, z := e.sourceLineRange(focus)
		if dir > 0 {
			if z >= len(e.Source()) {
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
	e.setSessionHead(targetStart)
	e.finishNativeLineSelection()
}

func (e *Editor) nativeLineRemainder() (int, int, bool) {
	line := navigation.LineRange(e.Source(), e.Caret())
	return e.Caret(), line.End, line.End > e.Caret()
}

func (e *Editor) nativeOpenLine(before bool) {
	line := navigation.LineRange(e.Source(), e.Caret())
	if before {
		e.begin(stepEdit)
		e.replace(line.Start, line.Start, "\n")
		e.commit(line.Start, line.Start)
	} else {
		e.begin(stepEdit)
		e.replace(line.End, line.End, "\n")
		e.commit(line.End+1, line.End+1)
	}
}

func (e *Editor) nativeSubstituteRange(count int, backwards bool) (int, int, bool) {
	if count < 1 {
		count = 1
	}
	a, z := e.Caret(), e.Caret()
	line := navigation.LineRange(e.Source(), e.Caret())
	for range count {
		if backwards {
			if a <= line.Start {
				break
			}
			a = e.step(a, -1, false)
		} else {
			if z >= line.End {
				break
			}
			z = e.step(z, 1, false)
		}
	}
	return a, z, a != z
}

func (e *Editor) nativeLineEndAt(off int) int {
	return navigation.LineEnd(e.Source(), off)
}
