package editor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"muse/markdown"
	appearance "muse/theme"
)

func TestSmartTypographyToggleAndEditingGeometry(t *testing.T) {
	src := strings.Repeat("Text with varied words should retain accurate native caret geometry while its paragraph is optimized. ", 6)
	h := newHarness(t, src)
	opts := h.e.Options()
	opts.MaxWidth = 720
	h.e.SetOptions(opts)
	h.frame()
	l := h.e.leaves[0]
	if reflect.DeepEqual(l.view.layout.Lines(), l.view.layout.TextLayout.Lines()) {
		t.Fatal("default editor did not optimize paragraph")
	}
	h.shot("smart-typography-on")
	// Hit testing and caret placement must agree on the optimized second line.
	line := l.view.layout.Lines()[1]
	display := line.Start + 3
	off := l.lineStart + l.view.disp.dispToSrc(display, false)
	h.caretAt(off)
	r, ok := h.e.caretRect(off)
	if !ok {
		t.Fatal("no caret in optimized line")
	}
	if got := h.e.hitTest(r.X, r.Y+r.H/2); got != off {
		t.Fatalf("caret/hit disagree: %d != %d", got, off)
	}
	h.typ("Z")
	h.want(src[:off] + "Z" + src[off:])
	if !h.e.Undo() {
		t.Fatal("typing undo failed")
	}
	h.frame()
	h.want(src)
	opts.DisableSmartTypography = true
	h.e.SetOptions(opts)
	h.frame()
	l = h.e.leaves[0]
	if !reflect.DeepEqual(l.view.layout.Lines(), l.view.layout.TextLayout.Lines()) {
		t.Fatal("setting off retained optimized geometry")
	}
	h.shot("smart-typography-off")
	h.want(src)
	// Re-enabling invalidates all layout levels and preserves the source/caret.
	caret := h.e.caret
	opts.DisableSmartTypography = false
	h.e.SetOptions(opts)
	h.frame()
	if h.e.caret != caret || reflect.DeepEqual(h.e.leaves[0].view.layout.Lines(), h.e.leaves[0].view.layout.TextLayout.Lines()) {
		t.Fatal("re-enabling failed or moved the caret")
	}
	h.key(0, ui.KeyDown)
	if h.e.caret <= caret {
		t.Fatal("Down did not navigate to next optimized line")
	}
}

func TestMarkdownLinkCodeAndMarkColorsFollowAccent(t *testing.T) {
	if headingWeight(1) <= headingWeight(6) || headingWeight(6) < 600 {
		t.Fatalf("page title should be heavier than small headings: %d/%d", headingWeight(1), headingWeight(6))
	}
	for _, dark := range []bool{false, true} {
		var links, inks, codeBGs, marks = map[ui.Color]bool{}, map[ui.Color]bool{}, map[ui.Color]bool{}, map[ui.Color]bool{}
		for _, accent := range []ui.Color{ui.Hex("#c99a18"), ui.Hex("#3979d1")} {
			palette := appearance.From(accent, dark)
			lk := look{
				dark: dark, accent: accent, text: markdown.BodyTextColor(dark),
				muted: ui.Hex("#777777"), codeBg: ui.RGBA(0, 0, 0, 14), codeText: palette.Ink,
				markBg: palette.Soft.Alpha(2.1), mono: "monospace",
			}
			if regular := lk.span(istyle{}, 16); regular.Weight != 0 || regular.Color != markdown.BodyTextColor(dark) {
				t.Fatalf("dark=%v accent=%v regular body span=%+v", dark, accent, regular)
			}
			if bold := lk.span(istyle{bold: true}, 16); bold.Weight != 600 {
				t.Fatalf("bold weight=%d, want 600", bold.Weight)
			}
			if strike := lk.span(istyle{strike: true}, 16); strike.Color == markdown.BodyTextColor(dark) {
				t.Fatalf("dark=%v strike span is not subdued: %+v", dark, strike)
			}
			link := lk.span(istyle{link: true}, 16)
			code := lk.span(istyle{code: true}, 16)
			if link.Color != markdown.InlineLinkColorFor(accent, dark) || !link.Underline {
				t.Fatalf("dark=%v accent=%v link span=%+v", dark, accent, link)
			}
			wantBG, wantInk := markdown.InlineCodeColorsFor(accent, dark)
			if code.Color != wantInk || code.Background != wantBG || code.Font != lk.mono {
				t.Fatalf("dark=%v accent=%v code style=%+v, want neutral ink=%v bg=%v mono=%q", dark, accent, code, wantInk, wantBG, lk.mono)
			}
			links[link.Color], inks[code.Color], codeBGs[code.Background], marks[lk.markBg] = true, true, true, true
		}
		if len(links) != 2 || len(inks) != 1 || len(codeBGs) != 1 || len(marks) != 2 {
			t.Fatalf("dark=%v: accent colors or neutral code colors unexpected: links=%v code=%v code bg=%v marks=%v", dark, links, inks, codeBGs, marks)
		}
	}
}

func TestSmartTypographyKeepsCodeWholeBeforeSourceNewline(t *testing.T) {
	const src = "这是一段普通正文，用来检查行内语法的展开与收起：**粗体**、*斜体*、~~删除线~~、`行内代码`、\n[链接](https://example.com)、[[双链]]、[[双链|带标签]]、#标签 #回归/编辑器，以及行内公式 $a^2+b^2=c^2$。"
	for width := float32(520); width <= 800; width += 20 {
		h := newHarness(t, src)
		opts := h.e.Options()
		opts.MaxWidth = width
		h.e.SetOptions(opts)
		h.frame()
		l := h.e.leaves[0]
		if strings.Count(l.view.disp.text, "\n") != 1 {
			t.Fatal("source newline changed")
		}
		found := false
		for _, piece := range l.view.disp.pieces {
			if piece.text != "行内代码" && piece.text != "删除线" {
				continue
			}
			if piece.text == "行内代码" {
				found = true
			}
			for _, line := range l.view.layout.Lines() {
				if line.End > piece.d0 && line.End < piece.d1 {
					t.Fatalf("width %.0f split short code at %d inside %d..%d", width, line.End, piece.d0, piece.d1)
				}
			}
		}
		if !found {
			t.Fatal("code span missing")
		}
		if width == 600 {
			h.shot("smart-typography-code-source-newline")
		}
		h.want(src)
	}
}
