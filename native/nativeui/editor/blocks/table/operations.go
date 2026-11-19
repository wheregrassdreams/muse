package tableui

import (
	editormodel "muse/editor"
	tablemodel "muse/editor/blocks/table"
)

// DispatchDocumentAction applies table actions whose behavior is defined by
// the table document controller. The host supplies only a short-lived source
// view and an atomic transaction port; editor leaf/session state stays in the
// composition adapter.
func DispatchDocumentAction(controller *tablemodel.Controller, source tablemodel.Host, action Action) (Result, bool, error) {
	if controller == nil || source == nil {
		return Result{}, false, nil
	}
	switch action.Kind {
	case AddRow:
		changed, err := controller.AddRowBelow(source, action.LineStart)
		return Result{Changed: changed}, true, err
	case AddColumn:
		changed, err := controller.AddColumnRight(source, action.LineStart)
		return Result{Changed: changed}, true, err
	case InsertRowAfter:
		changed, insertion, err := controller.InsertRowAfter(source, action.LineStart, action.First, action.Focus.Col)
		return Result{Changed: changed, Insertion: insertion}, true, err
	case InsertColumnBefore:
		changed, err := controller.InsertColumnBefore(source, action.LineStart, action.First)
		return Result{Changed: changed}, true, err
	case DeleteRows:
		changed, insertion, err := controller.DeleteRows(source, action.LineStart, action.First, action.Last, action.Whole, action.Change)
		return Result{Changed: changed, Insertion: insertion}, true, err
	case DeleteColumns:
		changed, insertion, err := controller.DeleteColumns(source, action.LineStart, action.First, action.Last, action.Change)
		return Result{Changed: changed, Insertion: insertion}, true, err
	case ReorderRows, ReorderColumns:
		changed, insertion, err := controller.Reorder(source, action.LineStart, action.Axis, action.First, action.Last, action.Gap)
		return Result{Changed: changed, Insertion: insertion}, true, err
	case ClearRegion:
		if action.Selection == nil {
			return Result{}, true, nil
		}
		changed, err := controller.ClearRegion(source, *action.Selection)
		return Result{Changed: changed}, true, err
	case ReplaceRegion:
		if action.Selection == nil {
			return Result{}, true, nil
		}
		changed, err := controller.ReplaceRegion(source, *action.Selection, action.Matrix)
		return Result{Changed: changed}, true, err
	case TypeColumnSelection:
		changed, insertion, err := controller.TypeColumnSelection(source, action.LineStart, action.First, action.Last, action.Text)
		return Result{Changed: changed, Insertion: insertion}, true, err
	case TypeBlock:
		changed, insertion, err := controller.TypeBlock(source, action.LineStart, action.Focus.Col, action.Text)
		return Result{Changed: changed, Insertion: insertion}, true, err
	case PasteRows:
		changed, insertion, err := controller.PasteRows(source, action.LineStart, action.First, action.Before, action.Matrix.Cells)
		return Result{Changed: changed, Insertion: insertion}, true, err
	case PasteColumns:
		changed, insertion, err := controller.PasteColumns(source, action.LineStart, action.First, action.Focus.Col, action.Matrix)
		return Result{Changed: changed, Insertion: insertion}, true, err
	case PasteBlock:
		changed, err := controller.PasteBlock(source, action.First, action.Text)
		return Result{Changed: changed}, true, err
	case BackspaceBlock:
		changed, err := controller.BackspaceBlock(source, action.LineStart, action.Focus.Col, action.Text)
		return Result{Changed: changed}, true, err
	case ResetNavigation:
		controller.ResetNavigation()
		return Result{}, true, nil
	case ReplayNavigation:
		snapshot, ok := controller.Replay(action.BeforeState, action.RowDelta, action.ColDelta)
		return Result{Found: ok, Snapshot: snapshot}, true, nil
	case RecordNavigation:
		controller.Record(action.BeforeState, action.AfterState, action.RowDelta, action.ColDelta)
		return Result{}, true, nil
	default:
		return Result{}, false, nil
	}
}

