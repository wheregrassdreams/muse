package paragraph

// SourcePrefix returns the literal whitespace range before a paragraph's
// parsed content. The parser may omit this range from inline nodes, but it is
// still part of the document source and must keep ordinary source positions.
func SourcePrefix(source string, lineStart, contentStart int) (int, int, bool) {
	if lineStart < 0 || contentStart <= lineStart || contentStart > len(source) {
		return 0, 0, false
	}
	for i := lineStart; i < contentStart; i++ {
		if source[i] != ' ' && source[i] != '\t' {
			return 0, 0, false
		}
	}
	return lineStart, contentStart, true
}
