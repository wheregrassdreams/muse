package editor

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	editormodel "muse/editor"
	tablemodel "muse/editor/blocks/table"
	tableui "muse/nativeui/components/editor/blocks/table"
)

// editorTableHost adapts the editor's private parser/history state to the
// table subsystem's small source-view and transaction contract.
type editorTableHost struct{ editor *Editor }

func (h editorTableHost) TableView(lineStart int) (tablemodel.DocumentView, bool) {
	e := h.editor
	e.sync()
	t := e.tableByLineStart(lineStart)
	if t == nil {
		return tablemodel.DocumentView{}, false
	}
	return documentViewForTable(e, t), true
}

func (h editorTableHost) TextSelection() editormodel.Selection {
	return h.editor.TransactionSelection()
}

func (h editorTableHost) DocumentSource() string { return h.editor.Source() }

func (h editorTableHost) Apply(transaction editormodel.Transaction) error {
	return h.editor.ApplyTransaction(transaction)
}

func documentViewForTable(e *Editor, t *leaf) tablemodel.DocumentView {
	view := tablemodel.DocumentView{
		LineStart:  t.lineStart,
		End:        blockEnd(e.Source(), t),
		Source:     e.Source(),
		Prefix:     e.Source()[t.lineStart:t.start],
		Rows:       make([][]editormodel.TextRange, len(t.tab.rows)),
		Lines:      make([]tablemodel.LineView, 0),
		Alignments: make([]tablemodel.Alignment, len(t.tab.align)),
	}
	view.ContinuationPrefix = blankMarkers(view.Prefix)
	for col, alignment := range t.tab.align {
		view.Alignments[col] = tablemodel.Alignment(alignment)
	}
	for _, line := range e.tableLines(t) {
		view.Lines = append(view.Lines, tablemodel.LineView{
			Range: editormodel.TextRange{Start: line[0], End: line[1]},
			Pipes: splitRow(e.Source(), line),
		})
	}
	for row, cells := range t.tab.rows {
		view.Rows[row] = make([]editormodel.TextRange, len(cells))
		for col, cell := range cells {
			view.Rows[row][col] = editormodel.TextRange{Start: cell.start, End: cell.end}
		}
	}
	return view
}

// editorTableUIHost is the narrow presentation port implemented by the
// editor composition layer. It exposes value snapshots and typed table
// actions, never the Editor or its parser leaves, to the table UI package.
type editorTableUIHost struct{ editor *Editor }

func (h editorTableUIHost) Frame() tableui.Frame {
	e := h.editor
	e.sync()
	frame := tableui.Frame{ScrollY: e.scroll.Y, ScopeID: e.uiScopeID}
	if drag := e.tableHandleDrag; drag != nil {
		frame.Drag = &tableui.DragPreview{
			LineStart: drag.lineStart, Axis: drag.axis, First: drag.first, Last: drag.last,
			Index: drag.index, Gap: drag.gap, Position: drag.position, Moving: drag.moving,
		}
	}
	for _, leaf := range e.paintVisibleSpan(e.scroll.Y-32, e.scroll.Y+e.viewH+32) {
		if leaf.kind != kTable || leaf.tab == nil || leaf.geom == nil || e.foldHidden(leaf) {
			continue
		}
		frame.Tables = append(frame.Tables, tableui.Table{
			LineStart:        leaf.lineStart,
			Bounds:           tableui.Rect{X: leaf.x, Y: leaf.y, W: leaf.geom.w, H: leaf.h},
			Viewport:         tableui.Rect{X: leaf.x, Y: leaf.y, W: tableViewportWidth(leaf), H: leaf.h},
			Columns:          append([]float32(nil), leaf.geom.colX...),
			Rows:             append([]float32(nil), leaf.geom.rowY...),
			CellCounts:       tableCellCounts(leaf.tab.rows),
			HorizontalOffset: leaf.horizontal.Offset,
			Alignments:       tableAlignmentsFromParser(leaf.tab.align),
		})
	}
	return frame
}

