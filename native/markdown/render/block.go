package render

import "muse/markdown/syntax"

// Document is a presentation-oriented projection of a parsed Markdown tree.
type Document struct {
	Source string
	Blocks []Block
}

// Block contains syntax data plus render-time decisions that do not belong in
// the parser's semantic tree.
type Block struct {
	Kind      syntax.Kind
	Level     int
	Inlines   []syntax.Span
	HasMath   bool
	Text      string
	Lang      string
	Ordered   bool
	Start     int
	Items     []Item
	Children  []Block
	Rows      [][]Cell
	Align     []syntax.Align
	Src       string
	Alt       string
	Title     string
	ImgWidth  float32
	ImgHeight float32
	SourceStart int
	SourceEnd   int
	Open        bool
	Healed      bool
	Alert       syntax.Alert
}

// Item is one list item projected for presentation.
type Item struct {
	Marker   string
	Inlines  []syntax.Span
	HasMath  bool
	Children []Block
	Task     bool
	Checked  bool
	SourceStart int
	SourceEnd   int
}

// Cell is one table cell with precomputed inline math presence.
type Cell struct {
	Inlines []syntax.Span
	HasMath bool
}

// Project converts the syntax model into the render-owned view model. Inline
// slices and source strings are shared; only recursive block containers are
// allocated, so projections remain cheap and do not copy document text.
func Project(doc *syntax.Doc) *Document {
	if doc == nil {
		return nil
	}
	return &Document{Source: doc.Source, Blocks: ProjectBlocks(doc.Blocks)}
}

// ProjectBlocks converts blocks for presentation while sharing immutable
// inline and table-cell data from the parsed syntax tree.
func ProjectBlocks(source []syntax.Block) []Block {
	if len(source) == 0 {
		return nil
	}
	out := make([]Block, len(source))
	for i, b := range source {
		out[i] = Block{
			Kind: b.Kind, Level: b.Level, Inlines: b.Inlines, HasMath: spansHaveMath(b.Inlines),
			Text: b.Text, Lang: b.Lang, Ordered: b.Ordered, Start: b.Start,
			Src: b.Src, Alt: b.Alt, Title: b.Title, ImgWidth: b.ImgWidth, ImgHeight: b.ImgHeight,
			SourceStart: b.SourceStart, SourceEnd: b.SourceEnd, Open: b.Open, Healed: b.Healed, Alert: b.Alert,
			Children: ProjectBlocks(b.Children), Align: b.Align,
		}
		if len(b.Items) > 0 {
			out[i].Items = make([]Item, len(b.Items))
			for j, item := range b.Items {
				out[i].Items[j] = Item{
					Marker: item.Marker, Inlines: item.Inlines, HasMath: spansHaveMath(item.Inlines),
					Children: ProjectBlocks(item.Children), Task: item.Task, Checked: item.Checked,
					SourceStart: item.SourceStart, SourceEnd: item.SourceEnd,
				}
			}
		}
		if len(b.Rows) > 0 {
			out[i].Rows = make([][]Cell, len(b.Rows))
			for rowIndex, row := range b.Rows {
				out[i].Rows[rowIndex] = make([]Cell, len(row))
				for cellIndex, cell := range row {
					out[i].Rows[rowIndex][cellIndex] = Cell{Inlines: cell, HasMath: spansHaveMath(cell)}
				}
			}
		}
	}
	return out
}

func spansHaveMath(spans []syntax.Span) bool {
	for _, span := range spans {
		if span.Math {
			return true
		}
	}
	return false
}
