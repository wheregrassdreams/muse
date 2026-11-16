package render

import (
	"github.com/egoist/mygo/ui"
	uieditor "github.com/egoist/mygo/ui/editor"

	syntax "muse/markdown/syntax"
)

// SourceTokenSpan resolves a Markdown source token to the active editor
// palette. Token ranges and source projection remain with the editor host.
func SourceTokenSpan(base ui.Span, kind syntax.SourceTokenKind, palette *uieditor.Theme) ui.Span {
	if palette == nil {
		return base
	}

	token := sourceTokenKind(kind)
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

func sourceTokenKind(kind syntax.SourceTokenKind) uieditor.TokenKind {
	switch kind {
	case syntax.SourceTokenKeyword:
		return uieditor.TokenKeyword
	case syntax.SourceTokenType:
		return uieditor.TokenType
	case syntax.SourceTokenFunction:
		return uieditor.TokenFunction
	case syntax.SourceTokenVariable:
		return uieditor.TokenVariable
	case syntax.SourceTokenNumber:
		return uieditor.TokenNumber
	case syntax.SourceTokenString:
		return uieditor.TokenString
	case syntax.SourceTokenEscape:
		return uieditor.TokenEscape
	case syntax.SourceTokenComment:
		return uieditor.TokenComment
	case syntax.SourceTokenOperator:
		return uieditor.TokenOperator
	case syntax.SourceTokenPunctuation:
		return uieditor.TokenPunctuation
	default:
		return uieditor.TokenText
	}
}
