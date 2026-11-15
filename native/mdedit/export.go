package mdedit

import "muse/mdedit/internal/parser"

// ExportHTML renders Markdown with the editor's Markdown parser for export.
// The caller owns the surrounding HTML document and its presentation.
func ExportHTML(source string) string {
	return parser.Parse(source).HTML()
}
