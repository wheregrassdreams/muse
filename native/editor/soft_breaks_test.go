package mdedit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func softBreakHarness(t *testing.T, src string) *harness {
	h := newHarness(t, src)
	opts := h.e.Options()
	opts.SoftBreaksAsSpaces = true
	h.e.SetOptions(opts)
	h.frame()
	return h
}

func TestMarkdownSoftAndHardBreakProjection(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"soft", "第一行\n下一行", "第一行 下一行"},
		{"spaces hard break", "第一行  \n下一行", "第一行  \n下一行"},
		{"slash hard break", "第一行\\\n下一行", "第一行\n下一行"},
		{"html hard break", "第一行<br>下一行", "第一行\n下一行"},
		{"quote continuation", "> 第一行\n> 下一行", "第一行\n下一行"},
		{"list continuation", "- 第一行\n  下一行", "第一行 下一行"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := softBreakHarness(t, tc.src)
			if got := h.e.leaves[0].view.disp.text; got != tc.want {
				t.Fatalf("display=%q want %q", got, tc.want)
			}
			if tc.name == "quote continuation" {
				v := h.e.leaves[0].view
				if got := len(v.layout.Lines()); got != 2 {
					t.Fatalf("quote layout lines=%d want 2", got)
				}
				if _, h := v.layout.Size(); h < v.font.Size*2 {
					t.Fatalf("quote layout height=%v does not contain two source lines", h)
				}
			}
			h.want(tc.src)
			h.e.Select(0, len(tc.src))
			if h.e.SelectedText() != tc.src {
				t.Fatal("copy changed source newlines")
			}
		})
	}
}

func TestSoftBreakSourceMappingEditingAndOptOut(t *testing.T) {
	const src = "first line\nsecond line"
	h := softBreakHarness(t, src)
	for _, off := range []int{len("first line"), len("first line\n"), len(src)} {
		h.caretAt(off)
		r, ok := h.e.caretRect(off)
		if !ok {
			t.Fatal("caret missing at soft break")
		}
		if got := h.e.hitTest(r.X, r.Y+r.H/2); got != off {
			t.Fatalf("source/visual roundtrip %d != %d", got, off)
		}
	}
	h.caretAt(len("first line\n"))
	h.typ("X")
	h.want("first line\nXsecond line")
	h.e.Undo()
	h.frame()
	h.want(src)
	h.caretAt(len("first line\n"))
	h.key(0, ui.KeyBackspace)
	h.want("first linesecond line")
	h.e.Undo()
	h.frame()
	h.want(src)
	opts := h.e.Options()
	opts.SoftBreaksAsSpaces = false
	h.e.SetOptions(opts)
	h.frame()
	if !strings.Contains(h.e.leaves[0].view.disp.text, "\n") {
		t.Fatal("source line mode did not restore newline")
	}
}

func TestSoftBreakParagraphAndCodeBoundariesRemain(t *testing.T) {
	const src = "first\ncontinued\n\nsecond\nparagraph\n\n```go\na := 1\nb := 2\n```"
	h := softBreakHarness(t, src)
	paras, codes := 0, 0
	for _, l := range h.e.leaves {
		if l.kind == kPara {
			paras++
			if strings.Contains(l.view.disp.text, "\n") {
				t.Fatal("soft newline remained in prose")
			}
		}
		if l.kind == kCode {
			codes++
			if !strings.Contains(l.view.disp.text, "\n") {
				t.Fatal("code newlines collapsed")
			}
		}
	}
	if paras != 2 || codes != 1 {
		t.Fatalf("paragraph/code boundaries=%d/%d", paras, codes)
	}
	h.want(src)
}

func TestSoftWrappedInlineCodeUsesWholeParagraph(t *testing.T) {
	const src = "这是一段普通正文，用来检查行内语法的展开与收起：**粗体**、*斜体*、~~删除线~~、`行内代码`、\n[链接](https://example.com)、[[双链]]、[[双链|带标签]]、#标签 #回归/编辑器，以及行内公式 $a^2+b^2=c^2$。"
	h := softBreakHarness(t, src)
	opts := h.e.Options()
	opts.MaxWidth = 520
	h.e.SetOptions(opts)
	h.frame()
	if strings.Contains(h.e.leaves[0].view.disp.text, "\n") {
		t.Fatal("source soft break still forced a visual line")
	}
	h.shot("markdown-soft-wrap-inline-code")
	h.want(src)
}

func TestSoftWrappedCalloutKeepsTitleRow(t *testing.T) {
	const src = "> [!NOTE] 提示\n> body\n> continuation\n\nafter"
	h := softBreakHarness(t, src)
	h.caretAt(len(src))
	l := h.e.leaves[0]
	if got := l.view.disp.text; !strings.HasPrefix(got, " \n") || strings.Count(got, "\n") != 2 || !strings.Contains(got, "body\ncontinuation") {
		t.Fatalf("callout title/body lost structural break: %q", got)
	}
	if got := len(l.view.layout.Lines()); got != 3 {
		t.Fatalf("callout layout lines=%d want title plus two body lines", got)
	}
	h.shot("markdown-soft-wrap-callout")
	h.want(src)
}
