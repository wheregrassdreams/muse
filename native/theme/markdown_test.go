package theme

import (
	"image/color"
	"testing"
)

func TestMarkdownColorsExposeSemanticPaletteWithoutToolkitTypes(t *testing.T) {
	dark := MarkdownColors(true)
	if dark.Body != (color.RGBA{R: 0xd6, G: 0xd6, B: 0xd2, A: 255}) {
		t.Fatalf("dark body = %#v", dark.Body)
	}
	if dark.Rule != (color.RGBA{R: 255, G: 255, B: 255, A: 31}) {
		t.Fatalf("dark rule = %#v", dark.Rule)
	}
	light := MarkdownColors(false)
	if light.TableControlBackground != (color.RGBA{R: 0xed, G: 0xed, B: 0xed, A: 255}) {
		t.Fatalf("light table control background = %#v", light.TableControlBackground)
	}
	if light.Rule != (color.RGBA{A: 23}) {
		t.Fatalf("light rule = %#v", light.Rule)
	}
}
