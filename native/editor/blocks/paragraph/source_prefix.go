package paragraph

// CaretAfterLeadingIndent reports whether the caret is positioned after one
// or more literal indentation characters that the Markdown parser excludes
// from a paragraph's inline content. Such characters remain ordinary source
// text for editing, even though block commands normally treat ContentStart as
// the paragraph's structural start.
func CaretAfterLeadingIndent(source string, lineStart, contentStart, caret int) bool {
	if lineStart < 0 || contentStart <= lineStart || contentStart > len(source) || caret <= lineStart || caret > contentStart {
		return false
	}
	for i := lineStart; i < caret; i++ {
		if source[i] != ' ' && source[i] != '\t' {
			return false
		}
	}
	return true
}
