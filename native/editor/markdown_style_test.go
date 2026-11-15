package editor

import (
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"muse/markdown"
)

// One compact fixture exercises the hierarchy, prose, chrome and editable
// source together. MDEDIT_SHOTS exports both native renderers in both themes.
const markdownStyleFixture = "# A quiet document\n" +
	"Body **emphasis**, `inline code`, [a link](https://example.com), and #plain-tag.\n" +
	"## A smaller section\n" +
	"### Third level\n#### Fourth level\n##### Fifth level\n###### Sixth level\n\n" +
	"> Quoted words remain readable, with **emphasis** and `code`.\n\n" +
	"- First item\n- Second item\n  - A nested item\n\n" +
	"```go\nfunc quiet() string {\n    return \"Muse\"\n}\n```\n\n" +
	"| Element | Intent |\n| --- | --- |\n| Code | Identifiable |\n| Quote | Readable |\n\n" +
	"Final paragraph.\n"

func TestMarkdownStyleFixture(t *testing.T) {
	for _, dark := range []bool{false, true} {
		name, theme := "light", ui.LightTheme()
		if dark {
			name, theme = "dark", ui.DarkTheme()
		}
		t.Run(name, func(t *testing.T) {
			h := &harness{t: t, e: New(markdownStyleFixture, Options{FontSize: 16})}
			h.tt = ui.NewTester(func(c *ui.Context) {
				ui.Themed(c, theme).Fill().Background(theme.Background).Children(func() { h.e.View(c).Grow(1) })
			}, 840, 1200)
			h.frame()
			h.caretAt(len(markdownStyleFixture))
			h.shot("markdown-editor-" + name)
			lastBottom := float32(0)
			for _, l := range h.e.leaves {
				if l.y < lastBottom-0.01 {
					t.Fatal("styled blocks overlap")
				}
				lastBottom = l.y + l.h
				if l.kind == kBlank && l.h < h.e.lk.size*h.e.lk.lh {
					t.Fatal("styling collapsed a source blank line")
				}
			}
			// Contrast is checked against readability, not exact palette values.
			quote := h.e.lk.span(istyle{muted: true}, 16).Color
			if contrastOn(quote, ui.Transparent, theme.Background) < 4.5 {
				t.Fatal("quote body has insufficient contrast")
			}
			if contrastOn(h.e.lk.codeText, h.e.lk.codeBg, theme.Background) < 4.5 {
				t.Fatal("inline code has insufficient contrast")
			}
			h.focus()
			h.caretAt(strings.Index(markdownStyleFixture, "A quiet"))
			if !strings.HasPrefix(h.e.leaves[0].view.disp.text, "# ") {
				t.Fatal("heading source marker is no longer editable")
			}
			h.want(markdownStyleFixture)
			h.shot("markdown-editor-source-" + name)

			doc := markdown.Parse(markdownStyleFixture)
			h.tt = ui.NewTester(func(c *ui.Context) {
				ui.Themed(c, theme).Fill().Background(theme.Background).Children(func() {
					markdown.View(c, doc, markdown.Options{BodyFontSize: 16})
				})
			}, 840, 1200)
			h.frame()
			for _, text := range []string{"A quiet document", "Quoted words", "inline code", "#plain-tag", "Identifiable", "Final paragraph"} {
				if !h.tt.HasText(text) {
					t.Errorf("preview lost %q", text)
				}
			}
			h.shot("markdown-preview-" + name)
		})
	}
}

// contrastOn composites translucent document styles before computing WCAG
// relative luminance; it works with either a light or a dark page.
func contrastOn(ink, panel, page ui.Color) float64 {
	composite := func(top, bottom ui.Color) ui.Color {
		return bottom.Mix(ui.RGB(top.R, top.G, top.B), float32(top.A)/255)
	}
	luminance := func(c ui.Color) float64 {
		linear := func(v uint8) float64 {
			x := float64(v) / 255
			if x <= 0.04045 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
	}
	background := composite(panel, page)
	a, b := luminance(composite(ink, background)), luminance(background)
	return (max(a, b) + 0.05) / (min(a, b) + 0.05)
}
