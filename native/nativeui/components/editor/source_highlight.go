package editor

import (
	"github.com/egoist/mygo/ui"
	uieditor "github.com/egoist/mygo/ui/editor"

	syntax "muse/markdown/syntax"
)

func sourceTokenSpan(base ui.Span, kind syntax.SourceTokenKind, palette *uieditor.Theme) ui.Span {
	if palette == nil {
		return base
	}
	paletteKind := sourceTokenPaletteKind(kind)
	sp := base
	if c := palette.TokenColor(paletteKind); c.A != 0 {
		sp.Color = c
	}
	ts := palette.Tokens[paletteKind]
	sp.Italic = ts.Italic
	if ts.Bold {
		sp.Weight = 700
	}
	return sp
}

func sourceTokenPaletteKind(kind syntax.SourceTokenKind) uieditor.TokenKind {
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

// renderSourceTokens keeps each piece byte-for-byte tied to the same source
// bytes. Syntax owns lexical ranges; the editor maps neutral kinds to the
// configured presentation palette.
func renderSourceTokens(b *builder, offset int, text string, tokens []syntax.SourceToken, base ui.Span) {
	at := 0
	for _, tok := range tokens {
		start, end := max(0, tok.Start), min(len(text), tok.End)
		if end <= start || start < at {
			continue
		}
		b.codeRun(offset+at, offset+start, base)
		b.codeRun(offset+start, offset+end, sourceTokenSpan(base, tok.Kind, b.e.lk.code))
		at = end
	}
	b.codeRun(offset+at, offset+len(text), base)
}
