package mdedit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestModalBlockCaret(t *testing.T) {
	for _, mode := range []string{"vim", "helix"} {
		h := newHarness(t, "a中\n")
		h.e.SetKeymap(mode)
		if !h.e.blockCaret() {
			t.Fatal("no block in normal")
		}
		h.caretAt(0)
		r, _ := h.e.caretRect(0)
		latin := h.e.blockCaretRect(r)
		h.caretAt(1)
		r, _ = h.e.caretRect(1)
		cjk := h.e.blockCaretRect(r)
		if latin.W <= 1.75 || cjk.W <= latin.W {
			t.Fatalf("wrong glyph widths: %v %v", latin, cjk)
		}
		h.shot(mode + "-block-caret")
		h.e.modalText("v")
		h.e.modalText("l")
		h.shot(mode + "-selection-block-caret")
		h.e.modalKey(ui.KeyEscape, 0)
		h.e.modalText("i")
		if h.e.blockCaret() {
			t.Fatal("insert must have bar caret")
		}
	}
}

func TestBlockCaretAtEndAndEmptyLine(t *testing.T) {
	h := newHarness(t, "abc\n\n")
	h.e.SetKeymap("vim")
	for _, at := range []int{3, 4, 5} {
		h.caretAt(at)
		r, ok := h.e.caretRect(at)
		if !ok {
			t.Fatalf("no caret at %d", at)
		}
		r = h.e.blockCaretRect(r)
		if r.W <= 1.75 || r.H <= 0 {
			t.Fatalf("invalid block at %d: %+v", at, r)
		}
	}
}

func TestBlockCaretTextColorContrastsWithBackground(t *testing.T) {
	for _, tt := range []struct {
		bg   ui.Color
		want ui.Color
	}{
		{ui.RGB(24, 32, 48), ui.RGB(255, 255, 255)},
		{ui.RGB(240, 210, 40), ui.RGB(0, 0, 0)},
	} {
		got := blockCaretTextColor(tt.bg)
		if got != tt.want {
			t.Errorf("blockCaretTextColor(%v) = %v, want %v", tt.bg, got, tt.want)
		}
		if got.A != 255 {
			t.Errorf("text color is not opaque: %v", got)
		}
	}
}

func TestBlockCaretGlyphOverridesStyledSpanColor(t *testing.T) {
	style := ui.Span{Font: "monospace", Size: 13, Weight: 700, Color: ui.RGB(220, 30, 70), Background: ui.RGB(230, 210, 20), Underline: true}
	d := display{pieces: []piece{{d0: 0, d1: 2, text: "中A", span: style}}}
	fg := ui.RGB(255, 255, 255)
	got, ok := blockCaretGlyph(d, 0, fg)
	if !ok {
		t.Fatal("no glyph at display offset 0")
	}
	if got.Text != "中" || got.Color != fg || got.Background.A != 0 || got.Font != style.Font || got.Size != style.Size || got.Weight != style.Weight || !got.Underline {
		t.Fatalf("block glyph span = %+v", got)
	}
	if _, ok := blockCaretGlyph(d, 1, fg); !ok {
		t.Fatal("UTF-16 display offset did not advance past CJK glyph")
	}
}

func TestBlockCaretScreenshotsWithChineseHighlight(t *testing.T) {
	src := "正文 ==高亮中文== 尾部"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	h.caretAt(strings.Index(src, "高"))
	h.shot("block-caret-normal-chinese-highlight")
	h.e.modalText("v")
	h.e.modalText("l")
	h.shot("block-caret-selection-chinese-highlight")
}
