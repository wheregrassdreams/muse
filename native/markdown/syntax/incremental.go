package syntax

import incremental "muse/markdown/syntax/internal/incremental"

// IncrementalDocument is the syntax package's editable Markdown document.
// It exposes source offsets and parsed block/inline views to editor clients;
// callers should mutate it only through its Set/Edit methods.
type IncrementalDocument = incremental.Document

// IncrementalBlock and IncrementalInline are source-oriented syntax nodes.
// They are distinct from the immutable document projection in doc.go.
type IncrementalBlock = incremental.Block
type IncrementalInline = incremental.Inline
type IncrementalExtensions = incremental.Extensions
type IncrementalKind = incremental.Kind
type IncrementalInlineKind = incremental.InlineKind
type IncrementalTask = incremental.Task
type IncrementalAlign = incremental.Align
type IncrementalAlert = incremental.Alert

const (
	IncrementalExtTables        = incremental.ExtTables
	IncrementalExtTaskLists     = incremental.ExtTaskLists
	IncrementalExtStrikethrough = incremental.ExtStrikethrough
	IncrementalExtAutolinks     = incremental.ExtAutolinks
	IncrementalExtFootnotes     = incremental.ExtFootnotes
	IncrementalExtMath          = incremental.ExtMath
	IncrementalExtDollarMath    = incremental.ExtDollarMath
	IncrementalExtAlerts        = incremental.ExtAlerts
	IncrementalExtMentions      = incremental.ExtMentions
	IncrementalExtReferences    = incremental.ExtReferences
	IncrementalGFM              = incremental.GFM
	IncrementalDefault          = incremental.Default

	IncrementalRoot               = incremental.Root
	IncrementalParagraph          = incremental.Paragraph
	IncrementalHeading            = incremental.Heading
	IncrementalThematicBreak      = incremental.ThematicBreak
	IncrementalCodeBlock          = incremental.CodeBlock
	IncrementalHTMLBlock          = incremental.HTMLBlock
	IncrementalQuote              = incremental.Quote
	IncrementalList               = incremental.List
	IncrementalItem               = incremental.Item
	IncrementalTable              = incremental.Table
	IncrementalTableRow           = incremental.TableRow
	IncrementalTableCell          = incremental.TableCell
	IncrementalFootnoteDefinition = incremental.FootnoteDefinition
	IncrementalMathBlock          = incremental.MathBlock

	IncrementalNoTask         = incremental.NoTask
	IncrementalUnchecked      = incremental.Unchecked
	IncrementalChecked        = incremental.Checked
	IncrementalAlignNone      = incremental.AlignNone
	IncrementalAlignLeft      = incremental.AlignLeft
	IncrementalAlignCenter    = incremental.AlignCenter
	IncrementalAlignRight     = incremental.AlignRight
	IncrementalNoAlert        = incremental.NoAlert
	IncrementalAlertNote      = incremental.AlertNote
	IncrementalAlertTip       = incremental.AlertTip
	IncrementalAlertImportant = incremental.AlertImportant
	IncrementalAlertWarning   = incremental.AlertWarning
	IncrementalAlertCaution   = incremental.AlertCaution

	IncrementalText          = incremental.Text
	IncrementalSoftBreak     = incremental.SoftBreak
	IncrementalLineBreak     = incremental.LineBreak
	IncrementalCode          = incremental.Code
	IncrementalEmphasis      = incremental.Emphasis
	IncrementalStrong        = incremental.Strong
	IncrementalStrikethrough = incremental.Strikethrough
	IncrementalLink          = incremental.Link
	IncrementalImage         = incremental.Image
	IncrementalHTML          = incremental.HTML
	IncrementalMath          = incremental.Math
	IncrementalFootnoteRef   = incremental.FootnoteRef
	IncrementalMention       = incremental.Mention
	IncrementalReference     = incremental.Reference
)

// NewIncrementalDocument creates an editable source document using the
// requested syntax extensions.
func NewIncrementalDocument(extensions IncrementalExtensions) *IncrementalDocument {
	return incremental.New(extensions)
}

// ParseIncrementalSource parses source with the default editor syntax set.
func ParseIncrementalSource(source string) *IncrementalDocument {
	return incremental.Parse(source)
}

// IncrementalEntity decodes a Markdown character reference at the start of s.
func IncrementalEntity(s string) (n int, text string) { return incremental.Entity(s) }

// IncrementalPlainText returns plain text for a sequence of source inlines.
func IncrementalPlainText(inlines []IncrementalInline) string {
	return incremental.PlainText(inlines)
}
