package layout

import "math"

// Shared geometry factors used by Markdown editor layout and painting.
const (
	QuoteIndentEm     = float32(1.2)
	ListIndentEm      = float32(1.75)
	CalloutIndentEm   = float32(1.4)
	CodePaddingXEm    = float32(1.0)
	CodePaddingYEm    = float32(0.875)
	TablePaddingXEm   = float32(0.7)
	TablePaddingYEm   = float32(0.55)
	VirtualBlockLimit = 64
	VirtualImageLimit = 16
)

// ContainerKind is the layout category of one Markdown container.
type ContainerKind uint8

const (
	QuoteContainer ContainerKind = iota
	ListContainer
	FootnoteContainer
)

// ContainerIndent estimates how far a block moves into its column. Footnote
// label width is measured by the UI host because it depends on toolkit font
// shaping; the policy applies only its minimum indent.
func ContainerIndent(kind ContainerKind, fontSize, footnoteLabelWidth float32, callout bool) float32 {
	switch kind {
	case QuoteContainer:
		if callout {
			return fontSize * CalloutIndentEm
		}
		return fontSize * QuoteIndentEm
	case FootnoteContainer:
		return max(footnoteLabelWidth, fontSize*ListIndentEm)
	default:
		return fontSize * ListIndentEm
	}
}

// BlockKind describes layout policy categories, not parser-owned block types.
type BlockKind uint8

const (
	ParagraphBlock BlockKind = iota
	HeadingBlock
	CodeBlock
	MathBlock
	TableBlock
	HTMLBlock
	RuleBlock
	RawBlock
	BlankBlock
	PhantomBlock
)

// BlockMargins resolves default margins for a block. HeadingTop is supplied
// by the renderer typography policy; parser and pending-source facts are
// supplied by the editor host.
func BlockMargins(kind BlockKind, fontSize, headingTop float32, pending, callout bool) (top, bottom float32) {
	size := fontSize
	if callout {
		return size * .7, size * .7
	}
	switch kind {
	case HeadingBlock:
		if pending {
			return size * .5, size * .5
		}
		return headingTop, size * .35
	case CodeBlock, MathBlock, TableBlock, HTMLBlock, RuleBlock, RawBlock:
		return size * .6, size * .6
	case BlankBlock, PhantomBlock:
		return 0, 0
	default:
		return size * .5, size * .5
	}
}

// ShouldVirtualize applies document-size thresholds. Image presence is
// detected by the editor host because it depends on source/parser values.
func ShouldVirtualize(blockCount int, hasImages bool) bool {
	return blockCount >= VirtualBlockLimit || blockCount >= VirtualImageLimit && hasImages
}

// ShapeQuery is the immutable set of facts needed to decide whether one block
// should receive an exact UI layout during the current viewport pass.
type ShapeQuery struct {
	Virtualized                   bool
	Y, Height                     float32
	ViewportTop                   float32
	ViewportBottom                float32
	Overscan                      float32
	Caret, SourceStart, SourceEnd int
}

// ShouldShapeBlock keeps exact layout for viewport-intersecting blocks and
// the caret owner; unvirtualized documents are always shaped in full.
func ShouldShapeBlock(query ShapeQuery) bool {
	if !query.Virtualized {
		return true
	}
	if query.Y <= query.ViewportBottom+query.Overscan && query.Y+max(query.Height, query.Overscan) >= query.ViewportTop {
		return true
	}
	return query.Caret >= query.SourceStart && query.Caret <= query.SourceEnd
}

// EstimateCodeLines estimates wrapped rows for a code-like block from counts
// already collected by the editor host. The policy itself needs no source or
// parser objects.
func EstimateCodeLines(lineCount, byteCount int, width, fontSize float32, whole, padded bool) float32 {
	if !whole {
		width = max(width-2*fontSize*CodePaddingXEm, 40)
	}
	perLine := max(width/(fontSize*.875*.6), 8)
	wrapped := int(math.Ceil(float64(max(byteCount, 0)) / float64(perLine)))
	visualLines := max(lineCount, wrapped)
	height := float32(max(visualLines, 1)) * fontSize * .875 * 1.6
	if padded && !whole {
		height += 2 * fontSize * CodePaddingYEm
	}
	return height
}

// EstimateText returns a generous height estimate for prose whose display
// byte count was supplied by the editor host.
func EstimateText(byteCount int, width, fontSize, lineHeight float32, heading bool) float32 {
	line := fontSize * lineHeight
	if heading {
		line = fontSize * 1.35
	}
	perLine := max(width/fontSize*1.8, 8)
	lines := math.Ceil(float64(max(byteCount, 0)) / float64(perLine))
	return max(float32(lines), 1) * line
}

// EstimateTableRow estimates one table row after the host supplies the
// maximum source byte length of any cell in that row.
func EstimateTableRow(maxCellBytes, columns int, width, fontSize, lineHeight float32) float32 {
	columns = max(columns, 1)
	cellWidth := max(width/float32(columns)-2*fontSize*TablePaddingXEm, 10)
	perLine := max(cellWidth/(fontSize*.5), 4)
	lines := max(float32(math.Ceil(float64(max(maxCellBytes, 0))/float64(perLine))), 1)
	return lines*fontSize*lineHeight + 2*fontSize*TablePaddingYEm
}
