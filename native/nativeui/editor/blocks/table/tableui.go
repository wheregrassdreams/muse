// Package tableui owns the editor's table interaction contract. It receives
// immutable geometry snapshots and sends typed table actions to its host; it
// never retains an editor, parser leaf, or mutable renderer object.
package tableui

import (
	"time"

	"github.com/egoist/mygo/ui"
	tablemodel "muse/editor/blocks/table"
	"muse/locales"
	tabletheme "muse/theme"
)

// Host supplies a borrowed visible frame and applies typed table actions.
// Implementations translate between UI geometry, editor state, and the table
// domain Controller. Frame slices must remain immutable until the current
// draw/input operation returns; the component does not copy them.
type Host interface {
	Frame() Frame
	Dispatch(Action) (Result, error)
	// EdgeAction applies an edge insertion while preserving the document caret
	// as one host transaction.
	EdgeAction(Action) (Result, error)
	Focus()
	MenuInput(lineStart int, button, viewport ui.Rect, event ui.InputEvent) bool
}

// Component is the feature-owned UI entry point over the host contract.
type Component struct{ host Host }

func New(host Host) Component { return Component{host: host} }

func (c Component) Frame() Frame {
	if c.host == nil {
		return Frame{}
	}
	return c.host.Frame()
}

func (c Component) Dispatch(action Action) (Result, error) {
	if c.host == nil {
		return Result{}, ErrHostUnavailable
	}
	return c.host.Dispatch(action)
}

// EdgesView presents table-edge controls from a detached viewport snapshot.
// The host preserves the document caret when a control inserts a row/column.
func (c Component) EdgesView(ctx *ui.Context, viewport ui.Rect, pointer ui.Point, frame Frame, dark bool) {
	if c.host == nil {
		return
	}
	for _, table := range frame.Tables {
		if len(table.Rows) == 0 || table.Bounds.H <= 0 {
			continue
		}
		// Edge controls follow the visible table content. In particular, the
		// trailing edge moves into the viewport as a wide table scrolls, which
		// keeps the add-column affordance reachable at the exposed right edge.
		x, y := viewport.X+table.Bounds.X-table.HorizontalOffset, viewport.Y+table.Bounds.Y-frame.ScrollY
		width, height := table.Bounds.W, table.Rows[len(table.Rows)-1]
		for _, column := range []bool{false, true} {
			bounds := ui.Rect{X: x, Y: y + height - 6, W: width, H: 24}
			label := locales.EditorAddRowBelow()
			if column {
				if x+width > viewport.X+table.Bounds.X+table.Viewport.W+1 {
					continue
				}
				bounds = ui.Rect{X: x + width - 6, Y: y, W: 24, H: height}
				label = locales.EditorAddColumnRight()
			}
			trigger := bounds
			if column {
				bounds.X += 10
				bounds.W -= 10
			} else {
				bounds.Y += 10
				bounds.H -= 10
				right := min(bounds.X+bounds.W, viewport.X+table.Bounds.X+table.Viewport.W)
				bounds.X = max(bounds.X, viewport.X+table.Bounds.X)
				bounds.W = right - bounds.X
			}
			right, bottom := min(bounds.X+bounds.W, viewport.X+viewport.W), min(bounds.Y+bounds.H, viewport.Y+viewport.H)
			bounds.X, bounds.Y = max(bounds.X, viewport.X), max(bounds.Y, viewport.Y)
			bounds.W, bounds.H = right-bounds.X, bottom-bounds.Y
			if bounds.W <= 0 || bounds.H <= 0 {
				continue
			}
			lineStart, controlBounds, hitBounds := table.LineStart, bounds, trigger
			ui.Overlay(ctx, func() {
				el := ui.Box(ctx.Key(edgeKey{frame.ScopeID, lineStart, column})).Absolute().Left(controlBounds.X).Top(controlBounds.Y).Width(controlBounds.W).Height(controlBounds.H).
					Label(label).Cursor(ui.CursorPointer).Tooltip(label).
					HandleInput(func(event ui.InputEvent) bool { return c.host.MenuInput(lineStart, controlBounds, viewport, event) })
				_, _, over := el.PointerPosition()
				near := hitBounds.Contains(viewport.X+pointer.X, viewport.Y+pointer.Y-frame.ScrollY)
				if over || near {
					ctx.After(40 * time.Millisecond)
					el.Draw(func(p *ui.Painter, r ui.Rect) {
						colors := tabletheme.MarkdownColors(dark)
						bg := ui.RGBA(colors.TableControlBackground.R, colors.TableControlBackground.G, colors.TableControlBackground.B, float32(colors.TableControlBackground.A)/255)
						ink := ui.RGBA(colors.TableControlInk.R, colors.TableControlInk.G, colors.TableControlInk.B, float32(colors.TableControlInk.A)/255)
						p.Fill(r, bg, min(r.W, r.H)/2)
						cx, cy := r.X+r.W/2, r.Y+r.H/2
						p.Fill(ui.Rect{X: cx - 4, Y: cy - 1, W: 8, H: 2}, ink, 1)
						p.Fill(ui.Rect{X: cx - 1, Y: cy - 4, W: 2, H: 8}, ink, 1)
					})
				}
				if el.Clicked() {
					kind := AddRow
					if column {
						kind = AddColumn
					}
					_, _ = c.host.EdgeAction(Action{Kind: kind, LineStart: lineStart})
					c.host.Focus()
				}
			})
		}
	}
}

