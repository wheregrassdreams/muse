package syntax

import "strings"

// ParsePlainText parses source as literal text rather than Markdown: every
// line becomes its own block, so newlines survive and no character is
// treated as syntax. This is what the .txt (and any unknown text) document
// type uses, which is why Muse can open a file that is not Markdown
// without a separate editor.
func ParsePlainText(src string) *Doc {
	doc := &Doc{Source: src}
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	// A file ending in a newline should not draw a trailing blank line.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	for _, line := range lines {
		doc.Blocks = append(doc.Blocks, Block{
			Kind:    Paragraph,
			Inlines: []Span{{Text: line}},
		})
	}
	return doc
}
