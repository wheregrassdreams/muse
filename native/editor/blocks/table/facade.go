// Package table keeps the Muse import path stable while the backend-neutral
// Markdown table model lives in muse-editor/markdown/table.
package table

import model "github.com/wheregrassdreams/muse-editor/markdown/table"

type (
	Host             = model.Host
	Axis             = model.Axis
	Cell             = model.Cell
	Grid             = model.Grid
	DocumentView     = model.DocumentView
	LineView         = model.LineView
	BoundaryAffinity = model.BoundaryAffinity
	Matrix           = model.Matrix
	Alignment        = model.Alignment
	Selection        = model.Selection
	SelectionOrigin  = model.SelectionOrigin
	ContentBridge    = model.ContentBridge
	VirtualEdge      = model.VirtualEdge
	VirtualEdit      = model.VirtualEdit
	VisualMode       = model.VisualMode
	VisualSelection  = model.VisualSelection
	Snapshot         = model.Snapshot
	Controller       = model.Controller
	Register         = model.Register
	ShortcutPlan     = model.ShortcutPlan
	SelectionPart    = model.SelectionPart
	RegionMove       = model.RegionMove
	EditMapping      = model.EditMapping
)

const (
	Rows                      = model.Rows
	Columns                   = model.Columns
	Whole                     = model.Whole
	Cells                     = model.Cells
	PreferNextCell            = model.PreferNextCell
	PreferPreviousCell        = model.PreferPreviousCell
	AlignmentNone             = model.AlignmentNone
	AlignmentLeft             = model.AlignmentLeft
	AlignmentCenter           = model.AlignmentCenter
	AlignmentRight            = model.AlignmentRight
	SelectionOriginTable      = model.SelectionOriginTable
	SelectionOriginText       = model.SelectionOriginText
	SelectionOriginVisualText = model.SelectionOriginVisualText
	VirtualBeforeCell         = model.VirtualBeforeCell
	VirtualAfterCell          = model.VirtualAfterCell
	VisualModeNone            = model.VisualModeNone
	VisualModeLine            = model.VisualModeLine
	VisualModeColumn          = model.VisualModeColumn
)

var (
	NewRegister              = model.NewRegister
	CellAtSourceOffset       = model.CellAtSourceOffset
	CopyRegion               = model.CopyRegion
	MoveOrder                = model.MoveOrder
	SelectionForTextRange    = model.SelectionForTextRange
	EffectiveSelection       = model.EffectiveSelection
	PromoteCellRegion        = model.PromoteCellRegion
	MoveRegion               = model.MoveRegion
	ProjectDocumentSelection = model.ProjectDocumentSelection
	ValidateVirtualEdit      = model.ValidateVirtualEdit
	Clone                    = model.Clone
	Equal                    = model.Equal
	PlanShortcut             = model.PlanShortcut
	TransactionFromPlan      = model.TransactionFromPlan
)
