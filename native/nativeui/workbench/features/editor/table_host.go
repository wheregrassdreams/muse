package editor

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	tablemodel "github.com/wheregrassdreams/muse-editor/markdown/table"
	editormodel "github.com/wheregrassdreams/muse-editor/text/session"
	tableui "muse/nativeui/workbench/features/editor/blocks/table"
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
	return h.editor.Session.Selection()
}

func (h editorTableHost) DocumentSource() string { return h.editor.Source() }

func (h editorTableHost) Apply(transaction editormodel.Transaction) error {
	_, err := h.editor.Session.ApplyTransactionWithOptions(transaction, editormodel.TransactionOptions{
		Group:          editormodel.HistoryGroupExplicit,
		BeforeMetadata: h.editor.Session.CaptureHistory(),
	})
	return err
}

type editorTableEdgeHost struct {
	editorTableHost
	extendFinalLine bool
}

func (h editorTableEdgeHost) Apply(transaction editormodel.Transaction) error {
	transaction.After = transaction.MapSelection(transaction.Before)
	return h.editorTableHost.Apply(transaction)
}

func (h editorTableEdgeHost) addColumnRight(lineStart int) (bool, error) {
	e := h.editor
	t := e.tableByLineStart(lineStart)
	if t == nil {
		return false, nil
	}
	lines := e.tableLines(t)
	if len(lines) < 2 {
		return false, nil
	}
	tx := editormodel.Transaction{Before: e.Session.Selection()}
	for row, line := range lines {
		pipes := splitRow(e.Source(), line)
		if len(pipes) == 0 {
			continue
		}
		text := "   |"
		if row == 1 {
			text = " --- |"
		}
		at := line[1]
		if strings.TrimSpace(e.Source()[pipes[len(pipes)-1]+1:at]) != "" {
			text = " |" + text
		}
		tx.Edits = append(tx.Edits, editormodel.TextEdit{
			Range: editormodel.TextRange{Start: at, End: at}, Text: text,
		})
	}
	if len(tx.Edits) == 0 {
		return false, nil
	}
	tx.After = tx.MapSelection(tx.Before)
	if err := h.Apply(tx); err != nil {
		return false, err
	}
	return true, nil
}

func (h editorTableEdgeHost) TableView(lineStart int) (tablemodel.DocumentView, bool) {
	view, ok := h.editorTableHost.TableView(lineStart)
	if !ok || len(view.Lines) == 0 {
		return view, ok
	}
	if !h.extendFinalLine {
		return view, true
	}
	last := &view.Lines[len(view.Lines)-1].Range
	if last.End < len(view.Source) && view.Source[last.End] == '\n' {
		last.End++
	} else if last.End+1 < len(view.Source) && view.Source[last.End] == '\r' && view.Source[last.End+1] == '\n' {
		last.End += 2
	}
	return view, true
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

func (h editorTableUIHost) Dispatch(action tableui.Action) (tableui.Result, error) {
	e := h.editor
	if result, handled, err := tableui.DispatchDocumentAction(&e.tableController, editorTableHost{editor: e}, action); handled {
		return result, err
	}
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
		before := e.tableNavigationState()
		var horizontal float32
		if t := e.tableByLineStart(action.LineStart); t != nil {
			horizontal = t.horizontal.Offset
			foreignSelection := before.Selection != nil && before.Selection.LineStart != action.LineStart ||
				before.Bridge != nil && before.Bridge.LineStart != action.LineStart
			if !foreignSelection {
				e.focusTableMenuCell(t, action.Focus.Row, action.Focus.Col)
			}
		}
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
		if action.Kind == tableui.AddRow || action.Kind == tableui.AddColumn {
			edgeHost := editorTableEdgeHost{
				editorTableHost: editorTableHost{editor: h.editor}, extendFinalLine: action.Kind == tableui.AddRow,
			}
			result, _, err = tableui.DispatchDocumentAction(&h.editor.tableController, edgeHost, action)
			if action.Kind == tableui.AddColumn && !result.Changed && err == nil {
				result.Changed, err = edgeHost.addColumnRight(action.LineStart)
			}
			return
		}
		result, err = h.Dispatch(action)
	})
	return result, err
}

func (h editorTableUIHost) Focus() { h.editor.Focus() }

func (e *Editor) tableUI() tableui.Component { return tableui.New(editorTableUIHost{editor: e}) }

// TableCommand routes the legacy context-menu command names into typed table
// actions. The controller remains the sole owner of table source edits.
func (e *Editor) TableCommand(cmd string) {
	e.layoutIfNeeded()
	t, row, col := e.cellPos(e.Caret())
	if t == nil {
		return
	}
	action := tableui.Action{LineStart: t.lineStart, Focus: tablemodel.Cell{Row: row, Col: col}}
	switch cmd {
	case "rowAbove":
		action.Kind, action.First = tableui.InsertRowAfter, max(row-1, 0)
	case "rowBelow":
		action.Kind, action.First = tableui.InsertRowAfter, row
	case "deleteRow":
		if row == 0 || len(t.tab.rows) <= 1 {
			return
		}
		action.Kind, action.First, action.Last = tableui.DeleteRows, row, row
	case "colLeft":
		action.Kind, action.First = tableui.InsertColumnBefore, col
	case "colRight":
		action.Kind, action.First = tableui.InsertColumnBefore, col+1
	case "deleteCol":
		if t.tab.cols() <= 1 {
			return
		}
		action.Kind, action.First, action.Last = tableui.DeleteColumns, col, col
	case "alignLeft", "alignCenter", "alignRight":
		action.Kind = tableui.AlignColumn
		action.Alignment = tablemodel.AlignmentLeft
		if cmd == "alignCenter" {
			action.Alignment = tablemodel.AlignmentCenter
		}
		if cmd == "alignRight" {
			action.Alignment = tablemodel.AlignmentRight
		}
	case "deleteTable":
		action.Kind = tableui.DeleteTable
	default:
		return
	}
	_, _ = e.tableUI().Dispatch(action)
}

var _ tableui.Host = editorTableUIHost{}
