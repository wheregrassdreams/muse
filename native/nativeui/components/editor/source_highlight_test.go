package editor

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
	syntax "muse/markdown/syntax"
)

func TestMermaidSourceTokensPreserveUTF8ByteRanges(t *testing.T) {
	line := "A[开始] --> B[解析] %% 注释"
	tokens := syntax.HighlightMermaidLine(line)
	if len(tokens) == 0 {
		t.Fatal("Mermaid lexer returned no tokens")
	}
	at := 0
	for _, token := range tokens {
		if token.Start < at || token.End > len(line) || token.Start >= token.End {
			t.Fatalf("invalid or overlapping token range: %+v in %q", token, line)
		}
		if !utf8.RuneStart(line[token.Start]) || token.End < len(line) && !utf8.RuneStart(line[token.End]) {
			t.Fatalf("token split a UTF-8 rune: %+v in %q", token, line)
		}
		at = token.End
	}
	assertTokenTextKind(t, line, tokens, "A", syntax.SourceTokenVariable)
	assertTokenTextKind(t, line, tokens, "开始", syntax.SourceTokenString)
	assertTokenTextKind(t, line, tokens, "-->", syntax.SourceTokenOperator)
	assertTokenTextKind(t, line, tokens, "%% 注释", syntax.SourceTokenComment)
}

func assertTokenTextKind(t *testing.T, line string, tokens []syntax.SourceToken, text string, kind syntax.SourceTokenKind) {
	t.Helper()
	for _, token := range tokens {
		if token.Kind == kind && line[token.Start:token.End] == text {
			return
		}
	}
	t.Fatalf("did not find %q with token kind %v in %q: %+v", text, kind, line, tokens)
}

func TestMermaidSourceRevealColorsWithoutRenderingDiagram(t *testing.T) {
	const src = "```mermaid\ngraph LR\nA[开始] --> B[解析]\n%% 注释\n```\n\n尾\n"
	h := newHarness(t, src)
	l := h.e.leaves[0]
	if l.kind != kCode || !isMermaid(l) {
		t.Fatalf("fixture did not parse as Mermaid code: %+v", l)
	}
	h.e.toggleSource(l)
	h.frame()
	l = h.e.leaves[0]
	if l.view == nil || l.view.diagramModel != nil {
		t.Fatal("source reveal still rendered a diagram")
	}
	if !strings.Contains(l.view.disp.text, "graph LR\nA[开始] --> B[解析]") {
		t.Fatalf("source reveal lost Mermaid source: %q", l.view.disp.text)
	}
	h.shot("mermaid-syntax-source-reveal")
	assertCodeViewPiece(t, h, l, "graph", editor.TokenKeyword)
	assertCodeViewPiece(t, h, l, "开始", editor.TokenString)
	assertCodeViewPiece(t, h, l, "-->", editor.TokenOperator)

	// Every visible source piece maps directly to the same UTF-8 byte range.
	for _, piece := range l.view.disp.pieces {
		if piece.kind != pText || piece.s1 <= piece.s0 {
			continue
		}
		a, z := piece.s0+l.lineStart, piece.s1+l.lineStart
		if a < 0 || z > len(src) || piece.text != src[a:z] {
			t.Fatalf("Mermaid piece changed byte mapping: %+v source=%q", piece, src)
		}
	}
	selectStart := strings.Index(src, "解析")
	h.e.anchor, h.e.caret = selectStart, selectStart+len("解析")
	h.frame()
	if got := h.e.SelectedText(); got != "解析" {
		t.Fatalf("CJK selection range moved across Mermaid token styling: %q", got)
	}
}

func assertCodeViewPiece(t *testing.T, h *harness, l *leaf, text string, kind editor.TokenKind) {
	t.Helper()
	for _, piece := range l.view.disp.pieces {
		if piece.kind == pText && piece.text == text {
			want := h.e.lk.code.TokenColor(kind)
			if piece.span.Color != want {
				t.Fatalf("Mermaid token %q color = %+v, want token %v color %+v", text, piece.span.Color, kind, want)
			}
			return
		}
	}
	t.Fatalf("Mermaid source has no visible piece %q", text)
}

func TestInlineMathSourceUsesTeXSyntaxColorsAndKeepsPreview(t *testing.T) {
	const src = "before $\\frac{中文}{2}+x_1$ after\n"
	h := newHarness(t, src)
	formulaStart := strings.Index(src, "$\\frac")
	contentStart := formulaStart + 1
	contentEnd := strings.Index(src[contentStart:], "$") + contentStart
	h.caretAt(contentStart + 2)
	h.frame()
	if h.e.caret != contentStart+2 {
		t.Fatalf("TeX highlighting moved the source caret: got %d want %d", h.e.caret, contentStart+2)
	}
	requireMathCodeStyle(t, h, contentStart, contentEnd)
	l := h.e.leafAt(contentStart)
	assertMathToken(t, h, l, "\\frac", editor.TokenFunction)
	assertMathToken(t, h, l, "{", editor.TokenPunctuation)
	assertMathToken(t, h, l, "+", editor.TokenOperator)
	assertMathToken(t, h, l, "2", editor.TokenNumber)
	h.shot("inline-math-syntax-reveal")

	h.caretAt(len(src))
	h.frame()
	l = h.e.leaves[0]
	foundFormulaObject := false
	for _, piece := range l.view.disp.pieces {
		foundFormulaObject = foundFormulaObject || piece.kind == pAtom && piece.obj != nil && piece.obj.math != nil && piece.obj.math.Image != nil
	}
	if !foundFormulaObject {
		t.Fatal("collapsing source reveal stopped projecting the inline formula raster")
	}
}

func assertMathToken(t *testing.T, h *harness, l *leaf, text string, kind editor.TokenKind) {
	t.Helper()
	for _, piece := range l.view.disp.pieces {
		if piece.kind == pText && piece.text == text {
			if piece.span.Color != h.e.lk.code.TokenColor(kind) {
				t.Fatalf("LaTeX token %q color = %+v, want token %v color %+v", text, piece.span.Color, kind, h.e.lk.code.TokenColor(kind))
			}
			return
		}
	}
	t.Fatalf("inline math source has no token piece %q", text)
}

func TestDisplayMathSourceUsesTeXSyntaxColors(t *testing.T) {
	const src = "$$\n\\frac{中文}{2} + x_1\n$$\n\n尾\n"
	h := newHarness(t, src)
	math := h.e.leaves[0]
	if math.kind != kMath {
		t.Fatalf("fixture is not a display math block: %+v", math)
	}
	h.caretAt(strings.Index(src, "\\frac") + 1)
	h.frame()
	math = h.e.leaves[0]
	assertMathToken(t, h, math, "\\frac", editor.TokenFunction)
	assertMathToken(t, h, math, "2", editor.TokenNumber)
	if math.view == nil || math.view.block == nil || math.view.block.math == nil || math.view.block.math.Image == nil {
		t.Fatal("display math source reveal stopped projecting the formula preview raster")
	}
	h.shot("display-math-syntax-reveal")
}
