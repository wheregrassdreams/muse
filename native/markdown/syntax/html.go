package syntax

import (
	"html"
	"strings"
)

// HTMLText extracts readable text from common HTML fragments embedded in
// Markdown. It strips script/style bodies and preserves common block breaks.
func HTMLText(s string) string {
	var b strings.Builder
	skip := 0
	for i := 0; i < len(s); {
		if s[i] != '<' {
			if skip == 0 {
				b.WriteByte(s[i])
			}
			i++
			continue
		}
		end := strings.IndexByte(s[i:], '>')
		if end < 0 {
			break
		}
		tag := strings.TrimSpace(s[i+1 : i+end])
		low := strings.ToLower(tag)
		switch {
		case strings.HasPrefix(low, "script"), strings.HasPrefix(low, "style"):
			skip++
		case strings.HasPrefix(low, "/script"), strings.HasPrefix(low, "/style"):
			if skip > 0 {
				skip--
			}
		case skip == 0 && strings.HasPrefix(low, "br"):
			b.WriteByte('\n')
		case skip == 0 && strings.HasPrefix(low, "img"):
			if alt := HTMLAttribute(tag, "alt"); alt != "" {
				b.WriteString("🖼 ")
				b.WriteString(alt)
				b.WriteByte('\n')
			}
		case skip == 0 && (strings.HasPrefix(low, "/p") || strings.HasPrefix(low, "/div") ||
			strings.HasPrefix(low, "/h") || strings.HasPrefix(low, "/li") ||
			strings.HasPrefix(low, "/tr") || strings.HasPrefix(low, "hr")):
			b.WriteByte('\n')
		}
		i += end + 1
	}
	txt := html.UnescapeString(b.String())
	lines := strings.Split(txt, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// HTMLAttribute reads a simple quoted or unquoted HTML attribute value.
func HTMLAttribute(tag, name string) string {
	low := strings.ToLower(tag)
	i := strings.Index(low, name+"=")
	if i < 0 {
		return ""
	}
	rest := tag[i+len(name)+1:]
	if rest == "" {
		return ""
	}
	q := rest[0]
	if q != '"' && q != '\'' {
		end := strings.IndexAny(rest, " \t>")
		if end < 0 {
			return rest
		}
		return rest[:end]
	}
	end := strings.IndexByte(rest[1:], q)
	if end < 0 {
		return ""
	}
	return rest[1 : end+1]
}
