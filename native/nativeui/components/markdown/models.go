// Package markdown adapts neutral Markdown models and media to the MyGo UI.
package markdown

import markdowncore "muse/markdown"

type (
	Doc       = markdowncore.Doc
	Kind      = markdowncore.Kind
	Span      = markdowncore.Span
	Cell      = markdowncore.Cell
	Item      = markdowncore.Item
	Block     = markdowncore.Block
	Align     = markdowncore.Align
	InlineRun = markdowncore.InlineRun
)

const (
	Paragraph = markdowncore.Paragraph
	Heading   = markdowncore.Heading
	Code      = markdowncore.Code
	Quote     = markdowncore.Quote
	List      = markdowncore.List
	Rule      = markdowncore.Rule
	HTML      = markdowncore.HTML
	Table     = markdowncore.Table
	Image     = markdowncore.Image
	Math      = markdowncore.Math

	AlignNone   = markdowncore.AlignNone
	AlignLeft   = markdowncore.AlignLeft
	AlignCenter = markdowncore.AlignCenter
	AlignRight  = markdowncore.AlignRight
)

func Parse(source string) *Doc          { return markdowncore.Parse(source) }
func TeXToUnicode(source string) string { return markdowncore.TeXToUnicode(source) }
