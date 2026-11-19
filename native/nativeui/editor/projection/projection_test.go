package projection

import (
	"testing"

	"github.com/wheregrassdreams/muse-editor/markdown/source"
	"github.com/wheregrassdreams/muse-editor/markdown/syntax"
)

func parseView(src string) source.SyntaxView {
	frontMatterEnd := FrontmatterEnd(src)
	doc := source.New(syntax.IncrementalDefault, nil)
	doc.Set(src, frontMatterEnd, nil)
	return doc.Parser()
}

func TestProjectorReturnsDetachedHeadingListTableAndBlankFacts(t *testing.T) {
	src := "# Heading\n\n- item\n\n| A | B |\n| --- | --- |\n| x | y |"
	var projector Projector
	blocks := projector.Build(src, parseView(src), 0)
	if len(blocks) < 5 {
		t.Fatalf("projected blocks = %d, want heading, blanks, list item and table", len(blocks))
	}
	if blocks[0].Kind != Heading || blocks[0].Level != 1 || src[blocks[0].Start:blocks[0].End] != "Heading" {
		t.Fatalf("heading projection = %+v", blocks[0])
	}
	var list, table *Block
	blankCount := 0
	for _, block := range blocks {
		switch block.Kind {
		case Paragraph:
			if len(block.Containers) > 0 && block.Containers[len(block.Containers)-1].Kind == Item {
				list = block
			}
		case Blank:
			blankCount++
		case TableBlock:
			table = block
		}
	}
	if list == nil || len(list.Containers) == 0 || list.Containers[len(list.Containers)-1].Kind != Item {
		t.Fatalf("list item container missing: %+v", list)
	}
	if blankCount != 2 {
		t.Fatalf("blank source lines projected = %d, want 2", blankCount)
	}
	if table == nil || table.Table == nil || len(table.Table.Rows) != 2 || len(table.Table.Rows[1]) != 2 {
		t.Fatalf("table projection = %+v", table)
	}
	cell := table.Table.Rows[1][0]
	if src[cell.Start:cell.End] != "x" || cell.TableOwner != table {
		t.Fatalf("detached cell projection = %+v", cell)
	}
}

func TestFrontmatterProjectionPreservesByteOffsets(t *testing.T) {
	src := "---\ntitle: Note\n---\n# Body"
	end := FrontmatterEnd(src)
	if end != len("---\ntitle: Note\n---") {
		t.Fatalf("frontmatter end = %d", end)
	}
	masked := MaskFrontmatter(src, end)
	if len(masked) != len(src) || masked[end:] != src[end:] || HideFrontmatterEnd(src) != end {
		t.Fatalf("frontmatter projection changed offsets: end=%d masked=%q", end, masked)
	}
}
