package markdown

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCodeHighlighterPreservesCodeAndPalette(t *testing.T) {
	tests := []struct {
		name string
		lang string
		line string
		want map[string]ui.Color
	}{
		{
			name: "go tokens",
			lang: "go",
			line: `func run() { return 17 }`,
			want: map[string]ui.Color{
				"func": paletteFor(false).keyword,
				"run":  paletteFor(false).function,
				"17":   paletteFor(false).number,
			},
		},
		{
			name: "html attributes",
			lang: "html",
			line: `<a href="x">link</a>`,
			want: map[string]ui.Color{
				"a":    paletteFor(false).tag,
				"href": paletteFor(false).attr,
				`"x"`:  paletteFor(false).str,
				"</":   ui.Color{},
			},
		},
		{
			name: "css properties",
			lang: "css",
			line: `.card { color: #fff; }`,
			want: map[string]ui.Color{
				"color": paletteFor(false).attr,
				"#fff":  paletteFor(false).number,
			},
		},
		{
			name: "unknown language keeps generic legacy colors",
			lang: "unknown-language",
			line: `const x = 17; call("text"); null // note`,
			want: map[string]ui.Color{
				"const":   ui.Color{},
				"17":      paletteFor(false).number,
				"call":    paletteFor(false).function,
				`"text"`:  paletteFor(false).str,
				"null":    paletteFor(false).keyword,
				"// note": paletteFor(false).comment,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			highlighter := NewCodeHighlighter(tt.lang, false)
			if tt.lang == "unknown-language" && !highlighter.fallback {
				t.Fatalf("unknown language unexpectedly resolved to %T", highlighter.tokenizer)
			}
			spans := highlighter.Line(tt.line)
			if got := joinSpanText(spans); got != tt.line {
				t.Fatalf("highlight changed source: got %q, want %q", got, tt.line)
			}
			for text, want := range tt.want {
				if got := spanColorFor(spans, text); got != want {
					t.Errorf("%q color = %#v, want %#v; spans=%#v", text, got, want, spans)
				}
			}
		})
	}
}

func TestCodeHighlighterKeepsTokenizerStateAcrossLines(t *testing.T) {
	highlighter := NewCodeHighlighter("go", true)
	comment := paletteFor(true).comment
	for _, line := range []string{"/* comment starts", "and continues", "then ends */ func run() {}"} {
		spans := highlighter.Line(line)
		if got := joinSpanText(spans); got != line {
			t.Fatalf("highlight changed source: got %q, want %q", got, line)
		}
		if line != "then ends */ func run() {}" {
			if got := spanColorFor(spans, strings.TrimSpace(line)); got != comment {
				t.Errorf("line %q color = %#v, want comment %#v", line, got, comment)
			}
			continue
		}
		if got := spanColorFor(spans, "then ends */"); got != comment {
			t.Errorf("closing comment color = %#v, want %#v", got, comment)
		}
		if got := spanColorFor(spans, "func"); got != paletteFor(true).keyword {
			t.Errorf("keyword after comment color = %#v, want %#v", got, paletteFor(true).keyword)
		}
	}
}

func joinSpanText(spans []ui.Span) string {
	var joined strings.Builder
	for _, span := range spans {
		joined.WriteString(span.Text)
	}
	return joined.String()
}

func spanColorFor(spans []ui.Span, text string) ui.Color {
	for _, span := range spans {
		if span.Text == text {
			return span.Color
		}
	}
	return ui.Color{}
}

func BenchmarkHighlightLongCodeBlock(b *testing.B) {
	const line = `func renderMarkdown(source string) error { /* retain code bytes */ return parse(source, 1024) }`
	lines := make([]string, 1024)
	for i := range lines {
		lines[i] = line
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(line) * len(lines)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		highlighter := NewCodeHighlighter("go", true)
		for _, source := range lines {
			_ = highlighter.Line(source)
		}
	}
}
