package mdedit

import (
	"math"
	"muse/mdoc"
	"testing"
)

func TestHeadingSectionGapCountsEditableBlankLines(t *testing.T) {
	for _, blankCount := range []int{0, 1, 2, 3} {
		src := "前一段。\n"
		for i := 0; i < blankCount; i++ {
			src += "\n"
		}
		src += "### 🎨 速绘你的想法\n标题后正文。"
		h := paragraphEditingHarness(t, src)
		var above, heading, below *leaf
		blankHeight := float32(0)
		for _, l := range h.e.leaves {
			if l.kind == kBlank {
				blankHeight += l.h
				continue
			}
			if above == nil {
				above = l
			} else if heading == nil {
				heading = l
			} else {
				below = l
			}
		}
		gap := heading.y - (above.y + above.h)
		want := max(blankHeight, mdoc.HeadingTopMargin(heading.level, h.e.lk.size))
		if math.Abs(float64(gap-want)) > .1 {
			t.Fatalf("%d blanks: section gap=%v want=%v", blankCount, gap, want)
		}
		after := below.y - (heading.y + heading.h)
		if after >= gap {
			t.Fatalf("heading detached from following paragraph: above=%v below=%v", gap, after)
		}
		h.want(src)
		if blankCount == 1 {
			for _, l := range h.e.leaves {
				if l.kind == kBlank {
					h.caretAt(l.start)
					break
				}
			}
			h.shot("heading-section-spacing")
			h.want(src)
		}
	}
}

func TestHeadingSpacingKeepsDocumentStartAndTitleGroupCompact(t *testing.T) {
	for _, src := range []string{"# 文档标题\n## 小标题\n正文", "\n\n# 文档标题\n## 小标题\n正文"} {
		h := paragraphEditingHarness(t, src)
		var headings []*leaf
		for _, l := range h.e.leaves {
			if l.kind == kHeading {
				headings = append(headings, l)
			}
		}
		leading := float32(0)
		if src[0] == '\n' {
			leading = 2 * h.e.lk.size * h.e.lk.lh
		}
		if math.Abs(float64(headings[0].y-h.e.padTop-leading)) > .1 {
			t.Fatal("section margin inflated document top")
		}
		gap := headings[1].y - (headings[0].y + headings[0].h)
		if gap > h.e.lk.size*.65+.1 {
			t.Fatalf("title/subtitle group separated: %v", gap)
		}
		h.want(src)
	}
}
