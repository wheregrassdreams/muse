package vaultindex

import (
	"os"
	"regexp"
	"strings"
	"unicode"
)

var blockRE = regexp.MustCompile(`\^([A-Za-z0-9_-]+)\s*$`)

func readDocument(abs, rel string) (*Document, error) {
	b, e := os.ReadFile(abs)
	if e != nil {
		return nil, e
	}
	return parseDocument(rel, string(b)), nil
}

func parseDocument(path, source string) *Document {
	d := &Document{Path: cleanPath(path), Source: source}
	body := source
	if strings.HasPrefix(body, "---\n") || strings.HasPrefix(body, "---\r\n") {
		if end := frontmatterEnd(body); end >= 0 {
			d.Title, d.Aliases, d.Tags = parseFrontmatter(body[:end])
			body = body[end:]
		}
	}
	visible := maskNonContent(body)
	if d.Title == "" {
		for _, line := range strings.Split(visible, "\n") {
			s := strings.TrimSpace(line)
			if strings.HasPrefix(s, "# ") {
				d.Title = strings.TrimSpace(strings.TrimLeft(s, "# "))
				break
			}
		}
	}
	if d.Title == "" {
		base := d.Path[strings.LastIndex(d.Path, "/")+1:]
		d.Title = strings.TrimSuffix(base, base[strings.LastIndex(base, "."):])
	}
	baseOffset := len(source) - len(body)
	lines := strings.SplitAfter(visible, "\n")
	offset := 0
	for _, line := range lines {
		trim := strings.TrimLeft(line, " \t")
		indent := len(line) - len(trim)
		if indent <= 3 && strings.HasPrefix(trim, "#") {
			n := 0
			for n < len(trim) && trim[n] == '#' {
				n++
			}
			if n >= 1 && n <= 6 && n < len(trim) && unicode.IsSpace(rune(trim[n])) {
				text := strings.TrimSpace(strings.TrimLeft(trim[n:], " \t"))
				text = stripClosingHashes(text)
				if text != "" {
					d.Headings = append(d.Headings, Heading{Text: text, Slug: slugify(text), Offset: baseOffset + offset + indent})
				}
			}
		}
		if m := blockRE.FindStringSubmatch(strings.TrimSpace(line)); len(m) == 2 {
			d.Blocks = append(d.Blocks, BlockID{ID: m[1], Offset: baseOffset + offset + strings.Index(line, strings.TrimSpace(line))})
		}
		offset += len(line)
	}
	d.Links = parseLinks(d.Path, body, visible, baseOffset)
	d.SearchText = source
	return d
}

// maskNonContent preserves byte positions while hiding fenced/inline code and
// HTML comments. It intentionally does not implement every Markdown dialect.
func maskNonContent(s string) string {
	b := []byte(s)
	inFence := byte(0)
	fenceLen := 0
	inComment := false
	commentEnd := ""
	for p := 0; p < len(s); {
		lineEnd := strings.IndexByte(s[p:], '\n')
		if lineEnd < 0 {
			lineEnd = len(s)
		} else {
			lineEnd += p + 1
		}
		line := s[p:lineEnd]
		trimmed := strings.TrimLeft(line, " \t")
		indent := len(line) - len(trimmed)
		if inFence == 0 && !inComment && (indent >= 4 || strings.HasPrefix(line, "\t")) && strings.TrimSpace(line) != "" {
			maskBytes(b, p, lineEnd)
			p = lineEnd
			continue
		}
		if !inComment && indent <= 3 && len(trimmed) > 0 && (trimmed[0] == '`' || trimmed[0] == '~') {
			ch := trimmed[0]
			n := 0
			for n < len(trimmed) && trimmed[n] == ch {
				n++
			}
			if n >= 3 {
				if inFence == 0 {
					inFence = ch
					fenceLen = n
				} else if ch == inFence && n >= fenceLen && strings.TrimSpace(trimmed[n:]) == "" {
					inFence = 0
				}
				maskBytes(b, p, lineEnd)
				p = lineEnd
				continue
			}
		}
		if inFence != 0 {
			maskBytes(b, p, lineEnd)
			p = lineEnd
			continue
		}
		for p < lineEnd {
			if inComment {
				q := strings.Index(s[p:], commentEnd)
				if q < 0 || p+q >= lineEnd {
					maskBytes(b, p, lineEnd)
					p = lineEnd
					break
				}
				q += p + len(commentEnd)
				maskBytes(b, p, q)
				p = q
				inComment = false
				commentEnd = ""
				continue
			}
			if strings.HasPrefix(s[p:], "<!--") {
				inComment = true
				commentEnd = "-->"
				continue
			}
			if strings.HasPrefix(s[p:], "%%") {
				inComment = true
				commentEnd = "%%"
				maskBytes(b, p, p+2)
				p += 2
				continue
			}
			if s[p] == '`' {
				n := 1
				for p+n < len(s) && s[p+n] == '`' {
					n++
				}
				close := strings.Repeat("`", n)
				q := strings.Index(s[p+n:], close)
				if q >= 0 {
					q = p + n + q + n
					maskBytes(b, p, q)
					p = q
					continue
				}
			}
			p++
		}
	}
	return string(b)
}
func maskBytes(b []byte, a, z int) {
	for n := a; n < z; n++ {
		if b[n] != '\n' && b[n] != '\r' {
			b[n] = ' '
		}
	}
}

