package mdedit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// benchmarkSource builds a mixed Markdown document of at least size bytes:
// front matter, headings, CJK prose with emphasis and inline code, lists,
// and occasional fences and tables, so the mask and the parser see every
// kind of line.
func benchmarkSource(size int) string {
	var b strings.Builder
	b.WriteString("---\ntitle: 性能\n---\n\n")
	for n := 0; b.Len() < size; n++ {
		b.WriteString("## 小节\n\n")
		for i := 0; i < 4; i++ {
			b.WriteString("一段包含 **注意：**强调、`代码`、$x^2$ 与 [链接](https://example.com) 的文字，用来让这一行足够长并带有多种行内语法。\n\n")
		}
		b.WriteString("- 一项\n- 另一项\n\n")
		if n%4 == 0 {
			b.WriteString("```go\nfunc f() {}\n```\n\n")
			b.WriteString("| 列 | 值 |\n| --- | --- |\n| a | b |\n\n")
		}
	}
	return b.String()
}

// benchmarkProse returns the offset just inside a line of prose near the
// middle of src, where a keystroke does not change the block structure
// around it and both halves of the document are already parsed.
func benchmarkProse(tb testing.TB, src string) int {
	mid := len(src) / 2
	i := strings.Index(src[mid:], "一段")
	if i < 0 {
		i = strings.LastIndex(src[:mid], "一段")
		if i < 0 {
			tb.Fatal("the benchmark source has no prose")
		}
		mid = i
	} else {
		mid += i
	}
	return mid + len("一段")
}

// BenchmarkSyncAfterEdit measures giving the parser one edit and making
// the leaves again, without a frame or a layout.
func BenchmarkSyncAfterEdit(b *testing.B) {
	src := benchmarkSource(1 << 20)
	e := New(src, Options{})
	mid := benchmarkProse(b, src)
	// Warm the parser and the mask.
	e.begin(stepEdit)
	e.replace(mid, mid, "x")
	e.parsed = 0
	e.sync()
	e.hist.open = nil
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.begin(stepEdit)
		// Insert and take back the same byte, so the document stays the
		// same size while every iteration still edits a line.
		if i%2 == 0 {
			e.replace(mid, mid, "x")
		} else {
			e.replace(mid, mid+1, "")
		}
		e.parsed = 0
		e.sync()
		e.hist.open = nil
	}
}

// BenchmarkTypingFrame measures a keystroke end to end: the edit, the
// parse, the layout and a frame of the window, as typing in a large
// document does.
func BenchmarkTypingFrame(b *testing.B) {
	src := benchmarkSource(1 << 20)
	e := New(src, Options{FontSize: 16})
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			e.View(c).Grow(1)
		})
	}, 1000, 700)
	tt.Frame()
	tt.Frame()
	e.Focus()
	tt.Frame()
	tt.Frame()
	mid := benchmarkProse(b, src)
	e.anchor, e.caret = mid, mid
	e.wantReveal = true
	tt.Frame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.begin(stepTyping)
		if i%2 == 0 {
			e.replace(mid, mid, "x")
		} else {
			e.replace(mid, mid+1, "")
		}
		e.commit(e.caret, e.caret)
		tt.Frame()
	}
}

// BenchmarkLayoutAfterEdit measures a keystroke as the editor lays the
// document out again: the parse, the leaves and the layout of every leaf,
// with the views of the leaves the edit did not change kept.
func BenchmarkLayoutAfterEdit(b *testing.B) {
	src := benchmarkSource(1 << 20)
	e := New(src, Options{})
	e.colW = 720
	mid := benchmarkProse(b, src)
	e.begin(stepEdit)
	e.replace(mid, mid, "x")
	e.parsed = 0
	e.layoutIfNeeded()
	e.hist.open = nil
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.begin(stepEdit)
		if i%2 == 0 {
			e.replace(mid, mid, "x")
		} else {
			e.replace(mid, mid+1, "")
		}
		e.parsed = 0
		e.layoutIfNeeded()
		e.hist.open = nil
	}
}
