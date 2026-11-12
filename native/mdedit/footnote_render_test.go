package mdedit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestFootnoteBearStyleRender(t *testing.T) {
	const src = "分隔线上面和下面各一段。\n\n引用脚注[^note]。\n\n纯文本参照：引用脚注。\n\n[^note]: 这是脚注的内容，折行看看。"
	h := &harness{t: t, e: New(src, Options{FontSize: 24})}
	theme := ui.LightTheme()
	theme.Accent = ui.RGBA(142, 74, 210, 255)
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Themed(c, theme).Fill().Children(func() { h.e.View(c).Fill() })
	}, 600, 280)
	h.frame()
	h.frame()
	h.e.Select(len(src), len(src))
	h.frame()
	var markerWidth float32
	for _, l := range h.e.leaves {
		for _, container := range l.conts {
			if container.kind == cFootnote {
				markerWidth = h.e.footnoteIndent(container)
			}
		}
	}
	if markerWidth < 2.2*h.e.lk.size {
		t.Fatalf("footnote label column is too narrow for its label and gap: %v", markerWidth)
	}
	var referenceLine ui.TextLine
	for _, l := range h.e.leaves {
		if l.view != nil && strings.Contains(l.view.disp.text, "引用脚注") && l.start < strings.Index(src, "纯文本参照") {
			referenceLine = l.view.layout.Lines()[0]
		}
	}
	plain := newHarness(t, "引用脚注。")
	plain.e.SetOptions(Options{FontSize: 24})
	plain.frame()
	var plainLine ui.TextLine
	for _, l := range plain.e.leaves {
		if l.view != nil && strings.Contains(l.view.disp.text, "引用脚注") {
			plainLine = l.view.layout.Lines()[0]
		}
	}
	if referenceLine.Rect.H == 0 || referenceLine.Rect.X != plainLine.Rect.X ||
		referenceLine.Rect.Y != plainLine.Rect.Y || referenceLine.Rect.H != plainLine.Rect.H ||
		referenceLine.Baseline != plainLine.Baseline ||
		referenceLine.Ascent != plainLine.Ascent || referenceLine.Descent != plainLine.Descent {
		t.Fatalf("inline footnote changed the paragraph line geometry: with=%+v plain=%+v", referenceLine, plainLine)
	}
	h.shot("footnote-bear-style")
}
