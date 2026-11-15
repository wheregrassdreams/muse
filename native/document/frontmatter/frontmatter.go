// Package frontmatter projects YAML properties without rewriting unrelated source.
package frontmatter

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"muse/locales"
	"muse/notedate"

	"gopkg.in/yaml.v3"
)

type Field struct {
	Key, Value, Kind string
	Editable         bool
}
type Document struct {
	Fields     []Field
	Start, End int
	Err        error
	root       *yaml.Node
	body       string
	offset     int
}

func Parse(src string) Document {
	d := Document{}
	if !strings.HasPrefix(src, "---\n") && !strings.HasPrefix(src, "---\r\n") {
		return d
	}
	d.offset = strings.IndexByte(src, '\n') + 1
	off := d.offset
	for off < len(src) {
		end := lineEnd(src, off)
		next := end
		if next < len(src) {
			next++
		}
		if v := strings.TrimSpace(src[off:end]); v == "---" || v == "..." {
			d.End = next
			break
		}
		off = next
	}
	if d.End == 0 {
		d.Err = fmt.Errorf("%s", locales.FrontmatterDelimiterMissing())
		return d
	}
	d.body = src[d.offset:off]
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(d.body), &n); err != nil {
		d.Err = fmt.Errorf("%s", locales.PropertyYAMLParseError(err.Error()))
		return d
	}
	if len(n.Content) == 0 {
		n.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	d.root = n.Content[0]
	if d.root.Kind != yaml.MappingNode {
		d.Err = fmt.Errorf("%s", locales.FrontmatterMustBeMap())
		return d
	}
	seen := map[string]bool{}
	for i := 0; i < len(d.root.Content); i += 2 {
		k, v := d.root.Content[i], d.root.Content[i+1]
		if k.Kind != yaml.ScalarNode || seen[k.Value] {
			d.Err = fmt.Errorf("%s", locales.FrontmatterInvalidKey(k.Value))
			return d
		}
		seen[k.Value] = true
		f := Field{Key: k.Value, Value: v.Value, Kind: "text", Editable: v.Kind == yaml.ScalarNode && v.Style&(yaml.LiteralStyle|yaml.FoldedStyle) == 0}
		switch v.Tag {
		case "!!bool":
			f.Kind = "bool"
		case "!!int", "!!float":
			f.Kind = "number"
		case "!!timestamp":
			f.Kind = "date"
		}
		if v.Kind == yaml.SequenceNode {
			f.Kind = "list"
			f.Editable = k.LineComment == "" && v.HeadComment == ""
			var a []string
			for _, item := range v.Content {
				if item.Kind != yaml.ScalarNode || item.HeadComment != "" || item.LineComment != "" {
					f.Editable = false
				}
				a = append(a, item.Value)
			}
			f.Value = strings.Join(a, ", ")
		}
		if v.Kind == yaml.MappingNode || v.Kind == yaml.AliasNode {
			f.Kind = "structure"
			f.Value = ""
			f.Editable = false
		}
		if v.Kind == yaml.SequenceNode && v.Style&yaml.FlowStyle != 0 {
			endLine := lineEnd(d.body, lineOffset(d.body, v.Line, v.Column))
			if !strings.Contains(d.body[lineOffset(d.body, v.Line, v.Column):endLine], "]") {
				f.Editable = false
			}
		}
		if v.Kind == yaml.ScalarNode {
			nextLine := len(strings.Split(d.body, "\n"))
			if i+2 < len(d.root.Content) {
				nextLine = d.root.Content[i+2].Line
			}
			lines := strings.Split(d.body, "\n")
			for j := v.Line; j < nextLine-1 && j < len(lines); j++ {
				t := strings.TrimSpace(lines[j])
				if t != "" && !strings.HasPrefix(t, "#") {
					f.Editable = false
				}
			}
		}
		if v.Anchor != "" || k.Tag != "!!str" {
			f.Editable = false
		}
		d.Fields = append(d.Fields, f)
	}
	return d
}
func Title(src, path string) string {
	// A note's display title is derived from its first body line, not from
	// legacy YAML `title` properties. Only a leading ATX H1 is canonical.
	doc := Parse(src)
	body := src
	if doc.End > 0 && doc.Err == nil {
		body = src[doc.End:]
	}
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if title, ok := leadingATXTitle(line); ok {
			return title
		}
		break
	}
	if path == "" {
		return locales.FrontmatterUntitled()
	}
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func leadingATXTitle(line string) (string, bool) {
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent > 3 {
		return "", false
	}
	text := line[indent:]
	if !strings.HasPrefix(text, "#") {
		return "", false
	}
	marks := 0
	for marks < len(text) && text[marks] == '#' {
		marks++
	}
	if marks != 1 || marks == len(text) || (text[marks] != ' ' && text[marks] != '\t') {
		return "", false
	}
	title := strings.TrimSpace(text[marks:])
	closing := len(title)
	for closing > 0 && title[closing-1] == '#' {
		closing--
	}
	if closing < len(title) && closing > 0 && (title[closing-1] == ' ' || title[closing-1] == '\t') {
		title = strings.TrimSpace(title[:closing])
	}
	if title == "" {
		return "", false
	}
	return title, true
}

