package render

import (
	"github.com/egoist/mygo/ui"
	uieditor "github.com/egoist/mygo/ui/editor"
)

// SourceTokenKind is a small host-neutral syntax category. A parser or
// language adapter converts its token kinds to this presentation vocabulary.
type SourceTokenKind uint8

const (
	TokenText SourceTokenKind = iota
	TokenKeyword
	TokenType
	TokenFunction
	TokenVariable
	TokenNumber
	TokenString
	TokenEscape
	TokenComment
	TokenOperator
	TokenPunctuation
)

// SourceToken is a byte range and neutral syntax category, detached from any
// parser's syntax tree or token type.
type SourceToken struct {
	Start, End int
	Kind       SourceTokenKind
}

// SourceTokenSpan resolves a neutral token to the active MyGo palette.
func SourceTokenSpan(base ui.Span, kind SourceTokenKind, palette *uieditor.Theme) ui.Span {
	if palette == nil {
		return base
	}
	token := mygoTokenKind(kind)
	span := base
	if color := palette.TokenColor(token); color.A != 0 {
		span.Color = color
	}
	style := palette.Tokens[token]
	span.Italic = style.Italic
	if style.Bold {
		span.Weight = 700
	}
	return span
}

// SourceTokenRun is a styled byte range within one code/source line.
type SourceTokenRun struct {
	Start, End int
	Style      ui.Span
}

// ProjectSourceTokens clips and orders parser-independent syntax token
// ranges, filling gaps with the base span.
func ProjectSourceTokens(textLength int, tokens []SourceToken, base ui.Span, palette *uieditor.Theme) []SourceTokenRun {
	if textLength <= 0 {
		return nil
	}
	runs := make([]SourceTokenRun, 0, len(tokens)*2+1)
	at := 0
	for _, token := range tokens {
		start, end := max(0, token.Start), min(textLength, token.End)
		if end <= start || start < at {
			continue
		}
		if start > at {
			runs = append(runs, SourceTokenRun{Start: at, End: start, Style: base})
		}
		runs = append(runs, SourceTokenRun{Start: start, End: end, Style: SourceTokenSpan(base, token.Kind, palette)})
		at = end
	}
	if at < textLength {
		runs = append(runs, SourceTokenRun{Start: at, End: textLength, Style: base})
	}
	return runs
}

func mygoTokenKind(kind SourceTokenKind) uieditor.TokenKind {
	switch kind {
	case TokenKeyword:
		return uieditor.TokenKeyword
	case TokenType:
		return uieditor.TokenType
	case TokenFunction:
		return uieditor.TokenFunction
	case TokenVariable:
		return uieditor.TokenVariable
	case TokenNumber:
		return uieditor.TokenNumber
	case TokenString:
		return uieditor.TokenString
	case TokenEscape:
		return uieditor.TokenEscape
	case TokenComment:
		return uieditor.TokenComment
	case TokenOperator:
		return uieditor.TokenOperator
	case TokenPunctuation:
		return uieditor.TokenPunctuation
	default:
		return uieditor.TokenText
	}
}
