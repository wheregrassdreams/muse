package render

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	uieditor "github.com/egoist/mygo/ui/editor"
)

func TestProjectSourceTokensClipsAndFillsGaps(t *testing.T) {
	base := ui.Span{Color: ui.RGB(10, 20, 30)}
	palette := &uieditor.Theme{}
	palette.Tokens[uieditor.TokenKeyword] = uieditor.TokenStyle{Color: ui.RGB(200, 10, 20), Bold: true}
	palette.Tokens[uieditor.TokenString] = uieditor.TokenStyle{Color: ui.RGB(20, 180, 40)}
	palette.Tokens[uieditor.TokenNumber] = uieditor.TokenStyle{Italic: true}
	runs := ProjectSourceTokens(12, []SourceToken{
		{Start: -2, End: 2, Kind: TokenKeyword},
		{Start: 4, End: 7, Kind: TokenString},
		{Start: 6, End: 9, Kind: TokenComment},
		{Start: 10, End: 30, Kind: TokenNumber},
	}, base, palette)
	want := []SourceTokenRun{
		{Start: 0, End: 2, Style: ui.Span{Color: ui.RGB(200, 10, 20), Weight: 700}},
		{Start: 2, End: 4, Style: base},
		{Start: 4, End: 7, Style: ui.Span{Color: ui.RGB(20, 180, 40)}},
		{Start: 7, End: 10, Style: base},
		{Start: 10, End: 12, Style: ui.Span{Color: base.Color, Italic: true}},
	}
	if !reflect.DeepEqual(runs, want) {
		t.Fatalf("runs = %+v, want %+v", runs, want)
	}
}
