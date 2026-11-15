package markdown

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// codePalette is Typora/GitHub-like syntax coloring, adapted to the dark
// theme. It is deliberately small: comments, strings, numbers, keywords,
// functions, tags and attributes are enough to make code read as code.
type codePalette struct {
	keyword  ui.Color
	str      ui.Color
	comment  ui.Color
	number   ui.Color
	function ui.Color
	tag      ui.Color
	attr     ui.Color
}

func paletteFor(dark bool) codePalette {
	if dark {
		return codePalette{
			keyword:  ui.Hex("#ff7b72"),
			str:      ui.Hex("#a5d6ff"),
			comment:  ui.Hex("#8b949e"),
			number:   ui.Hex("#79c0ff"),
			function: ui.Hex("#d2a8ff"),
			tag:      ui.Hex("#7ee787"),
			attr:     ui.Hex("#d2a8ff"),
		}
	}
	return codePalette{
		keyword:  ui.Hex("#d73a49"),
		str:      ui.Hex("#032f62"),
		comment:  ui.Hex("#6a737d"),
		number:   ui.Hex("#005cc5"),
		function: ui.Hex("#6f42c1"),
		tag:      ui.Hex("#22863a"),
		attr:     ui.Hex("#6f42c1"),
	}
}

// highlightLine splits one line into styled spans. Concatenating the span
// texts always reproduces the line, so the renderer can never lose code.
func HighlightLine(line, lang string, dark bool) []ui.Span {
	lang = strings.ToLower(strings.TrimSpace(lang))
	p := paletteFor(dark)
	switch lang {
	case "html", "xml", "svg", "vue", "jsx", "tsx":
		return highlightHTML(line, p)
	case "css", "scss", "less":
		return highlightCSS(line, p, lang)
	}
	return highlightGeneric(line, lang, p)
}

func highlightGeneric(line, lang string, p codePalette) []ui.Span {
	kw := keywords(lang)
	commentsHash := lang == "python" || lang == "py" || lang == "sh" || lang == "bash" ||
		lang == "zsh" || lang == "shell" || lang == "yaml" || lang == "yml" || lang == "toml"
	commentsDash := lang == "sql"
	var spans []ui.Span
	var plain strings.Builder
	flush := func() {
		if plain.Len() > 0 {
			spans = append(spans, ui.Span{Text: plain.String()})
			plain.Reset()
		}
	}
	for i := 0; i < len(line); {
		rest := line[i:]
		switch {
		case strings.HasPrefix(rest, "/*"):
			flush()
			end := strings.Index(rest[2:], "*/")
			if end < 0 {
				spans = append(spans, ui.Span{Text: rest, Color: p.comment})
				i = len(line)
				continue
			}
			end += 4
			spans = append(spans, ui.Span{Text: rest[:end], Color: p.comment})
			i += end
		case strings.HasPrefix(rest, "//"):
			flush()
			spans = append(spans, ui.Span{Text: rest, Color: p.comment})
			i = len(line)
		case commentsDash && strings.HasPrefix(rest, "--"):
			flush()
			spans = append(spans, ui.Span{Text: rest, Color: p.comment})
			i = len(line)
		case commentsHash && line[i] == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t'):
			flush()
			spans = append(spans, ui.Span{Text: rest, Color: p.comment})
			i = len(line)
		case line[i] == '"' || line[i] == '\'' || line[i] == '`':
			flush()
			j := scanString(line, i)
			spans = append(spans, ui.Span{Text: line[i:j], Color: p.str})
			i = j
		case line[i] >= '0' && line[i] <= '9':
			flush()
			j := i + 1
			for j < len(line) && (isIdentPart(line[j]) || line[j] == '.') {
				j++
			}
			spans = append(spans, ui.Span{Text: line[i:j], Color: p.number})
			i = j
		case isIdentStart(line[i]):
			flush()
			j := i + 1
			for j < len(line) && isIdentPart(line[j]) {
				j++
			}
			word := line[i:j]
			switch {
			case kw[word]:
				spans = append(spans, ui.Span{Text: word, Color: p.keyword})
			case j < len(line) && line[j] == '(':
				spans = append(spans, ui.Span{Text: word, Color: p.function})
			default:
				spans = append(spans, ui.Span{Text: word})
			}
			i = j
		default:
			plain.WriteByte(line[i])
			i++
		}
	}
	flush()
	if len(spans) == 0 {
		spans = append(spans, ui.Span{Text: " "})
	}
	return spans
}

