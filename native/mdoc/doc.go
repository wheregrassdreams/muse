// Package mdoc is Muse's Markdown document model and its native renderer.
//
// Parse turns Markdown source into a flat block tree that a plugin, the
// editor or the AI layer can read and rewrite; View draws that tree with
// the mygo ui package, so a document shows in a native window with no
// webview and no frontend build.
//
// The model is deliberately shallow: every block is a value, and the tree
// is small enough to serialize, diff and hand to a model as context.
package mdoc

// Kind is the sort of a block.
type Kind uint8

const (
	// Paragraph is a run of inline text.
	Paragraph Kind = iota
	// Heading is a section title; Level is 1..6.
	Heading
	// Code is a fenced or indented code block; Text holds it, Lang its info string.
	Code
	// Quote is a block quote; Children holds the quoted blocks.
	Quote
	// List is an ordered or bullet list; Items holds its items.
	List
	// Rule is a thematic break.
	Rule
	// HTML is raw HTML the parser left alone; Text holds it.
	HTML
	// Table is a GFM table; Rows and Align describe it.
	Table
	// Image is a paragraph holding just one image; Src, Alt and Title describe it.
	Image
	// Math is a display-math block; Text holds the TeX.
	Math
)

// Span is a run of inline text with one style. The zero value is plain text.
type Span struct {
	Text      string
	Bold      bool
	Italic    bool
	Code      bool
	Strike    bool
	Link      string // non-empty for a link target
	Math      bool   // inline TeX
	SoftBreak bool   // an ordinary source newline, distinguishable from a hard break
}

// Cell is one table cell: a run of styled spans.
type Cell []Span

// Item is one item of a List. Inlines is its lead-in text; Children holds
// any nested blocks, such as a second paragraph or a sub-list.
type Item struct {
	Marker   string
	Inlines  []Span
	Children []Block
	Task     bool // a GFM task-list item
	Checked  bool
}

// Block is one block of a document. Which fields apply depends on Kind.
type Block struct {
	Kind      Kind     // what this block is
	Level     int      // Heading: 1..6
	Inlines   []Span   // Paragraph, Heading
	Text      string   // Code, HTML
	Lang      string   // Code: info string
	Ordered   bool     // List
	Start     int      // List: first number of an ordered list
	Items     []Item   // List
	Children  []Block  // Quote
	Rows      [][]Cell // Table: rows of cells
	Align     []Align  // Table: per-column alignment
	Src       string   // Image: destination
	Alt       string   // Image: alt text
	Title     string   // Image: title
	ImgWidth  float32  // Image: explicit width in DIPs, from HTML
	ImgHeight float32  // Image: explicit height in DIPs, from HTML
}

// Align is a table column's alignment.
type Align uint8

const (
	AlignNone Align = iota
	AlignLeft
	AlignCenter
	AlignRight
)

// Doc is a parsed document.
type Doc struct {
	// Source is the Markdown the document was parsed from.
	Source string
	// Blocks is the document's top-level blocks.
	Blocks []Block
}

// PlainText reports the document's text with all inline styling dropped,
// one block per line, which is what a plain-text view or a model's context
// wants.
func (d *Doc) PlainText() string {
	var b []byte
	writeBlocksText(&b, d.Blocks, "")
	return string(b)
}
