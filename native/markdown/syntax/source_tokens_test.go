package syntax

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSourceTokensPreserveUTF8ByteRanges(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		toks []SourceToken
	}{
		{"mermaid", "A[开始] --> B[解析] %% 注释", HighlightMermaidLine("A[开始] --> B[解析] %% 注释")},
		{"tex", `\frac{中文}{2} + \alpha % 注释`, HighlightTeXSource(`\frac{中文}{2} + \alpha % 注释`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := 0
			for _, token := range tc.toks {
				if token.Start < at || token.Start < 0 || token.End > len(tc.text) || token.Start >= token.End {
					t.Fatalf("invalid/overlapping token %+v in %q", token, tc.text)
				}
				if !utf8.RuneStart(tc.text[token.Start]) || token.End < len(tc.text) && !utf8.RuneStart(tc.text[token.End]) {
					t.Fatalf("token splits UTF-8 rune: %+v in %q", token, tc.text)
				}
				at = token.End
			}
		})
	}
}

func TestTeXSourceTokensIdentifyCommandsAndNumbers(t *testing.T) {
	source := `\frac{中文}{2} + x_1`
	tokens := HighlightTeXSource(source)
	wants := map[string]SourceTokenKind{
		`\frac`: SourceTokenFunction,
		"{":     SourceTokenPunctuation,
		"2":     SourceTokenNumber,
		"+":     SourceTokenOperator,
	}
	for text, kind := range wants {
		found := false
		for _, token := range tokens {
			if token.Kind == kind && source[token.Start:token.End] == text {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing TeX token %q with kind %d: %+v", text, kind, tokens)
		}
	}
	if !strings.Contains(source, "中文") {
		t.Fatal("test fixture lost CJK text")
	}
}
