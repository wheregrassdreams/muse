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
	if s := e.tableSelection; s != nil && s.lineStart == t.lineStart {
		return s
	}
	if s := e.tableDocumentRegion; s != nil && s.lineStart == t.lineStart {
		a, z := e.selection()
		if a < blockEnd(e.Source(), t) && z > t.lineStart {
			return s
		}
	}
	if e.block != nil {
		return nil
	}
	selection := e.Session.Selection()
	selection.Head = e.Caret()
	s, ok := tablemodel.SelectionForTextRange(documentViewForTable(e, t), selection)
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
	selection := e.Session.Selection()
	selection.Head = e.Caret()
	views := make([]tablemodel.DocumentView, 0, 1)
	for _, l := range e.leaves {
		if l.kind == kTable && l.tab != nil {
			views = append(views, documentViewForTable(e, l))
		}
	}
	projected := tablemodel.ProjectDocumentSelection(selection, views)
	parts := make([]selectionPart, 0, len(projected))
	for _, part := range projected {
		parts = append(parts, selectionPart{source: part.Source, structure: tableSelectionFromSnapshot(part.Table)})
	}
	return parts
}