// EnsureCore adds missing core keys without dropping or rewriting existing
// frontmatter. Empty user lists are represented as [] and unavailable times
// as empty strings; type defaults to note. Existing values are preserved.
func EnsureCore(src string) (string, error) {
	d := Parse(src)
	if d.Err != nil {
		return src, d.Err
	}
	defaults := []struct{ key, value string }{
		{"type", `"note"`}, {"tags", `[]`}, {"aliases", `[]`},
		{"created_at", `""`}, {"updated_at", `""`},
	}
	missing := make([]string, 0, len(defaults))
	for _, item := range defaults {
		found := false
		for _, field := range d.Fields {
			if field.Key == item.key {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, item.key+": "+item.value)
		}
	}
	if len(missing) == 0 {
		return src, nil
	}
	newline := "\n"
	if strings.HasPrefix(src, "---\r\n") {
		newline = "\r\n"
	}
	if d.End == 0 {
		return "---" + newline + strings.Join(missing, newline) + newline + "---" + newline + newline + src, nil
	}
	pos := d.offset + len(d.body)
	sep := newline
	if strings.HasSuffix(d.body, "\n") {
		sep = ""
	}
	return src[:pos] + sep + strings.Join(missing, newline) + newline + src[pos:], nil
}
func lineOffset(s string, line, col int) int {
	off := 0
	for i := 1; i < line; i++ {
		j := strings.IndexByte(s[off:], '\n')
		if j < 0 {
			return len(s)
		}
		off += j + 1
	}
	r := []rune(s[off:])
	for i := 1; i < col && i <= len(r); i++ {
		off += len(string(r[i-1]))
	}
	return off
}
func lineEnd(s string, off int) int {
	if j := strings.IndexByte(s[off:], '\n'); j >= 0 {
		return off + j
	}
	return len(s)
}
func encoded(value, kind string) (string, error) {
	switch kind {
	case "date":
		if _, _, err := notedate.Parse(value); err != nil {
			return "", fmt.Errorf("%s", locales.FrontmatterDateInvalid())
		}
		return value, nil
	case "list":
		var values []string
		if strings.HasPrefix(strings.TrimSpace(value), "[") {
			if err := yaml.Unmarshal([]byte(value), &values); err != nil {
				return "", err
			}
		} else {
			for _, v := range strings.Split(value, ",") {
				if v = strings.TrimSpace(v); v != "" {
					values = append(values, v)
				}
			}
		}
		var a []string
		for _, v := range values {
			a = append(a, strconv.Quote(v))
		}
		return "[" + strings.Join(a, ", ") + "]", nil
	case "bool":
		if value != "true" && value != "false" {
			return "", fmt.Errorf("%s", locales.FrontmatterBooleanInvalid())
		}
		return value, nil
	case "number":
		var n yaml.Node
		if err := yaml.Unmarshal([]byte(value), &n); err != nil {
			return "", err
		}
		if len(n.Content) == 0 || (n.Content[0].Tag != "!!int" && n.Content[0].Tag != "!!float") {
			return "", fmt.Errorf("%s", locales.FrontmatterNumberInvalid())
		}
		return value, nil
	default:
		return strconv.Quote(value), nil
	}
}
func Set(src, key, value string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return src, fmt.Errorf("%s", locales.FrontmatterKeyEmpty())
	}
	d := Parse(src)
	if d.Err != nil {
		return src, d.Err
	}
	for i, f := range d.Fields {
		if f.Key != key {
			continue
		}
		if !f.Editable {
			return src, fmt.Errorf("%s", locales.FrontmatterStructureSourceOnly())
		}
		kind := f.Kind
		if key == "title" {
			kind = "text"
		}
		text, err := encoded(value, kind)
		if err != nil {
			return src, err
		}
		v := d.root.Content[i*2+1]
		start := lineOffset(d.body, v.Line, v.Column)
		end := lineEnd(d.body, start)
		if v.Kind == yaml.SequenceNode && v.Style&yaml.FlowStyle == 0 {
			k := d.root.Content[i*2]
			start = lineOffset(d.body, k.Line, k.Column)
			colon := strings.LastIndexByte(d.body[start:lineEnd(d.body, start)], ':')
			start += colon + 1
			for start < len(d.body) && (d.body[start] == ' ' || d.body[start] == '\t') {
				start++
			}
			last := v.Content[len(v.Content)-1]
			end = lineEnd(d.body, lineOffset(d.body, last.Line, last.Column))
			text = " " + text
		} else if v.LineComment != "" {
			if p := strings.LastIndex(d.body[start:end], v.LineComment); p >= 0 {
				tail := start + p
				for tail > start && (d.body[tail-1] == ' ' || d.body[tail-1] == '\t') {
					tail--
				}
				end = tail
			}
		}
		if end > start && d.body[end-1] == '\r' {
			end--
		}
		return src[:d.offset+start] + text + src[d.offset+end:], nil
	}
	kind := "text"
	if key == "tags" || key == "aliases" {
		kind = "list"
	}
	text, err := encoded(value, kind)
	if err != nil {
		return src, err
	}
	return insert(src, key, text, d), nil
}

