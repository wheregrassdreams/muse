package render

import "muse/markdown/syntax"

// ExportHTML renders Markdown source to HTML without depending on an editor
// session or a display backend.
func ExportHTML(source string) string {
	return syntax.ParseIncrementalSource(source).HTML()
}
