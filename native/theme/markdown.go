package theme

import "image/color"

// MarkdownPalette contains semantic colors for Markdown content, independent
// of any widget or drawing backend.
type MarkdownPalette struct {
	Heading, Body, InlineCodeBackground, Marker color.RGBA
	InlineCodeText, CodePanel, Rule, TableRule  color.RGBA
	TableControlBackground, TableControlInk     color.RGBA
}

// MarkdownColors returns the content palette shared by the editor and
// standalone Markdown views.
func MarkdownColors(dark bool) MarkdownPalette {
	if dark {
		return MarkdownPalette{
			Heading:                hexRGBA(0xececea),
			Body:                   hexRGBA(0xd6d6d2),
			InlineCodeBackground:   hexRGBA(0x2a2a2a),
			InlineCodeText:         hexRGBA(0xd6d6d2),
			Marker:                 hexRGBA(0x6f6f6c),
			CodePanel:              color.RGBA{R: 255, G: 255, B: 255, A: 11},
			Rule:                   color.RGBA{R: 255, G: 255, B: 255, A: 31},
			TableRule:              hexRGBA(0x363638),
			TableControlBackground: hexRGBA(0x333335),
			TableControlInk:        hexRGBA(0xb0b0ad),
		}
	}
	return MarkdownPalette{
		Heading:                hexRGBA(0x292928),
		Body:                   hexRGBA(0x444442),
		InlineCodeBackground:   hexRGBA(0xf0f1f2),
		InlineCodeText:         hexRGBA(0x444442),
		Marker:                 hexRGBA(0xb5b5b3),
		CodePanel:              hexRGBA(0xf5f5f4),
		Rule:                   color.RGBA{R: 0, G: 0, B: 0, A: 23},
		TableRule:              hexRGBA(0xededed),
		TableControlBackground: hexRGBA(0xededed),
		TableControlInk:        hexRGBA(0x8b8b88),
	}
}

func hexRGBA(rgb uint32) color.RGBA {
	return color.RGBA{R: uint8(rgb >> 16), G: uint8(rgb >> 8), B: uint8(rgb), A: 255}
}
