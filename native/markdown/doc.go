// Package markdown is the stable entry point for Markdown parsing and native
// rendering. Parsing models and syntax policies live in markdown/syntax.
package markdown

import "muse/markdown/syntax"

type Kind = syntax.Kind

const (
	Paragraph = syntax.Paragraph
	Heading   = syntax.Heading
	Code      = syntax.Code
	Quote     = syntax.Quote
	List      = syntax.List
	Rule      = syntax.Rule
	HTML      = syntax.HTML
	Table     = syntax.Table
	Image     = syntax.Image
	Math      = syntax.Math
)

type Span = syntax.Span
type Cell = syntax.Cell
type Item = syntax.Item
type Block = syntax.Block
type Align = syntax.Align
type Doc = syntax.Doc

const (
	AlignNone   = syntax.AlignNone
	AlignLeft   = syntax.AlignLeft
	AlignCenter = syntax.AlignCenter
	AlignRight  = syntax.AlignRight
)

func Parse(src string) *Doc          { return syntax.Parse(src) }
func ParseFile(src []byte) *Doc      { return syntax.ParseFile(src) }
func ParsePlainText(src string) *Doc { return syntax.ParsePlainText(src) }
func CJKFriendly(src string) string  { return syntax.CJKFriendly(src) }

// spansText is retained inside the renderer for styling and layout code that
// needs a plain-text projection of styled inline spans.
func spansText(spans []Span) string { return syntax.InlineText(spans) }