// RegisterForSelection copies the semantic selection described by a detached
// source view into the table register. Selection shape and clipboard policy
// live with the table UI feature; the caller retains ownership of parser and
// editor state.
func RegisterForSelection(view tablemodel.DocumentView, selection tablemodel.Selection) *tablemodel.Register {
	if view.LineStart != selection.LineStart || len(view.Rows) == 0 || view.End < view.LineStart || view.End > len(view.Source) {
		return nil
	}
	whole := selection.Axis == tablemodel.Whole ||
		selection.Axis == tablemodel.Rows && selection.First == 0 && selection.Last == len(view.Rows)-1 ||
		selection.Axis == tablemodel.Columns && selection.First == 0 && selection.Last == maxColumns(view.Rows)-1
	if selection.CellRegion || selection.Axis == tablemodel.Cells {
		selection.CellRegion = true
		matrix, ok := tablemodel.CopyRegion(view, selection)
		if !ok {
			return nil
		}
		top, _, _, _ := selection.Bounds()
		return tablemodel.NewRegister(selection.Axis, matrix.Cells, matrix.Alignments, nil, whole, true, top == 0)
	}
	var rows [][]string
	var alignments []tablemodel.Alignment
	var header []string
	switch selection.Axis {
	case tablemodel.Rows, tablemodel.Whole:
		alignments = append(alignments, view.Alignments...)
		first, last := selection.First, selection.Last
		if selection.Axis == tablemodel.Whole {
			first, last = 0, len(view.Rows)-1
		} else if first > 0 && len(view.Rows[0]) > 0 {
			for _, cell := range view.Rows[0] {
				value, ok := sourceCell(view, cell)
				if !ok {
					return nil
				}
				header = append(header, value)
			}
		}
		first, last = clampRange(first, last, len(view.Rows))
		if first < 0 {
			return nil
		}
		for _, row := range view.Rows[first : last+1] {
			values := make([]string, len(row))
			for col, cell := range row {
				value, ok := sourceCell(view, cell)
				if !ok {
					return nil
				}
				values[col] = value
			}
			rows = append(rows, values)
		}
		return tablemodel.NewRegister(selection.Axis, rows, alignments, header, whole, false, false)
	case tablemodel.Columns:
		first, last := clampRange(selection.First, selection.Last, maxColumns(view.Rows))
		if first < 0 {
			return nil
		}
		for col := first; col <= last; col++ {
			alignment := tablemodel.AlignmentNone
			if col < len(view.Alignments) {
				alignment = view.Alignments[col]
			}
			alignments = append(alignments, alignment)
		}
		for _, row := range view.Rows {
			values := make([]string, last-first+1)
			for col := first; col <= last; col++ {
				if col < len(row) {
					value, ok := sourceCell(view, row[col])
					if !ok {
						return nil
					}
					values[col-first] = value
				}
			}
			rows = append(rows, values)
		}
		return tablemodel.NewRegister(selection.Axis, rows, alignments, nil, whole, false, false)
	default:
		return nil
	}
}

// SelectionOperationPlan resolves when a rectangular cell selection is also
// a complete row, column, or table selection. The editor host applies the
// resulting semantic value to its private session state.
type SelectionOperationPlan struct {
	Selection     tablemodel.Selection
	ClearCellArea bool
}

// PlanSelectionOperation applies the table selection-shape policy used by
// yank/delete/change. Partial cell rectangles remain cell selections; edits
// to those rectangles clear only their cells. Full-axis rectangles promote
// to their structural row/column/table equivalent.
func PlanSelectionOperation(selection tablemodel.Selection, grid tablemodel.Grid, operation string) SelectionOperationPlan {
	plan := SelectionOperationPlan{Selection: selection}
	if selection.Axis != tablemodel.Cells && !selection.CellRegion {
		return plan
	}
	top, left, bottom, right := selection.Bounds()
	fullRows := left == 0 && right == grid.Cols-1
	fullColumns := top == 0 && bottom == grid.Rows-1
	switch {
	case fullRows && fullColumns:
		plan.Selection.Axis = tablemodel.Whole
		plan.Selection.First, plan.Selection.Last = 0, max(grid.Rows-1, 0)
	case fullRows:
		plan.Selection.Axis = tablemodel.Rows
		plan.Selection.First, plan.Selection.Last = top, bottom
	case fullColumns:
		plan.Selection.Axis = tablemodel.Columns
		plan.Selection.First, plan.Selection.Last = left, right
	case operation != "y":
		plan.ClearCellArea = true
	}
	return plan
}

func sourceCell(view tablemodel.DocumentView, cell editormodel.TextRange) (string, bool) {
	if cell.Start < view.LineStart || cell.End < cell.Start || cell.End > view.End || cell.End > len(view.Source) {
		return "", false
	}
	return view.Source[cell.Start:cell.End], true
}

func maxColumns(rows [][]editormodel.TextRange) int {
	maximum := 0
	for _, row := range rows {
		maximum = max(maximum, len(row))
	}
	return maximum
}

func clampRange(first, last, size int) (int, int) {
	if size <= 0 {
		return -1, -1
	}
	first, last = max(0, min(first, size-1)), max(0, min(last, size-1))
	if first > last {
		first, last = last, first
	}
	return first, last
}
