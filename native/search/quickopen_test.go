package search

import (
	"reflect"
	"testing"

	"muse/document/types"
)

func TestRankQuickOpenRanksPathTitleAndAliasWithRuneHighlights(t *testing.T) {
	items := []QuickOpenItem{
		{Path: "z/other.md", PathText: "笔记/other.md", Title: "无关", Metadata: types.Metadata{Aliases: []string{"快速开始"}}},
		{Path: "a/guide.md", PathText: "笔记/guide.md", Title: "快速开始指南"},
		{Path: "b/guide.md", PathText: "笔记/guide.md", Title: "无关"},
	}
	got := RankQuickOpen("快速", items)
	if len(got) != 2 {
		t.Fatalf("matches = %+v", got)
	}
	if got[0].Path != "a/guide.md" || got[1].Path != "z/other.md" {
		t.Fatalf("rank order = %+v", got)
	}
	if !reflect.DeepEqual(got[0].TitleMatches, []int{0, 1}) {
		t.Fatalf("title rune offsets = %v", got[0].TitleMatches)
	}
	if len(got[1].TitleMatches) != 0 {
		t.Fatalf("alias-only hit has title highlights: %+v", got[1])
	}
}

func TestRankQuickOpenPathHighlightsAndStableEmptyQuery(t *testing.T) {
	items := []QuickOpenItem{
		{Path: "z/notes.md", PathText: "folder/notes.md", Title: "Notes"},
		{Path: "a/notes.md", PathText: "folder/notes.md", Title: "Notes"},
	}
	got := RankQuickOpen("f/n", items)
	if len(got) != 2 || got[0].Path != "a/notes.md" || !reflect.DeepEqual(got[0].PathMatches, []int{0, 6, 7}) {
		t.Fatalf("path ranks = %+v", got)
	}
	got = RankQuickOpen(" ", items)
	if len(got) != 2 || got[0].Path != "a/notes.md" || got[1].Path != "z/notes.md" {
		t.Fatalf("empty query order = %+v", got)
	}
}
