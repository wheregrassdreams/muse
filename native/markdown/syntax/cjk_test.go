package syntax

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// boldRuns returns the text of a paragraph's bold spans, joined by |.
func boldRuns(spans []Span) string {
	var out []string
	for _, s := range spans {
		if s.Bold {
			out = append(out, s.Text)
		}
	}
	return strings.Join(out, "|")
}

func hasCode(spans []Span, text string) bool {
	for _, span := range spans {
		if span.Code && span.Text == text {
			return true
		}
	}
	return false
}

func TestStrongBesideCJKPunctuation(t *testing.T) {
	for _, c := range []struct{ src, text, bold string }{
		{"**注意：**这是**重点**内容。", "注意：这是重点内容。", "注意：|重点"},
		{"中文**（括号）**中文", "中文（括号）中文", "（括号）"},
		{"**English:**中文", "English:中文", "English:"},
		{"“我说**是**”", "“我说是”", "是"},
		{"**“名称”**：**值**", "“名称”：值", "“名称”|值"},
	} {
		d := Parse(c.src)
		p := d.Blocks[0].Inlines
		if got := spansText(p); got != c.text {
			t.Errorf("%q reads %q, want %q", c.src, got, c.text)
		}
		if got := boldRuns(p); got != c.bold {
			t.Errorf("%q has bold %q, want %q", c.src, got, c.bold)
		}
	}
	d := Parse("- **步骤一：**安装依赖\n- **步骤二：**运行\n")
	if it := d.Blocks[0].Items; len(it) != 2 || boldRuns(it[0].Inlines) != "步骤一：" || spansText(it[1].Inlines) != "步骤二：运行" {
		t.Errorf("list = %+v", d.Blocks[0])
	}
}

func TestCJKFriendlyKeepsOffsets(t *testing.T) {
	src := "**注意：**内容，**（括号）**中文，`**代码：**中`\n\n```\n**注意：**中文\n```\n"
	got := CJKFriendly(src)
	if len(got) != len(src) || got == src {
		t.Fatalf("CJKFriendly(%q) = %q", src, got)
	}
	for i := 0; i < len(src); {
		r, n := utf8.DecodeRuneInString(src[i:])
		if r < utf8.RuneSelf && got[i] != src[i] {
			t.Fatalf("ASCII at %d changed: %q", i, got[i])
		}
		if g, gn := utf8.DecodeRuneInString(got[i:]); gn != n || g == utf8.RuneError {
			t.Fatalf("character at %d changed length: %q for %q", i, g, r)
		}
		i += n
	}
	if plain := "No CJK here: **bold** and *em*."; CJKFriendly(plain) != plain {
		t.Fatal("text without CJK changed")
	}
	// The text of a code block is the source's.
	if d := Parse(src); d.Blocks[1].Text != "**注意：**中文\n" {
		t.Fatalf("code = %q", d.Blocks[1].Text)
	}
}

func TestCJKFriendlyMatchesWholeDocumentAlgorithm(t *testing.T) {
	inputs := []string{
		"纯中文文本，没有任何强调标记。\n继续这一段内容。\n",
		"**注意：**这是**重点**内容。\nplain **English**\r\n",
		"> **引用：**内容\n> ~~删除：~~项\n- **列表（步骤一）：**安装\n",
		"**跨行\n强调**\n**English:**\n中文\n",
		"\\**转义：**和 \\\\**未转义：**内容\r\n",
		"中文__下划线__以及~~删除：~~，再来***三重强调***。\n",
		"emoji 😀**中文**与 𝄀**符号**。\n",
		"“**全角：**”\ufe01\n\n>   **引用标题：**\r\n",
	}
	inputs = append(inputs, strings.Repeat("**注意：**与 _中文_、~~删除：~~\nplain **English**\r\n> **引用：**\n", 40))
	for _, src := range inputs {
		got, want := CJKFriendly(src), cjkFriendlyWholeDocument(src)
		if got != want {
			t.Errorf("CJKFriendly differs from whole-document algorithm for %q\ngot  %q\nwant %q", src, got, want)
		}
	}
}

