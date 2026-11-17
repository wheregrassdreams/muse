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

	token := SourceTokenKind(kind)
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

// ProjectSourceTokens clips and orders syntax token ranges, filling gaps with
// the base span. The editor host applies the resulting ranges to its display
// pieces and handles toolkit-specific tab expansion.
func ProjectSourceTokens(textLength int, tokens []syntax.SourceToken, base ui.Span, palette *uieditor.Theme) []SourceTokenRun {
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

// SourceTokenKind maps Markdown token semantics to the editor palette.
func SourceTokenKind(kind syntax.SourceTokenKind) uieditor.TokenKind {
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
