package markdown

import (
	"github.com/egoist/mygo/ui"

	appearance "muse/theme"
)

// HeadingSize is the native heading scale, shared with the block editor.
func HeadingSize(level int, body float32) float32 {
	scale := [...]float32{2.5, 1.7, 1.35, 1.15, 1.05, 1}
	return body * scale[min(max(level, 1), 6)-1]
}

// HeadingWeight lets the page's H1 carry the title without a separate title UI.
func HeadingWeight(level int) int {
	if level <= 2 {
		return 700
	}
	return 600
}

func HeadingLineHeight(level int) float32 {
	if level <= 2 {
		return 1.25
	}
	return 1.35
}

func HeadingTextColor(dark bool) ui.Color {
	if dark {
		return ui.Hex("#ececea")
	}
	return ui.Hex("#292928")
}

// BodyTextColor keeps document prose independent of the surrounding UI theme.
func BodyTextColor(dark bool) ui.Color {
	if dark {
		return ui.Hex("#d6d6d2")
	}
	return ui.Hex("#444442")
}

// HeadingTopMargin is decorative space; source blank lines keep their own
// full line height in the editor and count toward this minimum section gap.
func HeadingTopMargin(level int, body float32) float32 {
	space := [...]float32{3.2, 3.2, 2.5, 2.2, 2, 2}
	return body * space[min(max(level, 1), 6)-1]
}

// InlineCodeColors pair neutral ink with a quiet, neutral background.
func InlineCodeColors(dark bool) (background, ink ui.Color) {
	return InlineCodeColorsFor(ui.Color{}, dark)
}

// InlineCodeColorsFor keeps code visually separate from accent-colored links.
// accent is retained for callers that resolve all inline colors together.
func InlineCodeColorsFor(_ ui.Color, dark bool) (background, ink ui.Color) {
	ink = BodyTextColor(dark)
	if dark {
		return ui.Hex("#2a2a2a"), ink
	}
	return ui.Hex("#f0f1f2"), ink
}

// MarkdownMarkerColor mutes revealed Markdown syntax without making it look
// disabled. Marker glyphs keep the body font and use a semibold weight.
func MarkdownMarkerColor(dark bool) ui.Color {
	if dark {
		return ui.Hex("#6f6f6c")
	}
	return ui.Hex("#b5b5b3")
}

// InlineLinkColor preserves the default red for callers without a context accent.
func InlineLinkColor(dark bool) ui.Color { return InlineLinkColorFor(ui.Color{}, dark) }

func InlineLinkColorFor(accent ui.Color, dark bool) ui.Color {
	return appearance.From(accent, dark).Link
}

// CodePanelColor is the neutral background of fenced code.
func CodePanelColor(dark bool) ui.Color {
	if dark {
		return ui.RGBA(255, 255, 255, 0.045)
	}
	return ui.Hex("#f5f5f4")
}

// MarkdownRuleColor separates quotes and table cells without framing the page.
func MarkdownRuleColor(dark bool) ui.Color {
	if dark {
		return ui.RGBA(255, 255, 255, 0.12)
	}
	return ui.RGBA(0, 0, 0, 0.09)
}

// TableRuleColor gives an idle table a quieter grid than document separators.
func TableRuleColor(dark bool) ui.Color {
	if dark {
		return ui.Hex("#363638")
	}
	return ui.Hex("#ededed")
}

func TableControlColors(dark bool) (background, ink ui.Color) {
	if dark {
		return ui.Hex("#333335"), ui.Hex("#b0b0ad")
	}
	return ui.Hex("#ededed"), ui.Hex("#8b8b88")
}
