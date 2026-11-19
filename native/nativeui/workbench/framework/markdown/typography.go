package markdown

import (
	"image/color"

	"github.com/egoist/mygo/ui"

	"github.com/wheregrassdreams/muse-editor/markdown/render"
	"muse/theme"
)

// HeadingSize is the native heading scale, shared with the block editor.
func HeadingSize(level int, body float32) float32 {
	return render.StyleHeading(level, body).Size
}

// HeadingWeight lets the page's H1 carry the title without a separate title UI.
func HeadingWeight(level int) int {
	return render.StyleHeading(level, 16).Weight
}

func HeadingLineHeight(level int) float32 {
	return render.StyleHeading(level, 16).LineHeight
}

func HeadingTextColor(dark bool) ui.Color {
	return uiColor(theme.MarkdownColors(dark).Heading)
}

// BodyTextColor keeps document prose independent of the surrounding UI theme.
func BodyTextColor(dark bool) ui.Color {
	return uiColor(theme.MarkdownColors(dark).Body)
}

// HeadingTopMargin is decorative space; source blank lines keep their own
// full line height in the editor and count toward this minimum section gap.
func HeadingTopMargin(level int, body float32) float32 {
	return render.StyleHeading(level, body).TopMargin
}

// InlineCodeColors pair neutral ink with a quiet, neutral background.
func InlineCodeColors(dark bool) (background, ink ui.Color) {
	return InlineCodeColorsFor(ui.Color{}, dark)
}

// InlineCodeColorsFor keeps code visually separate from accent-colored links.
// accent is retained for callers that resolve all inline colors together.
func InlineCodeColorsFor(_ ui.Color, dark bool) (background, ink ui.Color) {
	colors := theme.MarkdownColors(dark)
	return uiColor(colors.InlineCodeBackground), uiColor(colors.InlineCodeText)
}

// MarkdownMarkerColor mutes revealed Markdown syntax without making it look
// disabled. Marker glyphs keep the body font and use a semibold weight.
func MarkdownMarkerColor(dark bool) ui.Color {
	return uiColor(theme.MarkdownColors(dark).Marker)
}

// InlineLinkColor preserves the default red for callers without a context accent.
func InlineLinkColor(dark bool) ui.Color { return InlineLinkColorFor(ui.Color{}, dark) }

func InlineLinkColorFor(accent ui.Color, dark bool) ui.Color {
	return theme.From(accent, dark).Link
}

// CodePanelColor is the neutral background of fenced code.
func CodePanelColor(dark bool) ui.Color {
	return uiColor(theme.MarkdownColors(dark).CodePanel)
}

// MarkdownRuleColor separates quotes and table cells without framing the page.
func MarkdownRuleColor(dark bool) ui.Color {
	return uiColor(theme.MarkdownColors(dark).Rule)
}

// TableRuleColor gives an idle table a quieter grid than document separators.
func TableRuleColor(dark bool) ui.Color {
	return uiColor(theme.MarkdownColors(dark).TableRule)
}

func TableControlColors(dark bool) (background, ink ui.Color) {
	colors := theme.MarkdownColors(dark)
	return uiColor(colors.TableControlBackground), uiColor(colors.TableControlInk)
}

func uiColor(c color.RGBA) ui.Color { return ui.RGBA(c.R, c.G, c.B, float32(c.A)/255) }
