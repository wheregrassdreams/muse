// Package markdown is the stable entry point for Markdown parsing and neutral
// render projection. Parsing models and syntax policies live in markdown/syntax;
// native widget drawing lives in nativeui/markdown.
package markdown

import (
	editormath "github.com/wheregrassdreams/muse-editor/markdown/math"
	"github.com/wheregrassdreams/muse-editor/markdown/render"
	"github.com/wheregrassdreams/muse-editor/markdown/syntax"
)

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
type InlineRun = render.InlineRun

// Doc preserves the stable syntax-facing fields while caching its render
// projection for native views.
type Doc struct {
	Source   string
	Blocks   []Block
	rendered *render.Document
}

const (
	AlignNone   = syntax.AlignNone
	AlignLeft   = syntax.AlignLeft
	AlignCenter = syntax.AlignCenter
	AlignRight  = syntax.AlignRight
)

func Parse(src string) *Doc          { return fromSyntax(syntax.Parse(src)) }
func ParseFile(src []byte) *Doc      { return Parse(string(src)) }
func ParsePlainText(src string) *Doc { return fromSyntax(syntax.ParsePlainText(src)) }
func CJKFriendly(src string) string  { return syntax.CJKFriendly(src) }

// TeXToUnicode returns a readable Unicode fallback for common TeX constructs.
func TeXToUnicode(expr string) string { return editormath.TeXToUnicode(expr) }

// Clone creates an independent model tree while sharing immutable strings.
func Clone(doc *Doc) *Doc {
	if doc == nil {
		return nil
	}
	return fromSyntax(syntax.Clone(&syntax.Doc{Source: doc.Source, Blocks: doc.Blocks}))
}

// Project builds a Markdown facade around an existing syntax-owned document
// without parsing its source again.
func Project(doc *syntax.Doc) *Doc { return fromSyntax(doc) }

func InlineRuns(src string) []InlineRun { return render.InlineRuns(src) }

func fromSyntax(doc *syntax.Doc) *Doc {
	if doc == nil {
		return nil
	}
	return &Doc{Source: doc.Source, Blocks: doc.Blocks, rendered: render.Project(doc)}
}

// PlainText returns the document's syntax-preserving plain-text projection.
func (d *Doc) PlainText() string {
	if d == nil {
		return ""
	}
	return (&syntax.Doc{Source: d.Source, Blocks: d.Blocks}).PlainText()
}

func (d *Doc) RenderDocument() *render.Document {
	if d == nil {
		return nil
	}
	if d.rendered != nil {
		return d.rendered
	}
	return render.Project(&syntax.Doc{Source: d.Source, Blocks: d.Blocks})
}

// spansText is retained inside the renderer for styling and layout code that
// needs a plain-text projection of styled inline spans.
func spansText(spans []Span) string { return syntax.InlineText(spans) }

// HeadingStyle is the backend-neutral visual scale for a Markdown heading.
func HeadingStyle(level int, body float32) render.HeadingStyle {
	return render.StyleHeading(level, body)
}

// HeadingSize returns a heading's font size at the given body size.
func HeadingSize(level int, body float32) float32 { return HeadingStyle(level, body).Size }

// HeadingWeight returns the standard weight for a heading level.
func HeadingWeight(level int) int { return HeadingStyle(level, 16).Weight }

// HeadingLineHeight returns the standard line-height multiplier for a heading.
func HeadingLineHeight(level int) float32 { return HeadingStyle(level, 16).LineHeight }

// HeadingTopMargin returns the decorative top margin for a heading.
func HeadingTopMargin(level int, body float32) float32 {
	return HeadingStyle(level, body).TopMargin
}
