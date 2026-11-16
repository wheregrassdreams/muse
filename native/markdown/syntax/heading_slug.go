package syntax

import "strings"

// HeadingSlug returns the fragment identifier used for a Markdown heading.
// It follows the editor's GitHub-style behavior: spaces and hyphens become
// hyphens, ASCII punctuation is removed, and non-ASCII letters are preserved.
func HeadingSlug(text string) string {
	var slug strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(text)) {
		switch {
		case r == ' ' || r == '-':
			slug.WriteRune('-')
		case r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r > 127:
			slug.WriteRune(r)
		}
	}
	return slug.String()
}