type edgeKey struct {
	scopeID   uint64
	lineStart int
	column    bool
}

// Frame contains detached table geometry for one presentation pass. The Host
// creates the snapshot once and callers may borrow its slices until rendering
// and input dispatch for that pass finish. Pixel values are backend-neutral.
type Frame struct {
	Tables  []Table
	ScrollY float32
	ScopeID uint64
	Drag    *DragPreview
}

// DragPreview is the value-only presentation of an active handle drag. The
// editor host owns the session and revision checks; tableui only paints it.
type DragPreview struct {
	LineStart   int
	Axis        tablemodel.Axis
	First, Last int
	Index, Gap  int
	Position    float32
	Moving      bool
}

type Table struct {
	LineStart        int
	Bounds           Rect
	Viewport         Rect
	Columns          []float32
	Rows             []float32
	CellCounts       []int
	HorizontalOffset float32
	Alignments       []tablemodel.Alignment
}

type Rect struct{ X, Y, W, H float32 }

// CellAtPoint resolves a pointer within a table's local row/column grid.
// Columns and Rows contain ordered leading edges, including the trailing edge.
func CellAtPoint(frame Table, pointX, pointY float32) (tablemodel.Cell, bool) {
	if len(frame.Columns) < 2 || len(frame.Rows) < 2 {
		return tablemodel.Cell{}, false
	}
	x, y := pointX-frame.Bounds.X, pointY-frame.Bounds.Y
	if x < 0 || x >= frame.Viewport.W || y < 0 || y >= frame.Bounds.H {
		return tablemodel.Cell{}, false
	}
	x += frame.HorizontalOffset
	col := ColumnAtPoint(frame.Columns, x)
	row := RowAtPoint(frame.Rows, y)
	if row < 0 || col < 0 || row >= len(frame.CellCounts) || col >= frame.CellCounts[row] {
		return tablemodel.Cell{}, false
	}
	return tablemodel.Cell{Row: row, Col: col}, true
}

// RowAtPoint maps a local Y coordinate to the row whose vertical interval
// contains it. Edges include the trailing table boundary.
func RowAtPoint(edges []float32, y float32) int {
	for i := 0; i+1 < len(edges); i++ {
		if y < edges[i+1] {
			return i
		}
	}
	return max(len(edges)-2, 0)
}

// ColumnAtPoint maps a local X coordinate to the column whose horizontal
// interval contains it. Edges include the trailing table boundary.
func ColumnAtPoint(edges []float32, x float32) int {
	for i := 0; i+1 < len(edges); i++ {
		if x < edges[i+1] {
			return i
		}
	}
	return max(len(edges)-2, 0)
}

// HoverAxes reports the row/column handle at the exposed top and left edges.
func HoverAxes(frame Table, pointer ui.Point) (row, col int) {
	row, col = -1, -1
	cell, ok := CellAtPoint(frame, pointer.X, pointer.Y)
	if !ok {
		return row, col
	}
	if cell.Col == 0 && frame.HorizontalOffset == 0 {
		row = cell.Row
	}
	if cell.Row == 0 {
		col = cell.Col
	}
	return row, col
}

