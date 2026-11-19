package editor

import "testing"

func TestPunctuationSpacingScreenshotAndCaret(t *testing.T) {
	const src = "一个很长的段落，用来检查换行与滚动：在很长很长的文本里，排版应当把每一行都摆对，光标应当能落在任意两字之间，方向键上下移动应当尽量保持列的位置，而不该常常跳到行首行尾。再加一些标点，逗号、句号、顿号、括号（圆括号）、【方括号】、「引号」，以及英文单词 an occasional English sentence to check mixed script wrapping and the baseline alignment of latin letters beside CJK glyphs in the same line.\n\n> 普通引用。\n> 第二行。"
	h := paragraphEditingHarness(t, src)
	opts := h.e.Options()
	opts.MaxWidth = 560
	opts.FontSize = 20
	h.e.SetOptions(opts)
	h.frame()
	h.shot("punctuation-pairs-after")
	h.want(src)
	l := h.e.leaves[0]
	for _, piece := range l.view.disp.pieces {
		for _, stop := range []int{piece.s0, piece.s1} {
			off := l.lineStart + stop
			r, ok := h.e.caretRect(off)
			if !ok {
				t.Fatalf("missing caret at %d", off)
			}
			if got := h.e.hitTest(r.X, r.Y+r.H/2); got != off {
				t.Fatalf("caret/hit mismatch: %d != %d", got, off)
			}
		}
	}
	opts.DisableSmartTypography = true
	h.e.SetOptions(opts)
	h.frame()
	h.shot("punctuation-pairs-before")
}

func TestPunctuationPairCloseup(t *testing.T) {
	h := newHarness(t, "括号（圆括号）、【方括号】、「引号」，结束")
	o := h.e.Options()
	o.FontSize = 36
	o.MaxWidth = 900
	h.e.SetOptions(o)
	h.frame()
	h.shot("punctuation-closeup-after")
	o.DisableSmartTypography = true
	h.e.SetOptions(o)
	h.frame()
	h.shot("punctuation-closeup-before")
}