func BenchmarkCJKFriendlyStress(b *testing.B) {
	var srcBuilder strings.Builder
	for srcBuilder.Len() < 1<<20 {
		srcBuilder.WriteString("一段包含 **注意：**强调、`代码` 与 [链接](https://example.com) 的中文文本。\n\nplain **English**\r\n")
	}
	src := srcBuilder.String()
	b.Run("line-buffer", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = CJKFriendly(src)
		}
	})
	b.Run("whole-document-reference", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = cjkFriendlyWholeDocument(src)
		}
	})
}

func BenchmarkCJKFriendlyPlainCJK(b *testing.B) {
	const unit = "这是一段没有强调标记的中文文档，包含常见中文标点和数字123。\n"
	var srcBuilder strings.Builder
	for srcBuilder.Len() < 1<<20 {
		srcBuilder.WriteString(unit)
	}
	src := srcBuilder.String()
	b.SetBytes(int64(len(src)))
	for _, tc := range []struct {
		name string
		fn   func(string) string
	}{
		{name: "no-delimiter-fast-path", fn: CJKFriendly},
		{name: "line-scanner", fn: cjkFriendlyScan},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = tc.fn(src)
			}
		})
	}
}

// cjkFriendlyWholeDocument preserves the pre-optimization algorithm as an
// oracle while CJKFriendly processes the source one line at a time.
func cjkFriendlyWholeDocument(src string) string {
	if !hasNonASCIIByte(src) {
		return src
	}
	runs := legacyDelimRuns(src)
	if len(runs) == 0 {
		return src
	}
	m := masker{src: src, runs: runs}
	for k := range runs {
		runs[k].open, runs[k].close = m.can(k, true)
	}
	swapped := false
	for k := range runs {
		if !m.matches(k) && m.fix(k) {
			swapped = true
		}
	}
	if !swapped {
		return src
	}
	b := []byte(src)
	for _, r := range runs {
		if r.sb != 0 {
			utf8.EncodeRune(b[r.before:], r.sb)
		}
		if r.sa != 0 {
			utf8.EncodeRune(b[r.after:], r.sa)
		}
	}
	return string(b)
}

func legacyDelimRuns(src string) []delimRun {
	var runs []delimRun
	wide := false
	lead := true
	for i := 0; i < len(src); {
		next := strings.IndexAny(src[i:], "*_~\n")
		if next < 0 {
			break
		}
		if next > 0 && lead && strings.TrimLeft(src[i:i+next], " \t>") != "" {
			lead = false
		}
		i += next
		c := src[i]
		if c == '\n' {
			lead = true
			i++
			continue
		}
		j := i
		for j < len(src) && src[j] == c {
			j++
		}
		s := i
		if escapedAt(src, i) {
			s++
		}
		i = j
		if s == j || c == '~' && j-s > 2 {
			lead = false
			continue
		}
		r := delimRun{start: s, end: j, c: c, before: -1, after: -1, br: '\n', ar: '\n'}
		if s > 0 {
			if p, n := utf8.DecodeLastRuneInString(src[:s]); p != '\n' && p != '\r' {
				r.before, r.br = s-n, p
				r.lead = lead
			}
		}
		if j < len(src) && src[j] != '\n' && src[j] != '\r' {
			r.after = j
			r.ar, _ = utf8.DecodeRuneInString(src[j:])
		}
		lead = false
		wide = wide || r.br >= utf8.RuneSelf || r.ar >= utf8.RuneSelf
		runs = append(runs, r)
	}
	if !wide {
		return nil
	}
	return runs
}

func TestCJKFriendlyASCIIIsUnchangedWithoutAllocations(t *testing.T) {
	src := strings.Repeat("A", 1<<20)
	if got := CJKFriendly(src); got != src {
		t.Fatal("ASCII input changed")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = CJKFriendly(src) }); allocs != 0 {
		t.Fatalf("CJKFriendly allocated %g times for ASCII input", allocs)
	}
}

func TestParseResolvesEscapesAndReferences(t *testing.T) {
	d := Parse("\\*not em\\* a\\_b &amp; &copy; &#x4e2d; 1 \\< 2 `a\\_b` &bogus;\n")
	p := d.Blocks[0].Inlines
	if got := spansText(p); got != "*not em* a_b & © 中 1 < 2 a\\_b &bogus;" {
		t.Fatalf("text = %q", got)
	}
	if !hasCode(p, "a\\_b") {
		t.Fatalf("code lost its backslash: %+v", p)
	}
}