// SetTyped assigns a type when creating a property; existing types are retained.
func SetTyped(src, key, value, kind string) (string, error) {
	d := Parse(src)
	if d.Err != nil {
		return src, d.Err
	}
	for _, f := range d.Fields {
		if f.Key == key {
			return Set(src, key, value)
		}
	}
	if strings.TrimSpace(key) == "" {
		return src, fmt.Errorf("%s", locales.FrontmatterKeyEmpty())
	}
	text, err := encoded(value, kind)
	if err != nil {
		return src, err
	}
	return insert(src, key, text, d), nil
}
func insert(src, key, text string, d Document) string {
	newline := "\n"
	if strings.HasPrefix(src, "---\r\n") {
		newline = "\r\n"
	}
	keyBytes, _ := yaml.Marshal(key)
	entry := strings.TrimSpace(string(keyBytes)) + ": " + text + newline
	if d.End == 0 {
		return "---\n" + entry + "---\n\n" + src
	}
	return src[:d.offset] + d.body + entry + src[d.offset+len(d.body):]
}
func Delete(src, key string) (string, error) {
	d := Parse(src)
	if d.Err != nil {
		return src, d.Err
	}
	for i, f := range d.Fields {
		if f.Key != key {
			continue
		}
		k, v := d.root.Content[i*2], d.root.Content[i*2+1]
		start := lineOffset(d.body, k.Line, 1)
		end := lineEnd(d.body, lineOffset(d.body, v.Line, v.Column))
		if i+1 < len(d.Fields) {
			end = lineOffset(d.body, d.root.Content[(i+1)*2].Line, 1) - 1
		} else {
			end = len(d.body) - 1
		}
		if end < len(d.body) {
			end++
		}
		// Preserve standalone comments and spacing belonging to following fields.
		for end > start {
			segment := strings.TrimSuffix(d.body[start:end], "\n")
			pos := strings.LastIndexByte(segment, '\n') + 1
			last := strings.TrimSpace(segment[pos:])
			if last != "" && !strings.HasPrefix(last, "#") {
				break
			}
			end = start + pos
		}
		return src[:d.offset+start] + src[d.offset+end:], nil
	}
	return src, nil
}
