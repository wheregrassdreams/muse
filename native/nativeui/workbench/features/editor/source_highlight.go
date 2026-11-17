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
	for _, run := range render.ProjectSourceTokens(len(text), tokens, base, b.e.lk.code) {
		b.codeRun(offset+run.Start, offset+run.End, run.Style)
	}
}
