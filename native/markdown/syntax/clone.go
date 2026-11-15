package syntax

// Clone returns an independent document tree. Source and span strings are
// immutable Go strings and are shared; every mutable slice is copied.
func Clone(doc *Doc) *Doc {
	if doc == nil {
		return nil
	}
	return &Doc{Source: doc.Source, Blocks: cloneBlocks(doc.Blocks)}
}

func cloneBlocks(source []Block) []Block {
	if len(source) == 0 {
		return nil
	}
	out := make([]Block, len(source))
	for i, block := range source {
		out[i] = block
		out[i].Inlines = append([]Span(nil), block.Inlines...)
		out[i].Align = append([]Align(nil), block.Align...)
		out[i].Children = cloneBlocks(block.Children)
		if len(block.Items) > 0 {
			out[i].Items = make([]Item, len(block.Items))
			for j, item := range block.Items {
				out[i].Items[j] = item
				out[i].Items[j].Inlines = append([]Span(nil), item.Inlines...)
				out[i].Items[j].Children = cloneBlocks(item.Children)
			}
		}
		if len(block.Rows) > 0 {
			out[i].Rows = make([][]Cell, len(block.Rows))
			for rowIndex, row := range block.Rows {
				out[i].Rows[rowIndex] = make([]Cell, len(row))
				for cellIndex, cell := range row {
					out[i].Rows[rowIndex][cellIndex] = append(Cell(nil), cell...)
				}
			}
		}
	}
	return out
}