func tableCellCounts(rows [][]*leaf) []int {
	counts := make([]int, len(rows))
	for row := range rows {
		counts[row] = len(rows[row])
	}
	return counts
}

func tableFrameForLeaf(leaf *leaf) tableui.Table {
	if leaf == nil || leaf.tab == nil || leaf.geom == nil {
		return tableui.Table{}
	}
	return tableui.Table{
		LineStart:        leaf.lineStart,
		Bounds:           tableui.Rect{X: leaf.x, Y: leaf.y, W: leaf.geom.w, H: leaf.h},
		Viewport:         tableui.Rect{X: leaf.x, Y: leaf.y, W: tableViewportWidth(leaf), H: leaf.h},
		Columns:          leaf.geom.colX,
		Rows:             leaf.geom.rowY,
		CellCounts:       tableCellCounts(leaf.tab.rows),
		HorizontalOffset: leaf.horizontal.Offset,
	}
}

// tableCellIndexAt keeps existing Editor navigation callsites on the shared
// table UI geometry contract instead of maintaining a second hit-test.
func tableCellIndexAt(leaf *leaf, x, y float32) (row, col int, ok bool) {
	if leaf == nil || leaf.geom == nil || leaf.tab == nil {
		return -1, -1, false
	}
	cell, ok := tableui.CellAtPoint(tableFrameForLeaf(leaf), x, y)
	return cell.Row, cell.Col, ok
}

func tableHoverAxes(leaf *leaf, x, y float32) (row, col int) {
	if leaf == nil || leaf.geom == nil || leaf.tab == nil {
		return -1, -1
	}
	return tableui.HoverAxes(tableFrameForLeaf(leaf), ui.Point{X: x, Y: y})
}

func tableRowIndexAt(leaf *leaf, y float32) int {
	if leaf == nil || leaf.geom == nil || len(leaf.geom.rowY) < 2 || leaf.tab == nil {
		return 0
	}
	return min(tableui.RowAtPoint(leaf.geom.rowY, y-leaf.y), max(len(leaf.tab.rows)-1, 0))
}

func tableColumnIndexAt(leaf *leaf, x float32) int {
	if leaf == nil || leaf.geom == nil || len(leaf.geom.colX) < 2 || leaf.tab == nil {
		return 0
	}
	return min(tableui.ColumnAtPoint(leaf.geom.colX, x), max(leaf.tab.cols()-1, 0))
}

