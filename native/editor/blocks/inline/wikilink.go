package inline

import "strings"

// WikiLink is a parsed source projection of a Muse wiki link or embed.
// Offsets are half-open UTF-8 byte ranges into the original source.
type WikiLink struct {
	From, To           int
	LabelFrom, LabelTo int
	Target             string
	Embed              bool
}

// ParseWikiLinks finds Muse wiki-link syntax in a source range. This is a
// Muse inline extension, not a second Markdown parser; callers should pass
// the smallest parser-owned text range that can contain these constructs.
func ParseWikiLinks(source string, from, to int) []WikiLink {
	if from < 0 || to < from || to > len(source) {
		return nil
	}
	var out []WikiLink
	for p := from; p+1 < to; {
		i := strings.Index(source[p:to], "[[")
		if i < 0 {
			return out
		}
		i += p
		if i > from && source[i-1] == '\\' {
			p = i + 2
			continue
		}
		j := strings.Index(source[i+2:to], "]]")
		if j < 0 {
			return out
		}
		j += i + 2
		inner := source[i+2 : j]
		if strings.ContainsAny(inner, "\n\r") || strings.TrimSpace(inner) == "" {
			p = i + 2
			continue
		}
		start, embed := i, i > from && source[i-1] == '!'
		if embed {
			start--
		}
		labelFrom, labelTo, target := i+2, j, inner
		if k := strings.IndexByte(inner, '|'); k >= 0 {
			target = inner[:k]
			labelFrom = i + 3 + k
		}
		out = append(out, WikiLink{
			From: start, To: j + 2, LabelFrom: labelFrom, LabelTo: labelTo,
			Target: strings.TrimSpace(strings.ReplaceAll(target, `\|`, "|")), Embed: embed,
		})
		p = j + 2
	}
	return out
}

// WikiLinkAt resolves the target of a wiki link whose source range contains
// offset. It returns false for an empty target.
func WikiLinkAt(source string, from, to, offset int) (string, bool) {
	for _, link := range ParseWikiLinks(source, from, to) {
		if link.From <= offset && offset <= link.To {
			target := strings.TrimSpace(link.Target)
			return target, target != ""
		}
	}
	return "", false
}
