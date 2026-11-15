package syntax

import "strings"

// writeBlocksText appends the plain text of blocks, one line each, to b.
func writeBlocksText(b *[]byte, blocks []Block, indent string) {
	for _, blk := range blocks {
		switch blk.Kind {
		case Paragraph, Heading:
			*b = append(*b, indent...)
			*b = append(*b, spansText(blk.Inlines)...)
			*b = append(*b, '\n')
		case Code:
			for _, line := range strings.Split(strings.TrimRight(blk.Text, "\n"), "\n") {
				*b = append(*b, indent...)
				*b = append(*b, line...)
				*b = append(*b, '\n')
			}
		case Quote:
			writeBlocksText(b, blk.Children, indent+"> ")
		case List:
			for _, it := range blk.Items {
				*b = append(*b, indent...)
				*b = append(*b, it.Marker...)
				*b = append(*b, ' ')
				*b = append(*b, spansText(it.Inlines)...)
				*b = append(*b, '\n')
				writeBlocksText(b, it.Children, indent+"  ")
			}
		case Rule:
			*b = append(*b, indent...)
			*b = append(*b, "---\n"...)
		case HTML:
			*b = append(*b, indent...)
			*b = append(*b, blk.Text...)
			*b = append(*b, '\n')
		case Table:
			for _, row := range blk.Rows {
				*b = append(*b, indent...)
				for i, cell := range row {
					if i > 0 {
						*b = append(*b, " | "...)
					}
					*b = append(*b, spansText(cell)...)
				}
				*b = append(*b, '\n')
			}
		}
	}
}

func spansText(spans []Span) string {
	var sb strings.Builder
	for _, s := range spans {
		sb.WriteString(s.Text)
	}
	return sb.String()
}

// InlineText joins the visible text from styled inline spans.
func InlineText(spans []Span) string { return spansText(spans) }
