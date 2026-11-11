package mdoc

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestBearTypographyAccentAndNeutralCode(t *testing.T) {
	accents := []ui.Color{ui.Hex("#c99a18"), ui.Hex("#3979d1")}
	for _, dark := range []bool{false, true} {
		links := make(map[ui.Color]bool)
		inks := make(map[ui.Color]bool)
		backgrounds := make(map[ui.Color]bool)
		for _, accent := range accents {
			links[InlineLinkColorFor(accent, dark)] = true
			background, ink := InlineCodeColorsFor(accent, dark)
			inks[ink] = true
			backgrounds[background] = true
		}
		if len(links) != len(accents) || len(inks) != 1 || len(backgrounds) != 1 {
			t.Fatalf("dark=%v: links=%v should follow accent while code stays neutral (inks=%v backgrounds=%v)", dark, links, inks, backgrounds)
		}
		_, ink := InlineCodeColorsFor(accents[0], dark)
		if ink != BodyTextColor(dark) {
			t.Fatalf("dark=%v: code ink=%v, want neutral body color %v", dark, ink, BodyTextColor(dark))
		}
	}
	if got, want := BodyTextColor(false), ui.Hex("#444442"); got != want {
		t.Fatalf("light body = %v, want %v", got, want)
	}
	if got, want := BodyTextColor(true), ui.Hex("#d6d6d2"); got != want {
		t.Fatalf("dark body = %v, want %v", got, want)
	}
	if got, want := HeadingWeight(1), 600; got != want {
		t.Fatalf("heading weight = %d, want %d", got, want)
	}
	if got := InlineLinkColor(false); got != InlineLinkColorFor(ui.Color{}, false) {
		t.Fatalf("default link color %v differs from default palette %v", got, InlineLinkColorFor(ui.Color{}, false))
	}
}
