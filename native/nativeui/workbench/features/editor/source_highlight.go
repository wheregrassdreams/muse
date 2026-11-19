package editor

import (
	"github.com/egoist/mygo/ui"

	"muse/nativeui/workbench/features/editor/render"
	syntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"
)

// renderSourceTokens keeps each piece byte-for-byte tied to the same source
// bytes. Syntax owns lexical ranges; the render package maps token kinds to
// the configured presentation palette.
func renderSourceTokens(b *builder, offset int, text string, tokens []syntax.SourceToken, base ui.Span) {
	portable := make([]render.SourceToken, 0, len(tokens))
	for _, token := range tokens {
		portable = append(portable, render.SourceToken{Start: token.Start, End: token.End, Kind: render.SourceTokenKind(token.Kind)})
	}
	for _, run := range render.ProjectSourceTokens(len(text), portable, base, b.e.lk.code) {
		b.codeRun(offset+run.Start, offset+run.End, run.Style)
	}
}
