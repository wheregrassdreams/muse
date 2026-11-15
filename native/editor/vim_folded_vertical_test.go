package mdedit

import (
	"fmt"
	"strings"
	"testing"
)

func TestVimDownFromFirstTwoCharactersOfFoldedInlineParagraph(t *testing.T) {
	const src = "这是一段普通正文，用来检查行内语法的展开与收起：**粗体**、*斜体*、~~删除线~~、`行内代码`、\n[链接](https://example.com)、[[双链]]、[[双链|带标签]]、#标签 #回归/编辑器，以及行内公式 $a^2+b^2=c^2$。"
	for _, width := range []float32{420, 440, 460, 480, 500, 520, 540, 560, 580, 600, 620, 640, 680, 720, 760, 800} {
		for _, off := range []int{0, len("这")} {
			t.Run(fmt.Sprintf("width=%.0f/byte=%d", width, off), func(t *testing.T) {
				h := newHarness(t, src)
				opts := h.e.Options()
				opts.MaxWidth = width
				opts.SoftBreaksAsSpaces = true
				h.e.SetOptions(opts)
				h.e.SetKeymap("vim")
				h.e.Select(off, off)
				h.frame()
				u := h.e.unitAt(off)
				if strings.Contains(u.view.disp.text, "**粗体**") || len(u.view.layout.Lines()) < 2 {
					t.Fatal("fixture must have folded inline syntax and multiple display lines")
				}
				before, ok := h.e.caretRect(off)
				if !ok {
					t.Fatal("initial character has no native caret geometry")
				}
				h.e.ReplaceText(nil, "j")
				h.frame()
				after, ok := h.e.caretRect(h.e.caret)
				if !ok || after.Y <= before.Y+before.H/2 {
					t.Fatalf("j remained on its original display row after source reveal: byte=%d before=%+v after=%+v", h.e.caret, before, after)
				}
				if !h.e.hasGoal || h.e.goalX != before.X {
					t.Fatalf("source reveal lost the geometric column: goal=%v want=%v", h.e.goalX, before.X)
				}
				h.e.ReplaceText(nil, "k")
				h.frame()
				back, ok := h.e.caretRect(h.e.caret)
				if !ok || back.Y >= after.Y-after.H/2 {
					t.Fatalf("k failed to return to the preceding display row: before=%+v after=%+v", after, back)
				}
				if h.e.Source() != src {
					t.Fatal("vertical navigation changed source")
				}
			})
		}
	}
}

func TestVimVerticalNavigationAcrossStructuralRows(t *testing.T) {
	t.Run("callout blank quote line belongs to its own row", func(t *testing.T) {
		const src = "> [!TIP] 提示\n> callout 的第一段。\n>\n> callout 的第二段。\n\n后续正文。"
		h := newHarness(t, src)
		h.e.SetKeymap("vim")
		start := strings.Index(src, "callout 的第一段") + len("callo")
		h.e.Select(start, start)
		var seen []int
		for range 4 {
			before := h.e.caret
			h.e.ReplaceText(nil, "j")
			if h.e.caret == before {
				t.Fatalf("j stalled at source byte %d", before)
			}
			seen = append(seen, h.e.caret)
		}
		if seen[0] != strings.Index(src, "\n>\n")+2 {
			t.Fatalf("first j should land on the blank quote row, got %d (%q)", seen[0], src[seen[0]:])
		}
		second := strings.Index(src, "callout 的第二段")
		if seen[1] < second || seen[1] >= second+len("callout 的第二段") {
			t.Fatalf("second j did not reach the next callout paragraph: %d", seen[1])
		}
		later := strings.Index(src, "后续正文")
		if seen[3] < later || seen[3] > later+len("后续正文") {
			t.Fatalf("vertical navigation did not leave the callout: caret=%d source=%q", seen[3], src[seen[3]:])
		}
		h.shot("vim-nav-callout-blank-line")
		if h.e.Source() != src {
			t.Fatal("vertical navigation changed source")
		}
	})

	t.Run("fenced code closing fence is caretable source", func(t *testing.T) {
		const src = "```go\nfunc main() {\n    println(\"hello, muse\")\n}\n```\n\n后续正文。"
		h := newHarness(t, src)
		h.e.SetKeymap("vim")
		code := h.e.leaves[0]
		at := strings.Index(src, "}")
		h.e.setCaret(at, false)
		h.e.sourceOK, h.e.sourceAt = true, code.lineStart // reveal source and decoration rows
		h.e.invalidate()
		h.frame()
		v := h.e.unitView(code, at)
		closeStart := strings.LastIndex(src, "```")
		if _, ok := v.disp.srcToDisp(closeStart - code.lineStart); !ok {
			t.Fatal("closing fence has no source-backed display stop")
		}
		before := h.e.caret
		h.e.ReplaceText(nil, "j")
		if h.e.caret == before || h.e.caret < closeStart || h.e.caret >= blockEnd(src, code) {
			t.Fatalf("j failed to reach the closing fence row: from=%d to=%d closing=%d", before, h.e.caret, closeStart)
		}
		closeRect, ok := h.e.caretRect(h.e.caret)
		if !ok {
			t.Fatal("closing fence caret has no geometry")
		}
		h.e.ReplaceText(nil, "k")
		bodyRect, ok := h.e.caretRect(h.e.caret)
		if !ok || bodyRect.Y >= closeRect.Y {
			t.Fatalf("k did not move from the closing fence to the code body: close=%+v body=%+v", closeRect, bodyRect)
		}
		h.e.ReplaceText(nil, "j")
		backRect, ok := h.e.caretRect(h.e.caret)
		if !ok || backRect.Y != closeRect.Y {
			t.Fatalf("j did not return to the closing fence row: close=%+v back=%+v", closeRect, backRect)
		}
		h.shot("vim-nav-fence-caret")
		h.e.modalText("v")
		h.e.modalText("l")
		if !strings.Contains(h.e.SelectedText(), "`") {
			t.Fatalf("Visual selection does not include the closing fence: %q", h.e.SelectedText())
		}
		h.shot("vim-nav-fence-caret-and-selection")
		if h.e.Source() != src {
			t.Fatal("vertical navigation changed source")
		}
	})
}

