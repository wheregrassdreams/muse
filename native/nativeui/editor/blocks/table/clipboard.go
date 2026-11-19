package tableui

import (
	"strings"

	syntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"
	tablemodel "github.com/wheregrassdreams/muse-editor/markdown/table"
)

// ClipboardHTML serializes a detached rectangular table selection as HTML.
// The Markdown parser remains the authority for cell escaping and markup;
// this adapter only restores whether the first selected row was the source
// header or a body row.
func ClipboardHTML(register tablemodel.Register) string {
	if !register.Region() || len(register.Rows()) == 0 {
		return ""
	}
	html := syntax.ParseIncrementalSource(register.Markdown()).HTML()
	if !register.SourceHeader() {
		html = strings.ReplaceAll(html, "<th>", "<td>")
		html = strings.ReplaceAll(html, "</th>", "</td>")
		html = strings.ReplaceAll(html, "<thead>", "<tbody>")
		html = strings.ReplaceAll(html, "</thead>", "</tbody>")
	}
	return html
}