func highlightHTML(line string, p codePalette) []ui.Span {
	var spans []ui.Span
	for i := 0; i < len(line); {
		if line[i] != '<' {
			j := strings.IndexByte(line[i:], '<')
			if j < 0 {
				spans = append(spans, ui.Span{Text: line[i:]})
				break
			}
			spans = append(spans, ui.Span{Text: line[i : i+j]})
			i += j
			continue
		}
		end := strings.IndexByte(line[i:], '>')
		if end < 0 {
			spans = append(spans, ui.Span{Text: line[i:], Color: p.tag})
			break
		}
		end += i
		seg := line[i : end+1]
		// Opening angle and optional slash.
		j := 1
		if j < len(seg) && seg[j] == '/' {
			j++
		}
		spans = append(spans, ui.Span{Text: seg[:j]})
		k := j
		for k < len(seg) && isTagChar(seg[k]) {
			k++
		}
		if k > j {
			spans = append(spans, ui.Span{Text: seg[j:k], Color: p.tag})
		}
		for k < len(seg) {
			if seg[k] == '"' || seg[k] == '\'' {
				e := scanString(seg, k)
				spans = append(spans, ui.Span{Text: seg[k:e], Color: p.str})
				k = e
				continue
			}
			if isIdentStart(seg[k]) {
				e := k + 1
				for e < len(seg) && isIdentPart(seg[e]) {
					e++
				}
				spans = append(spans, ui.Span{Text: seg[k:e], Color: p.attr})
				k = e
				continue
			}
			spans = append(spans, ui.Span{Text: string(seg[k])})
			k++
		}
		i = end + 1
	}
	if len(spans) == 0 {
		spans = append(spans, ui.Span{Text: " "})
	}
	return spans
}

func highlightCSS(line string, p codePalette, lang string) []ui.Span {
	// CSS is close enough to the generic lexer once strings, numbers and
	// comments are colored; property names before a colon get the attribute
	// color.
	spans := highlightGeneric(line, lang, p)
	for idx := range spans {
		s := &spans[idx]
		if s.Color != (ui.Color{}) {
			continue
		}
		if colon := strings.IndexByte(s.Text, ':'); colon > 0 {
			head, tail := s.Text[:colon], s.Text[colon:]
			if strings.TrimSpace(head) != "" && !strings.ContainsAny(head, "{};") {
				spans[idx].Text = head
				spans[idx].Color = p.attr
				spans = append(spans[:idx+1], append([]ui.Span{{Text: tail}}, spans[idx+1:]...)...)
				return spans
			}
		}
	}
	return spans
}

func scanString(s string, start int) int {
	quote := s[start]
	i := start + 1
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			i += 2
			continue
		}
		if s[i] == quote {
			return i + 1
		}
		i++
	}
	return len(s)
}

func isIdentStart(b byte) bool {
	return b == '_' || b == '$' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= 0x80
}

func isIdentPart(b byte) bool {
	return isIdentStart(b) || b >= '0' && b <= '9'
}

func isTagChar(b byte) bool {
	return isIdentPart(b) || b == '-' || b == ':'
}

func keywords(lang string) map[string]bool {
	set := map[string]bool{}
	add := func(words string) {
		for _, w := range strings.Fields(words) {
			set[w] = true
		}
	}
	add("true false null nil")
	switch lang {
	case "go", "golang":
		add("break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var")
	case "js", "javascript", "jsx", "ts", "typescript", "tsx":
		add("break case catch class const continue debugger default delete do else export extends finally for function if import in instanceof let new return super switch this throw try typeof var void while with yield async await static get set interface type enum implements public private protected readonly")
	case "python", "py":
		add("and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield True False None")
	case "rust", "rs":
		add("as break const continue crate else enum extern false fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait true type unsafe use where while async await dyn")
	case "sql":
		add("select from where insert into values update set delete create table alter drop join left right inner outer on group by order having limit offset as and or not null")
	case "sh", "bash", "zsh", "shell":
		add("if then else elif fi for while do done case esac function in echo export local return exit")
	case "yaml", "yml", "toml":
		add("true false null yes no on off")
	case "json":
		add("true false null")
	}
	return set
}
