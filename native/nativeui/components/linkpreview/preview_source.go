package linkpreview

import (
	"path/filepath"
	"regexp"
	"strings"
)

var previewWikiReference = regexp.MustCompile(`!?\[\[([^\]]+)\]\]`)

// PreviewSource projects a note source into the read-only Markdown shown by a
// hover card. It removes frontmatter, narrows a heading link to its section,
// and renders wiki references as their visible labels.
func PreviewSource(source string, byteOffset int) string {
	if strings.HasPrefix(source, "---\n") || strings.HasPrefix(source, "---\r\n") {
		lines := strings.SplitAfter(source, "\n")
		end := -1
		for i := 1; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			if line == "---" || line == "..." {
				end = i + 1
				break
			}
		}
		if end > 0 {
			source = strings.Join(lines[end:], "")
			byteOffset = max(0, byteOffset-len(strings.Join(lines[:end], "")))
		}
	}
	if byteOffset > 0 && byteOffset < len(source) {
		// For a heading target, show that section from its heading through the
		// next heading at the same or a higher level.
		start := strings.LastIndex(source[:byteOffset], "\n") + 1
		line := strings.TrimSpace(source[start:byteOffset])
		level := 0
		for level < len(line) && line[level] == '#' {
			level++
		}
		if level > 0 && level <= 6 {
			lines := strings.Split(source[start:], "\n")
			end := len(lines)
			for i := 1; i < len(lines); i++ {
				candidate := strings.TrimSpace(lines[i])
				n := 0
				for n < len(candidate) && candidate[n] == '#' {
					n++
				}
				if n > 0 && n <= level && (len(candidate) == n || candidate[n] == ' ') {
					end = i
					break
				}
			}
			source = strings.Join(lines[:end], "\n")
		}
	}
	source = previewWikiReference.ReplaceAllStringFunc(source, func(ref string) string {
		parts := previewWikiReference.FindStringSubmatch(ref)
		if len(parts) != 2 {
			return ref
		}
		target, alias, _ := strings.Cut(parts[1], "|")
		label := strings.TrimSpace(alias)
		if label == "" {
			label = strings.TrimSpace(strings.SplitN(target, "#", 2)[0])
			label = strings.TrimSuffix(filepath.Base(filepath.FromSlash(label)), filepath.Ext(label))
		}
		if strings.HasPrefix(ref, "!") {
			return label
		}
		return "**" + label + "**"
	})
	return strings.TrimSpace(source)
}
