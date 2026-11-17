package editor

import (
	editormodel "muse/editor"
	tablemodel "muse/editor/blocks/table"
)

type selectionPart struct {
	source    editormodel.TextRange
	structure *tableSelection
}

func cloneTableSelection(s *tableSelection) *tableSelection {
	if s == nil {
		return nil
	}
	copy := *s
	return &copy
}

func (e *Editor) tableStructureInRange(t *leaf, a, z int) *tableSelection {
	if t == nil || t.tab == nil {
		return nil
	}
	projected, ok := tablemodel.SelectionForTextRange(documentViewForTable(e, t), editormodel.Selection{Anchor: a, Head: z})
	if !ok {
		return nil
	}
	return tableSelectionFromSnapshot(projected)
}

func (e *Editor) tableStructureSelection(t *leaf) *tableSelection {
	if t == nil || t.tab == nil {
		return nil
	}
	selection := e.Session.Selection()
	selection.Head = e.Caret()
	s, ok := tablemodel.EffectiveSelection(
		documentViewForTable(e, t), selection,
		tableSelectionSnapshot(e.tableSelection),
		tableSelectionSnapshot(e.tableDocumentRegion),
		e.block != nil,
	)
	if !ok {
		return nil
	}
	return tableSelectionFromSnapshot(s)
}

func (e *Editor) textSelectionForUnit(u *leaf) (int, int) {
	if e.block != nil || u != nil && u.table != nil && e.tableStructureSelection(u.table) != nil {
		return 0, 0
	}
	return e.textSelectionRange()
}

// selectionParts projects a document selection into ordered source ranges and
// structural table regions. This adapter translates between editor parser
// leaves and the table domain model.
func (e *Editor) selectionParts() []selectionPart {
	e.sync()
	start, end := e.selection()
	if start == end {
		return nil
	}
	parts := make([]selectionPart, 0, len(e.leaves))
	startOffset := start
	for _, l := range e.leaves {
		if l.kind != kTable || l.tab == nil {
			continue
		}
		structure := e.tableStructureSelection(l)
		if structure == nil {
			continue
		}
		lo, hi := l.lineStart, blockEnd(e.Source(), l)
		if startOffset < lo {
			parts = append(parts, selectionPart{source: editormodel.TextRange{Start: startOffset, End: lo}})
		}
		parts = append(parts, selectionPart{source: editormodel.TextRange{Start: lo, End: hi}, structure: structure})
		startOffset = hi
	}
	if startOffset < end {
		parts = append(parts, selectionPart{source: editormodel.TextRange{Start: startOffset, End: end}})
	}
	return parts
}
