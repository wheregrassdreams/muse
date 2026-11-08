package mdedit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// TestRenderedTextHasNoStrayGlyph checks what a document's display may put
// in the text: an object's replacement character only on an object with a
// box, no replacement character on its own, and nothing the font has to
// invent a glyph for.
//
// The empty square — the .notdef glyph — is what a reader sees when the
// text holds a character with no glyph: the object replacement character of
// an object with no box, a broken zero width character pasted from a web
// page, a half of a surrogate pair. A display that keeps them out of the
// text cannot draw one.
func TestRenderedTextHasNoStrayGlyph(t *testing.T) {
	for name, src := range renderCorpus(t) {
		e := New(src, Options{FontSize: 16})
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Fill().Children(func() {
				e.View(c).Grow(1)
			})
		}, 1000, 700)
		tt.Frame()
		tt.Frame()
		e.Focus()
		for _, caret := range caretsIn(src) {
			e.anchor, e.caret = caret, caret
			e.wantReveal = true
			tt.Frame()
			tt.Frame()
			checkDisplay(t, name, e)
		}
	}
}

// renderCorpus returns the documents to render: every Markdown file of the
// repository, and a set of comments in every place one can be written.
func renderCorpus(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	files, _ := filepath.Glob("../**/*.md")
	files = append(files, "../testdata/all-syntax.md", "../testdata/playground.md")
	for _, f := range files {
		if b, err := os.ReadFile(f); err == nil && len(b) < 200<<10 {
			out[filepath.Base(f)] = string(b)
		}
	}
	for _, c := range []struct{ name, src string }{
		{"comment-inline", "正文前 %%这是注释%% 正文后"},
		{"comment-heading", "## 标题 %%注释%% 更多的字"},
		{"comment-list", "- 一项 %%注释%% 的字\n- 二项"},
		{"comment-quote", "> 引用 %%注释%% 的字"},
		{"comment-table", "| 甲 | 乙 |\n| --- | --- |\n| a %%注释%% b | c |"},
		{"comment-start", "%%注释%% 正文"},
		{"comment-end", "正文 %%注释%%"},
		{"comment-alone", "%%注释%%"},
		{"comment-unclosed", "正文 %%没闭合"},
		{"comment-block", "前\n%%\n块注释\n%%\n后"},
		{"comment-two", "a %%一%% b %%二%% c"},
		{"comment-code", "```\n%%不是注释%%\n```"},
		{"comment-escape", `正文 \%\% 不是注释`},
		{"comment-emoji", "正文 %%🏷️😀%% 后"},
		{"comment-wide", "正文 %%宽字符：𠀋𠀌%% 后"},
		{"comment-item", "- %%注释%% 一项\n- 二项"},
		{"comment-numbered", "1. %%注释%% 一项"},
		{"comment-task", "- [ ] %%注释%% 一件事"},
		{"comment-footnote", "正文[^1]\n\n[^1]: %%注释%% 脚注"},
		{"comment-table-opens", "| %%注释%% 甲 | 乙 |\n| --- | --- |\n| a | b |"},
		{"comment-heading-opens", "## %%注释%% 标题"},
		{"comment-callout", "> [!NOTE]\n> %%注释%% 内容"},
		{"comment-multiline-opens", "%%一\n二%% 正文"},
		{"comment-indented", "    %%注释%% 代码"},
		{"comment-two-lines", "%%a%%\n%%b%%"},
		{"comment-blank-between", "%%a%%\n\n%%b%%"},
		{"comment-quote-nested", "> > %%注释%% 引用"},
		{"comment-list-multi", "- 一\n  %%注释%% 续行\n- 二"},
	} {
		out[c.name] = c.src
	}
	return out
}

// caretsIn returns the offsets to render: the start, the end, the middle,
// and the first byte of every inline construct of the source.
func caretsIn(src string) []int {
	cs := []int{0, len(src) / 2, len(src)}
	if i := strings.Index(src, "%%"); i >= 0 {
		cs = append(cs, i, i+2, (i+len(src))/2)
	}
	return cs
}

// checkDisplay checks every leaf of the document as it shows with the caret
// where it is.
func checkDisplay(t *testing.T, name string, e *Editor) {
	t.Helper()
	e.layoutIfNeeded()
	for _, l := range e.leaves {
		v := l.view
		if v == nil {
			continue
		}
		if !utf8.ValidString(v.disp.text) {
			t.Fatalf("%s: leaf %d..%d (line %d, kind %d) shows what is not text: %q\n  source: %q",
				name, l.start, l.end, l.lineStart, l.kind, v.disp.text, e.src[l.start:min(l.end+8, len(e.src))])
		}
		for _, p := range v.disp.pieces {
			if !utf8.ValidString(p.text) {
				t.Fatalf("%s: a piece is not text: %q (source %d..%d)", name, p.text, p.s0, p.s1)
			}
			if p.obj != nil && (p.span.ObjectWidth <= 0 || p.span.ObjectHeight <= 0) {
				t.Fatalf("%s: an object with no box: %q on a box of %.1fx%.1f",
					name, p.text, p.span.ObjectWidth, p.span.ObjectHeight)
			}
			if p.obj == nil && strings.ContainsRune(p.text, 0xFFFC) {
				t.Fatalf("%s: the replacement character without an object: %q", name, p.text)
			}
			if strings.ContainsRune(p.text, 0xFFFD) {
				t.Fatalf("%s: the replacement character of a broken character: %q", name, p.text)
			}
		}
	}
}
