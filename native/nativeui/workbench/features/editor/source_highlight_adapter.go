package editor

import (
	"github.com/egoist/mygo/ui"

	syntax "muse/markdown/syntax"
	"muse/nativeui/workbench/features/editor/render"
)

// renderSourceTokens keeps each piece byte-for-byte tied to the same source
// bytes. Syntax owns lexical ranges; the render package maps token kinds to
// the configured presentation palette.
func renderSourceTokens(b *builder, offset int, text string, tokens []syntax.SourceToken, base ui.Span) {
	at := 0
	for _, tok := range tokens {
		start, end := max(0, tok.Start), min(len(text), tok.End)
		if end <= start || start < at {
			continue
		}
		b.codeRun(offset+at, offset+start, base)
		b.codeRun(offset+start, offset+end, render.SourceTokenSpan(base, tok.Kind, b.e.lk.code))
		at = end
	}
	b.codeRun(offset+at, offset+len(text), base)
}
