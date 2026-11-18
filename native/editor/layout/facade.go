// Package layout preserves Muse's historical import path for backend-neutral
// document geometry and viewport policy.
package layout

import model "github.com/wheregrassdreams/muse-editor/text/layout"

const (
	QuoteIndentEm     = model.QuoteIndentEm
	ListIndentEm      = model.ListIndentEm
	CalloutIndentEm   = model.CalloutIndentEm
	CodePaddingXEm    = model.CodePaddingXEm
	CodePaddingYEm    = model.CodePaddingYEm
	TablePaddingXEm   = model.TablePaddingXEm
	TablePaddingYEm   = model.TablePaddingYEm
	VirtualBlockLimit = model.VirtualBlockLimit
	VirtualImageLimit = model.VirtualImageLimit
)

type ContainerKind = model.ContainerKind

const (
	QuoteContainer    = model.QuoteContainer
	ListContainer     = model.ListContainer
	FootnoteContainer = model.FootnoteContainer
)

type BlockKind = model.BlockKind

const (
	ParagraphBlock = model.ParagraphBlock
	HeadingBlock   = model.HeadingBlock
	CodeBlock      = model.CodeBlock
	MathBlock      = model.MathBlock
	TableBlock     = model.TableBlock
	HTMLBlock      = model.HTMLBlock
	RuleBlock      = model.RuleBlock
	RawBlock       = model.RawBlock
	BlankBlock     = model.BlankBlock
	PhantomBlock   = model.PhantomBlock
)

type ShapeQuery = model.ShapeQuery
type Rect = model.Rect
type HorizontalFrame = model.HorizontalFrame
type HorizontalDrag = model.HorizontalDrag
type ViewportBand = model.ViewportBand
type VirtualIndex = model.VirtualIndex
type PageJumpAlignment = model.PageJumpAlignment

const (
	PageJumpNearest   = model.PageJumpNearest
	PageJumpTopMargin = model.PageJumpTopMargin
	PageJumpCenter    = model.PageJumpCenter
	PageJumpTop       = model.PageJumpTop
	PageJumpBottom    = model.PageJumpBottom
)

var (
	ContainerIndent     = model.ContainerIndent
	BlockMargins        = model.BlockMargins
	ShouldVirtualize    = model.ShouldVirtualize
	ShouldShapeBlock    = model.ShouldShapeBlock
	EstimateCodeLines   = model.EstimateCodeLines
	EstimateText        = model.EstimateText
	EstimateTableRow    = model.EstimateTableRow
	ClampOffset         = model.ClampOffset
	RevealOffset        = model.RevealOffset
	VirtualRows         = model.VirtualRows
	HorizontalBarRects  = model.HorizontalBarRects
	ScrollOffset        = model.ScrollOffset
	BeginHorizontalDrag = model.BeginHorizontalDrag
	DragHorizontal      = model.DragHorizontal
	VirtualBand         = model.VirtualBand
)