func (h editorTableUIHost) Dispatch(action tableui.Action) (tableui.Result, error) {
	e := h.editor
	switch action.Kind {
	case tableui.BeginHandleDrag:
		t := e.tableByLineStart(action.LineStart)
		if t == nil {
			return tableui.Result{}, nil
		}
		e.beginTableHandleDrag(t, tableAxis(action.Axis), action.HandleIndex, action.Extend, action.Pointer.X, action.Pointer.Y)
		return tableui.Result{Changed: e.tableHandleDrag != nil}, nil
	case tableui.MoveHandleDrag:
		e.updateTableHandleDrag(action.Pointer.X, action.Pointer.Y)
		return tableui.Result{Changed: e.tableHandleDrag != nil}, nil
	case tableui.FinishHandleDrag:
		dragging := e.tableHandleDrag != nil
		e.finishTableHandleDrag(action.Canceled)
		return tableui.Result{Changed: dragging}, nil
	case tableui.SelectCell:
		t := e.tableByLineStart(action.LineStart)
		if t == nil {
			return tableui.Result{}, nil
		}
		return tableui.Result{Changed: e.applyTableCellRegion(t, action.Anchor.Row, action.Anchor.Col, action.Focus.Row, action.Focus.Col)}, nil
	case tableui.SelectAxis:
		t := e.tableByLineStart(action.LineStart)
		if t == nil {
			return tableui.Result{}, nil
		}
		e.applyTableAxis(t, tableAxis(action.Axis), action.First, action.Last, action.Extend)
	case tableui.AlignColumn:
		e.finishBlockInput()
		e.layoutIfNeeded()
		var horizontal float32
		if t := e.tableByLineStart(action.LineStart); t != nil {
			horizontal = t.horizontal.Offset
			e.focusTableMenuCell(t, action.Focus.Row, action.Focus.Col)
		}
		before := e.tableNavigationState()
		changed, mapping, err := e.tableController.AlignColumn(editorTableHost{editor: e}, action.LineStart, action.Focus.Col, action.Alignment)
		if err != nil || !changed {
			return tableui.Result{Changed: changed, Mapping: mapping}, err
		}
		if before.Selection != nil || before.Bridge != nil {
			if before.Selection != nil {
				before.Selection.LineStart = mapping.Offset(before.Selection.LineStart)
			}
			if before.Bridge != nil {
				before.Bridge.LineStart = mapping.Offset(before.Bridge.LineStart)
				before.Bridge.SelectionAnchor = mapping.Offset(before.Bridge.SelectionAnchor)
			}
			selection := e.Session.Selection()
			before.TextSelection = editormodel.Selection{Anchor: selection.Anchor, Head: selection.Head}
			e.restoreTableNavigation(before)
			e.Session.UpdateLastHistoryAfter()
		}
		if e.block != nil {
			e.blockAnchorOffset = mapping.Offset(e.blockAnchorOffset)
			for i := range e.block.ranges {
				e.block.ranges[i].Start = mapping.Offset(e.block.ranges[i].Start)
				e.block.ranges[i].End = mapping.Offset(e.block.ranges[i].End)
			}
			e.block.revision++
		}
		e.preserveTableHorizontal(action.LineStart, horizontal)
		e.Focus()
		return tableui.Result{Changed: true, Mapping: mapping}, nil
	case tableui.AddRow:
		changed, err := e.tableController.AddRowBelow(editorTableHost{editor: e}, action.LineStart)
		return tableui.Result{Changed: changed}, err
	case tableui.AddColumn:
		changed, err := e.tableController.AddColumnRight(editorTableHost{editor: e}, action.LineStart)
		return tableui.Result{Changed: changed}, err
	case tableui.InsertRowAfter:
		changed, insertion, err := e.tableController.InsertRowAfter(editorTableHost{editor: e}, action.LineStart, action.First, action.Focus.Col)
		return tableui.Result{Changed: changed, Insertion: insertion}, err
	case tableui.InsertColumnBefore:
		changed, err := e.tableController.InsertColumnBefore(editorTableHost{editor: e}, action.LineStart, action.First)
		return tableui.Result{Changed: changed}, err
	case tableui.DeleteTable:
		t := e.tableByLineStart(action.LineStart)
		changed, insertion, err := e.tableController.DeleteTable(editorTableHost{editor: e}, action.LineStart)
		if changed && (e.leafAt(insertion) == nil || e.leafAt(insertion).lineStart != insertion) {
			e.tidyEmpty()
			if e.phantom == nil && t != nil {
				e.setPhantom(&phantom{at: insertion, post: "\n", before: true, conts: t.conts})
			}
		}
		return tableui.Result{Changed: changed, Insertion: insertion}, err
	case tableui.DeleteRows:
		changed, insertion, err := e.tableController.DeleteRows(editorTableHost{editor: e}, action.LineStart, action.First, action.Last, action.Whole, action.Change)
		return tableui.Result{Changed: changed, Insertion: insertion}, err
	case tableui.DeleteColumns:
		changed, insertion, err := e.tableController.DeleteColumns(editorTableHost{editor: e}, action.LineStart, action.First, action.Last, action.Change)
		return tableui.Result{Changed: changed, Insertion: insertion}, err
	case tableui.ReorderRows, tableui.ReorderColumns:
		changed, insertion, err := e.tableController.Reorder(editorTableHost{editor: e}, action.LineStart, action.Axis, action.First, action.Last, action.Gap)
		return tableui.Result{Changed: changed, Insertion: insertion}, err
	case tableui.ClearRegion:
		if action.Selection == nil {
			return tableui.Result{}, nil
		}
		changed, err := e.tableController.ClearRegion(editorTableHost{editor: e}, *action.Selection)
		return tableui.Result{Changed: changed}, err
	case tableui.ReplaceRegion:
		if action.Selection == nil {
			return tableui.Result{}, nil
		}
		changed, err := e.tableController.ReplaceRegion(editorTableHost{editor: e}, *action.Selection, action.Matrix)
		return tableui.Result{Changed: changed}, err
	case tableui.TypeColumnSelection:
		changed, insertion, err := e.tableController.TypeColumnSelection(editorTableHost{editor: e}, action.LineStart, action.First, action.Last, action.Text)
		return tableui.Result{Changed: changed, Insertion: insertion}, err
	case tableui.TypeBlock:
		changed, insertion, err := e.tableController.TypeBlock(editorTableHost{editor: e}, action.LineStart, action.Focus.Col, action.Text)
		return tableui.Result{Changed: changed, Insertion: insertion}, err
	case tableui.PasteRows:
		changed, insertion, err := e.tableController.PasteRows(editorTableHost{editor: e}, action.LineStart, action.First, action.Before, action.Matrix.Cells)
		return tableui.Result{Changed: changed, Insertion: insertion}, err
	case tableui.PasteColumns:
		changed, insertion, err := e.tableController.PasteColumns(editorTableHost{editor: e}, action.LineStart, action.First, action.Focus.Col, action.Matrix)
		return tableui.Result{Changed: changed, Insertion: insertion}, err
	case tableui.PasteBlock:
		changed, err := e.tableController.PasteBlock(editorTableHost{editor: e}, action.First, action.Text)
		return tableui.Result{Changed: changed}, err
	case tableui.BackspaceBlock:
		changed, err := e.tableController.BackspaceBlock(editorTableHost{editor: e}, action.LineStart, action.Focus.Col, action.Text)
		return tableui.Result{Changed: changed}, err
	case tableui.ResetNavigation:
		e.tableController.ResetNavigation()
		return tableui.Result{}, nil
	case tableui.ReplayNavigation:
		snapshot, ok := e.tableController.Replay(action.BeforeState, action.RowDelta, action.ColDelta)
		return tableui.Result{Found: ok, Snapshot: snapshot}, nil
	case tableui.RecordNavigation:
		e.tableController.Record(action.BeforeState, action.AfterState, action.RowDelta, action.ColDelta)
		return tableui.Result{}, nil
	default:
		return tableui.Result{}, fmt.Errorf("unknown table UI action %d", action.Kind)
	}
	return tableui.Result{}, nil
}

func (h editorTableUIHost) MenuInput(lineStart int, button, viewport ui.Rect, event ui.InputEvent) bool {
	e := h.editor
	t := e.tableByLineStart(lineStart)
	if t == nil {
		return false
	}
	if event.Kind == ui.InputScroll {
		return e.scrollTableOverlay(t, event)
	}
	if event.Kind == ui.InputPointerMove {
		e.hover(button.X+event.X-viewport.X, button.Y+event.Y-viewport.Y+e.scroll.Y)
	}
	return false
}

func (h editorTableUIHost) EdgeAction(action tableui.Action) (result tableui.Result, err error) {
	h.editor.actCaretStill(func() {
		result, err = h.Dispatch(action)
	})
	return result, err
}

func (h editorTableUIHost) Focus() { h.editor.Focus() }

func (e *Editor) tableUI() tableui.Component { return tableui.New(editorTableUIHost{editor: e}) }

var _ tableui.Host = editorTableUIHost{}