func parseLinks(path, source, visible string, base int) []Link {
	var out []Link
	for p := 0; p+1 < len(visible); {
		i := strings.Index(visible[p:], "[[")
		if i < 0 {
			break
		}
		i += p
		j := strings.Index(visible[i+2:], "]]")
		if j < 0 {
			break
		}
		j += i + 2
		inside := source[i+2 : j]
		if strings.ContainsAny(inside, "\r\n") || strings.TrimSpace(inside) == "" {
			p = i + 2
			continue
		}
		k := strings.IndexByte(inside, '|')
		target, label := inside, ""
		if k >= 0 {
			target = strings.TrimSpace(inside[:k])
			label = strings.TrimSpace(inside[k+1:])
		}
		if target != "" {
			out = append(out, Link{Source: path, Target: target, Label: label, Kind: "wiki", Start: base + i, End: base + j + 2})
		}
		p = j + 2
	}
	// Inline Markdown links: [label](destination), with balanced destination
	// parentheses and optional angle-bracket destinations. Reference links and
	// images are omitted deliberately in this first index implementation.
	for p := 0; p < len(visible); p++ {
		if visible[p] != '[' || (p > 0 && visible[p-1] == '!') {
			continue
		}
		close := strings.IndexByte(visible[p+1:], ']')
		if close < 0 {
			continue
		}
		close += p + 1
		if close+1 >= len(visible) || visible[close+1] != '(' {
			continue
		}
		q := close + 2
		depth := 0
		for q < len(visible) {
			if visible[q] == '(' {
				depth++
			} else if visible[q] == ')' {
				if depth == 0 {
					break
				}
				depth--
			}
			q++
		}
		if q >= len(visible) {
			continue
		}
		dest := strings.TrimSpace(source[close+2 : q])
		if strings.HasPrefix(dest, "<") && strings.HasSuffix(dest, ">") {
			dest = dest[1 : len(dest)-1]
		}
		if cut := strings.IndexAny(dest, " \t\n"); cut >= 0 {
			dest = dest[:cut]
		}
		if dest != "" {
			out = append(out, Link{Source: path, Target: dest, Label: strings.TrimSpace(source[p+1 : close]), Kind: "markdown", Start: base + p, End: base + q + 1})
		}
		p = q
	}
	// The two scans above naturally visit wiki links before inline links; restore
	// source order for consumers and backlink rendering.
	for a := 1; a < len(out); a++ {
		for b := a; b > 0 && out[b].Start < out[b-1].Start; b-- {
			out[b], out[b-1] = out[b-1], out[b]
		}
	}
	return out
}

func frontmatterEnd(s string) int {
	p := strings.Index(s, "\n")
	if p < 0 {
		return -1
	}
	for at := p + 1; at < len(s); {
		end := strings.IndexByte(s[at:], '\n')
		if end < 0 {
			end = len(s)
		} else {
			end += at
		}
		line := strings.TrimSpace(strings.TrimSuffix(s[at:end], "\r"))
		if line == "---" || line == "..." {
			if end < len(s) {
				return end + 1
			}
			return end
		}
		if end == len(s) {
			break
		}
		at = end + 1
	}
	return -1
}
func parseFrontmatter(s string) (title string, aliases, tags []string) {
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	key := ""
	for _, line := range lines[1:] {
		t := strings.TrimSpace(line)
		if t == "---" || t == "..." {
			break
		}
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasPrefix(t, "-") && (key == "aliases" || key == "tags") {
			v := yamlScalar(strings.TrimSpace(strings.TrimPrefix(t, "-")))
			if v != "" {
				if key == "aliases" {
					aliases = append(aliases, v)
				} else {
					tags = append(tags, v)
				}
			}
			continue
		}
		k, v, ok := strings.Cut(t, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		switch key {
		case "title":
			title = yamlScalar(v)
		case "aliases":
			aliases = append(aliases, yamlList(v)...)
		case "tags":
			tags = append(tags, yamlList(v)...)
		default:
			key = ""
		}
	}
	return
}
func yamlList(s string) []string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		s = s[1 : len(s)-1]
	}
	var out []string
	for _, v := range strings.Split(s, ",") {
		if x := yamlScalar(v); x != "" {
			out = append(out, x)
		}
	}
	return out
}
func yamlScalar(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		if s[0] == '"' {
			s = strings.ReplaceAll(s[1:len(s)-1], `\"`, `"`)
		} else {
			s = s[1 : len(s)-1]
		}
	}
	return strings.TrimSpace(s)
}
func stripClosingHashes(s string) string {
	s = strings.TrimSpace(s)
	for strings.HasSuffix(s, "#") {
		s = strings.TrimSpace(strings.TrimSuffix(s, "#"))
	}
	return s
}
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			b.WriteRune(r)
			dash = false
		} else if unicode.IsSpace(r) {
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
func snippet(source string, start, end int) string {
	lo := start - 48
	if lo < 0 {
		lo = 0
	}
	hi := end + 72
	if hi > len(source) {
		hi = len(source)
	}
	for lo > 0 && lo < len(source) && (source[lo]&0xC0) == 0x80 {
		lo--
	}
	for hi < len(source) && (source[hi]&0xC0) == 0x80 {
		hi++
	}
	s := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(source[lo:hi], "\n", " "), "\r", " "))
	if lo > 0 {
		s = "…" + s
	}
	if hi < len(source) {
		s += "…"
	}
	return s
}