// CellMenus renders table-local alignment controls from immutable geometry.
// The callback surface only emits typed actions; it never receives an Editor.
func (c Component) CellMenus(ctx *ui.Context, viewport ui.Rect, pointer ui.Point, frame Frame, dark bool) {
	if c.host == nil {
		return
	}
	for _, table := range frame.Tables {
		if len(table.Rows) < 2 || len(table.Columns) < 2 {
			continue
		}
		for row := 0; row+1 < len(table.Rows); row++ {
			columns := len(table.Columns) - 1
			if row < len(table.CellCounts) && table.CellCounts[row] < columns {
				columns = table.CellCounts[row]
			}
			for col := 0; col < columns; col++ {
				right := table.Bounds.X - table.HorizontalOffset + table.Columns[col+1]
				if right > table.Bounds.X+table.Viewport.W || right-12 < table.Bounds.X {
					continue
				}
				top := table.Bounds.Y + table.Rows[row] - frame.ScrollY
				bottom := table.Bounds.Y + table.Rows[row+1] - frame.ScrollY
				r := ui.Rect{X: viewport.X + right - 12, Y: viewport.Y + (top+bottom)/2 - 12, W: 12, H: 24}
				if r.Y+r.H < viewport.Y || r.Y > viewport.Y+viewport.H {
					continue
				}
				ui.Overlay(ctx, func() {
					button := ui.ButtonBase(ctx.Key(cellMenuKey{frame.ScopeID, table.LineStart, row, col})).Absolute().Left(r.X).Top(r.Y).Width(r.W).Height(r.H).Padding(0).Radius(5).
						Label(locales.EditorTableCellAlignment(row+1, col+1)).Tooltip(locales.EditorTableColumnAlignment())
					button.HandleInput(func(event ui.InputEvent) bool {
						return c.host.MenuInput(table.LineStart, r, viewport, event)
					})
					button.Menu(func(menu *ui.Menu) {
						alignment := tablemodel.AlignmentNone
						if col < len(table.Alignments) {
							alignment = table.Alignments[col]
						}
						items := []struct {
							label string
							value tablemodel.Alignment
						}{{locales.EditorAlignLeftColumn(), tablemodel.AlignmentLeft}, {locales.EditorAlignCenterColumn(), tablemodel.AlignmentCenter}, {locales.EditorAlignRightColumn(), tablemodel.AlignmentRight}}
						for _, item := range items {
							checked := alignment == item.value || item.value == tablemodel.AlignmentLeft && alignment == tablemodel.AlignmentNone
							if menu.Item(item.label).Checked(checked).Chosen() {
								_, _ = c.Dispatch(Action{Kind: AlignColumn, LineStart: table.LineStart, Focus: tablemodel.Cell{Row: row, Col: col}, Alignment: item.value})
							}
						}
					})
					hoverCell, inside := CellAtPoint(table, pointer.X, pointer.Y)
					hot := inside && hoverCell.Row == row && hoverCell.Col == col
					if button.Hovered() || hot {
						ctx.After(40 * time.Millisecond)
						button.Draw(func(p *ui.Painter, bounds ui.Rect) {
							if button.Hovered() {
								background, _ := tableControlColors(dark)
								p.Fill(bounds, background, 5)
							}
							_, ink := tableControlColors(dark)
							for _, dy := range []float32{-4, 0, 4} {
								p.Fill(ui.Rect{X: bounds.X + bounds.W/2 - 1, Y: bounds.Y + bounds.H/2 + dy - 1, W: 2, H: 2}, ink, 1)
							}
						})
					}
				})
			}
		}
	}
}

type cellMenuKey struct {
	ScopeID                uint64
	LineStart, Row, Column int
}

func tableControlColors(dark bool) (background, ink ui.Color) {
	colors := tabletheme.MarkdownColors(dark)
	return ui.RGBA(colors.TableControlBackground.R, colors.TableControlBackground.G, colors.TableControlBackground.B, float32(colors.TableControlBackground.A)/255),
		ui.RGBA(colors.TableControlInk.R, colors.TableControlInk.G, colors.TableControlInk.B, float32(colors.TableControlInk.A)/255)
}

type ActionKind uint8

const (
	SelectCell ActionKind = iota + 1
	SelectAxis
	BeginHandleDrag
	MoveHandleDrag
	FinishHandleDrag
	AlignColumn
	AddRow
	AddColumn
	DeleteRows
	DeleteColumns
	ReorderRows
	ReorderColumns
	ClearRegion
	ReplaceRegion
	TypeColumnSelection
	TypeBlock
	PasteRows
	PasteColumns
	PasteBlock
	InsertRowAfter
	InsertColumnBefore
	DeleteTable
	BackspaceBlock
	ResetNavigation
	ReplayNavigation
	RecordNavigation
)

// Action is a typed request for one table operation. Fields irrelevant to
// Kind are ignored by the host.
type Action struct {
	Kind        ActionKind
	LineStart   int
	HandleIndex int
	Axis        tablemodel.Axis
	Anchor      tablemodel.Cell
	Focus       tablemodel.Cell
	Selection   *tablemodel.Selection
	First       int
	Last        int
	Gap         int
	Alignment   tablemodel.Alignment
	Matrix      tablemodel.Matrix
	Text        string
	Before      bool
	Whole       bool
	Change      bool
	Extend      bool
	Canceled    bool
	Pointer     Point
	RowDelta    int
	ColDelta    int
	BeforeState tablemodel.Snapshot
	AfterState  tablemodel.Snapshot
}

// Point uses editor-canvas coordinates: X is relative to the viewport's left
// edge and Y includes the vertical scroll offset. This matches source/leaf
// geometry consumed by the host adapter.
type Point struct{ X, Y float32 }

type Result struct {
	Changed   bool
	Insertion int
	Found     bool
	Snapshot  tablemodel.Snapshot
	Mapping   tablemodel.EditMapping
}

// ErrHostUnavailable is returned when a zero-value Component receives an
// action before a host is attached.
var ErrHostUnavailable = errHostUnavailable{}

type errHostUnavailable struct{}

func (errHostUnavailable) Error() string { return "table UI host is unavailable" }
