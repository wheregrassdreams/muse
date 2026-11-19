package render

import "github.com/wheregrassdreams/muse-editor/markdown/syntax"

// ExportHTML renders Markdown source to HTML without depending on an editor
// session or a display backend.
func ExportHTML(source string) string {
	return syntax.ParseIncrementalSource(source).HTML()
}
