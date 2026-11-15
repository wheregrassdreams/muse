package render

import (
	"testing"

	"muse/markdown/syntax"
)

func TestProjectSharesInlineDataAndEnrichesNestedBlocks(t *testing.T) {
	source := &syntax.Doc{Source: "x", Blocks: []syntax.Block{{
		Kind:     syntax.Heading,
		Level:    2,
		Inlines:  []syntax.Span{{Text: "title"}, {Text: "x^2", Math: true}},
		Children: []syntax.Block{{Kind: syntax.Paragraph, Inlines: []syntax.Span{{Text: "nested"}}}},
		Items:    []syntax.Item{{Inlines: []syntax.Span{{Text: "item", Math: true}}}},
		Rows:     [][]syntax.Cell{{syntax.Cell{{Text: "cell", Math: true}}}},
	}}}
	projected := Project(source)
	if projected == nil || len(projected.Blocks) != 1 {
		t.Fatalf("projection = %+v", projected)
	}
	block := projected.Blocks[0]
	if !block.HasMath || len(block.Children) != 1 || block.Children[0].Kind != syntax.Paragraph {
		t.Fatalf("projected heading = %+v", block)
	}
	if !block.Items[0].HasMath || !block.Rows[0][0].HasMath {
		t.Fatalf("nested math was not projected: %+v", block)
	}
	if &block.Inlines[0] != &source.Blocks[0].Inlines[0] {
		t.Fatal("projection copied inline slices instead of sharing immutable data")
	}
}
