package render

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestResolveInlineStyleAppliesHostPaletteAndSemanticOverrides(t *testing.T) {
	base := Style{
		Text: ui.RGB(20, 30, 40), Muted: ui.RGB(80, 90, 100), Heading: ui.RGB(120, 20, 30),
		Marker: ui.RGB(30, 120, 60), Link: ui.RGB(20, 60, 180), LinkHover: ui.RGB(100, 20, 180),
		InlineCodeText: ui.RGB(10, 10, 10), InlineCodeBackground: ui.RGB(220, 220, 220),
		MarkBackground: ui.RGB(240, 220, 80), Mono: "mono", Font: "body",
	}

	heading := ResolveInlineStyle(base, InlineStyle{Heading: true, Bold: true}, 16)
	if heading.Color != base.Heading || heading.Weight != 600 {
		t.Fatalf("heading span = %+v", heading)
	}
	invalid := ResolveInlineStyle(base, InlineStyle{Link: true, LinkInvalid: true}, 16)
	if !invalid.Underline || !invalid.DottedUnderline || invalid.Color == base.Link {
		t.Fatalf("invalid link span = %+v", invalid)
	}
	hover := ResolveInlineStyle(base, InlineStyle{Link: true, LinkInvalid: true, LinkHover: true}, 16)
	if hover.Color != base.LinkHover {
		t.Fatalf("hover color = %+v, want %+v", hover.Color, base.LinkHover)
	}
	code := ResolveInlineStyle(base, InlineStyle{Code: true}, 16)
	if code.Font != base.Mono || code.Size != 14 || code.Background != base.InlineCodeBackground || code.Color != base.InlineCodeText {
		t.Fatalf("code span = %+v", code)
	}
	marker := ResolveInlineStyle(base, InlineStyle{Code: true, Marker: true, Italic: true, Under: true}, 16)
	if marker.Font != base.Font || marker.Color != base.Marker || marker.Italic || marker.Underline || marker.Background != (ui.Color{}) {
		t.Fatalf("revealed marker span = %+v", marker)
	}
	sub := ResolveInlineStyle(base, InlineStyle{Sub: true}, 16)
	if sub.Size != 12 || sub.BaselineShift >= 0 {
		t.Fatalf("subscript span = %+v", sub)
	}
}
