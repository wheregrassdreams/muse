package editor

import syntax "muse/markdown/syntax"

// ExportHTML renders Markdown with the editor's Markdown parser for export.
// The caller owns the surrounding HTML document and its presentation.
func ExportHTML(source string) string {
	return syntax.ParseIncrementalSource(source).HTML()
}
