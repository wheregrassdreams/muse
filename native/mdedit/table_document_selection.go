package mdedit

// selectionPart is a semantic unit in a document selection. Text before and
// after a table retains its precise source range; complete table structures
// own their range and highlighting, just like an explicitly selected row.
type selectionPart struct {
	source    Range
	structure *tableSelection
}

func (e *Editor) selectionParts() []selectionPart {
	a, z := e.selection()
	if a == z {
		return nil
	}
	if e.tableSelection != nil {
		return []selectionPart{{source: Range{a, z}, structure: e.tableSelection}}
	}
	var parts []selectionPart
	start := a
	for _, t := range e.leaves {
		if t.kind != kTable || t.tab == nil {
			continue
		}
		s := e.tableStructureInRange(t, a, z)
		if s == nil {
			continue
		}
		lo, hi, _ := e.tableSelectionSourceRange(t, s)
		if start < lo {
			parts = append(parts, selectionPart{source: Range{start, lo}})
		}
		parts = append(parts, selectionPart{source: Range{lo, hi}, structure: s})
		start = hi
	}
	if start < z {
		parts = append(parts, selectionPart{source: Range{start, z}})
	}
	return parts
}

func (e *Editor) tableStructureSelection(t *leaf) *tableSelection {
	if t == nil {
		return nil
	}
	if s := e.tableSelection; s != nil {
		if s.lineStart == t.lineStart {
			return s
		}
		return nil
	}
	if e.block != nil {
		return nil
	}
	a, z := e.selection()
	return e.tableStructureInRange(t, a, z)
}

func (e *Editor) tableStructureInRange(t *leaf, a, z int) *tableSelection {
	end := blockEnd(e.src, t)
	// Content selections wholly inside a table remain content selections.
	if a > t.lineStart && z < end || a >= end || z <= t.lineStart {
		return nil
	}
	s := &tableSelection{lineStart: t.lineStart, axis: tableWhole, first: 0, last: len(t.tab.rows) - 1, right: e.caret >= e.anchor}
	if a > t.lineStart || z < end {
		s.axis, s.first, s.last = tableRows, -1, -1
		for row := range t.tab.rows {
			lo, hi, ok := e.tableSelectionSourceRange(t, &tableSelection{axis: tableRows, first: row, last: row})
			if ok && a <= lo && z >= hi {
				if s.first < 0 {
					s.first = row
				}
				s.last = row
			}
		}
		if s.first < 0 {
			return nil
		}
	}
	s.anchor, s.active = s.first, s.last
	if !s.right {
		s.anchor, s.active = s.active, s.anchor
	}
	s.activeRow = s.active
	return s
}

func (e *Editor) textSelectionForUnit(u *leaf) (int, int) {
	if e.block != nil {
		return 0, 0
	}
	if u.table != nil {
		if s := e.tableStructureSelection(u.table); s != nil {
			_, row, col := cellAt(u.table, u.start)
			if s.axis == tableWhole || s.axis == tableRows && row >= s.first && row <= s.last || s.axis == tableColumns && col >= s.first && col <= s.last {
				return 0, 0
			}
		}
	}
	return e.textSelectionRange()
}

// leaveTableBoundary carries the selection envelope into an adjacent document
// block. Only the active head moves; the table is retained as a semantic part.
func (e *Editor) leaveTableBoundary(t *leaf, dir int, extend bool) {
	if !e.hasGoal {
		if r, ok := e.caretRect(e.selectionFocus()); ok {
			e.goalX, e.hasGoal = r.X, true
		}
	}
	for i, l := range e.leaves {
		if l != t {
			continue
		}
		if i+dir < 0 || i+dir >= len(e.leaves) {
			anchor := e.anchor
			if e.phantomAtEdge(i, dir) {
				if extend {
					e.anchor = anchor
				}
				return
			}
		}
		e.enterLeaf(i+dir, dir, extend)
		return
	}
}
