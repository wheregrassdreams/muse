package markdown

import (
	"strings"

	"github.com/egoist/mygo/ui"
	mygoeditor "github.com/egoist/mygo/ui/editor"
)

// codePalette keeps static Markdown code blocks in the app's existing palette.
// Tokenization itself is shared with MyGo's editor language registry.
type codePalette struct {
	keyword  ui.Color
	str      ui.Color
	comment  ui.Color
	number   ui.Color
	function ui.Color
	tag      ui.Color
	attr     ui.Color
}

func paletteFor(dark bool) codePalette {
	if dark {
		return codePalette{
			keyword:  ui.Hex("#ff7b72"),
			str:      ui.Hex("#a5d6ff"),
			comment:  ui.Hex("#8b949e"),
			number:   ui.Hex("#79c0ff"),
			function: ui.Hex("#d2a8ff"),
			tag:      ui.Hex("#7ee787"),
			attr:     ui.Hex("#d2a8ff"),
		}
	}
	return codePalette{
		keyword:  ui.Hex("#d73a49"),
		str:      ui.Hex("#032f62"),
		comment:  ui.Hex("#6a737d"),
		number:   ui.Hex("#005cc5"),
		function: ui.Hex("#6f42c1"),
		tag:      ui.Hex("#22863a"),
		attr:     ui.Hex("#6f42c1"),
	}
}

// CodeHighlighter carries tokenizer state across lines in one fenced block and
// reuses MyGo's token buffer. Each returned span owns only the line substring
// headers; the source bytes remain unchanged.
type CodeHighlighter struct {
	tokenizer mygoeditor.Tokenizer
	state     uint64
	tokens    []mygoeditor.Token
	palette   codePalette
	fallback  bool
}

// NewCodeHighlighter creates a highlighter for a Markdown fence language.
// JSX and TSX use the same markup presentation as before this adapter; other
// known language names resolve through MyGo's public language registry.
func NewCodeHighlighter(lang string, dark bool) *CodeHighlighter {
	name := strings.ToLower(strings.TrimSpace(lang))
	if name == "jsx" || name == "tsx" {
		name = "html"
	}
	var tokenizer mygoeditor.Tokenizer
	if language := mygoeditor.LanguageNamed(name); language != nil {
		tokenizer = language.Tokenizer
	}
	fallback := tokenizer == nil
	if fallback {
		// The legacy highlighter still colored strings, numbers, function calls,
		// comments, and the common literal words for unknown fence names. Use
		// MyGo's shared lexer for those tokens, but suppress language-specific
		// Go keywords so unknown names keep their generic appearance.
		if language := mygoeditor.LanguageNamed("go"); language != nil {
			tokenizer = language.Tokenizer
		}
	}
	return &CodeHighlighter{tokenizer: tokenizer, palette: paletteFor(dark), fallback: fallback}
}

// Line highlights one source line and returns spans whose concatenated text is
// byte-for-byte equal to line. Unknown languages keep the legacy generic token
// colors without applying Go-specific keyword colors.
func (h *CodeHighlighter) Line(line string) []ui.Span {
	if line == "" {
		return []ui.Span{{Text: " "}}
	}
	if h == nil || h.tokenizer == nil {
		return []ui.Span{{Text: line}}
	}
	h.tokens, h.state = h.tokenizer.Tokenize(line, h.state, h.tokens[:0])
	spans := make([]ui.Span, 0, len(h.tokens))
	start := 0
	for _, token := range h.tokens {
		end := min(max(token.End, start), len(line))
		if end == start {
			continue
		}
		text := line[start:end]
		spans = append(spans, ui.Span{Text: text, Color: h.tokenColor(token.Kind, text)})
		start = end
	}
	if start < len(line) {
		spans = append(spans, ui.Span{Text: line[start:]})
	}
	if len(spans) == 0 {
		return []ui.Span{{Text: line}}
	}
	return spans
}

func (h *CodeHighlighter) tokenColor(kind mygoeditor.TokenKind, text string) ui.Color {
	if h.fallback {
		switch kind {
		case mygoeditor.TokenFunction, mygoeditor.TokenNumber, mygoeditor.TokenString,
			mygoeditor.TokenEscape, mygoeditor.TokenComment:
			return h.colorForKind(kind)
		case mygoeditor.TokenKeyword, mygoeditor.TokenConstant, mygoeditor.TokenText, mygoeditor.TokenVariable:
			switch text {
			case "true", "false", "null", "nil":
				return h.palette.keyword
			}
		}
		return ui.Color{}
	}
	return h.colorForKind(kind)
}

func (h *CodeHighlighter) colorForKind(kind mygoeditor.TokenKind) ui.Color {
	switch kind {
	case mygoeditor.TokenKeyword, mygoeditor.TokenConstant:
		return h.palette.keyword
	case mygoeditor.TokenFunction:
		return h.palette.function
	case mygoeditor.TokenNumber:
		return h.palette.number
	case mygoeditor.TokenString, mygoeditor.TokenEscape:
		return h.palette.str
	case mygoeditor.TokenComment:
		return h.palette.comment
	case mygoeditor.TokenTag:
		return h.palette.tag
	case mygoeditor.TokenAttribute, mygoeditor.TokenProperty:
		return h.palette.attr
	default:
		return ui.Color{}
	}
}

// HighlightLine is the stateless single-line convenience used by callers and
// tests. Code block rendering should keep a CodeHighlighter for its full block
// so multiline comments and strings retain tokenizer state.
func HighlightLine(line, lang string, dark bool) []ui.Span {
	return NewCodeHighlighter(lang, dark).Line(line)
}