func TestVimRightNavigationCrossesCollapsedInlineSyntax(t *testing.T) {
	const src = "前文 **粗体**、`行内代码`、[链接](https://example.com) 以及 $a^2+b^2=c^2$。后文"
	h := newHarness(t, src)
	h.e.SetKeymap("vim")
	h.e.Select(strings.Index(src, "前文"), strings.Index(src, "前文"))
	last := h.e.caret
	for n := 0; n < 100 && h.e.caret < len(src)-len("后文"); n++ {
		h.e.ReplaceText(nil, "l")
		if h.e.caret <= last {
			t.Fatalf("l stalled or moved backward at byte %d (step %d)", last, n)
		}
		last = h.e.caret
	}
	if !strings.HasPrefix(src[h.e.caret:], "文") && h.e.caret < strings.Index(src, "后文") {
		t.Fatalf("l did not cross the projected inline constructs: byte %d", h.e.caret)
	}
	if h.e.Source() != src {
		t.Fatal("horizontal navigation changed source")
	}
}

func TestVimRightNavigationSkipsZeroSourceLinkDecoration(t *testing.T) {
	const src = "`行内代码`、[链接](https://example.com)后文"
	for _, width := range []float32{240, 320, 420, 820} {
		for _, start := range []int{0, strings.Index(src, "`行内代码`"), strings.Index(src, "、"), strings.Index(src, "链接")} {
			h := newHarness(t, src)
			opts := h.e.Options()
			opts.MaxWidth = width
			h.e.SetOptions(opts)
			h.e.SetKeymap("vim")
			h.caretAt(start)
			last := h.e.caret
			for n := 0; n < 100 && h.e.caret < len(src)-len("后文"); n++ {
				h.e.ReplaceText(nil, "l")
				if h.e.caret <= last {
					t.Fatalf("l stalled or moved backward at width %.0f from %d at byte %d (step %d); suffix=%q", width, start, last, n, src[last:])
				}
				last = h.e.caret
			}
			if h.e.caret < strings.Index(src, "后文") {
				t.Fatalf("l did not cross the external-link decoration at width %.0f from %d: byte %d, suffix=%q", width, start, h.e.caret, src[h.e.caret:])
			}
			if h.e.Source() != src {
				t.Fatal("horizontal navigation changed source")
			}
		}
	}
}

func TestVimRightNavigationThroughFullSyntaxFixture(t *testing.T) {
	const src = "这是一段普通正文，用来检查行内语法的展开与收起：**粗体**、*斜体*、~~删除线~~、`行内代码`、\n[链接](https://example.com)、[[双链]]、[[双链|带标签]]、#标签 #回归/编辑器，以及行内公式 $a^2+b^2=c^2$。"
	code := strings.Index(src, "`行内代码`")
	for _, width := range []float32{420, 520, 680, 820} {
		for _, start := range []int{code + len("`行内代码") - 1, code + len("`行内代码`"), strings.Index(src, "[链接")} {
			h := newHarness(t, src)
			opts := h.e.Options()
			opts.MaxWidth, opts.SoftBreaksAsSpaces = width, true
			h.e.SetOptions(opts)
			h.e.SetKeymap("vim")
			h.caretAt(start)
			if width == 420 && start == code+len("`行内代码")-1 {
				for range 3 {
					h.e.ReplaceText(nil, "l")
				}
				if h.e.caret < strings.Index(src, "\n[链接]") {
					t.Fatalf("l did not cross the folded soft break from the inline-code ending: caret=%d", h.e.caret)
				}
				h.shot("vim-right-folded-softbreak")
				h.caretAt(start)
			}
			last := h.e.caret
			for n := 0; n < 150 && h.e.caret < len(src)-len("。"); n++ {
				h.e.ReplaceText(nil, "l")
				if h.e.caret <= last {
					t.Fatalf("l stalled in full syntax fixture at width %.0f from %d, byte %d (step %d): %q", width, start, last, n, src[last:])
				}
				last = h.e.caret
			}
			if h.e.caret < len(src)-len("。") {
				t.Fatalf("l did not progress through the full syntax fixture at width %.0f: byte %d, suffix=%q", width, h.e.caret, src[h.e.caret:])
			}
		}
	}
}
