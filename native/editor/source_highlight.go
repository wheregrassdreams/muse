package editor

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// sourceToken is a byte range in the unchanged source, styled with the same
// token palette as fenced code. Gaps remain ordinary text.
type sourceToken struct {
	start, end int
	kind       editor.TokenKind
}

func (e *Editor) sourceTokenSpan(base ui.Span, kind editor.TokenKind) ui.Span {
	sp := base
	if c := e.lk.code.TokenColor(kind); c.A != 0 {
		sp.Color = c
	}
	ts := e.lk.code.Tokens[kind]
	sp.Italic = ts.Italic
	if ts.Bold {
		sp.Weight = 700
	}
	return sp
}

// renderSourceTokens keeps each piece byte-for-byte tied to the same source
// bytes. The caller runs this only while building a source-visible leaf view.
func renderSourceTokens(b *builder, offset int, text string, tokens []sourceToken, base ui.Span) {
	at := 0
	for _, tok := range tokens {
		start, end := max(0, tok.start), min(len(text), tok.end)
		if end <= start || start < at {
			continue
		}
		b.codeRun(offset+at, offset+start, base)
		b.codeRun(offset+start, offset+end, b.e.sourceTokenSpan(base, tok.kind))
		at = end
	}
	b.codeRun(offset+at, offset+len(text), base)
}

func mermaidSourceTokens(line string) []sourceToken {
	var out []sourceToken
	add := func(start, end int, kind editor.TokenKind) {
		if end > start {
			out = append(out, sourceToken{start: start, end: end, kind: kind})
		}
	}
	for i := 0; i < len(line); {
		if strings.HasPrefix(line[i:], "%%") {
			add(i, len(line), editor.TokenComment)
			break
		}
		if line[i] == ' ' || line[i] == '\t' || line[i] == '\r' {
			i++
			continue
		}
		if line[i] == '\'' || line[i] == '"' {
			q := line[i]
			j := i + 1
			for j < len(line) && line[j] != q {
				if line[j] == '\\' && j+1 < len(line) {
					j++
				}
				j++
			}
			if j < len(line) {
				j++
			}
			add(i, j, editor.TokenString)
			i = j
			continue
		}
		if close, ok := mermaidShapeClose(line[i]); ok {
			if end := mermaidMatchingClose(line, i, line[i], close); end > i {
				add(i, i+1, editor.TokenPunctuation)
				add(i+1, end, editor.TokenString)
				add(end, end+1, editor.TokenPunctuation)
				i = end + 1
				continue
			}
		}
		if line[i] == '|' {
			if j := strings.IndexByte(line[i+1:], '|'); j >= 0 {
				j += i + 1
				add(i, i+1, editor.TokenPunctuation)
				add(i+1, j, editor.TokenString)
				add(j, j+1, editor.TokenPunctuation)
				i = j + 1
				continue
			}
		}
		if opEnd := mermaidOperatorEnd(line, i); opEnd > i {
			add(i, opEnd, editor.TokenOperator)
			i = opEnd
			continue
		}
		r, n := utf8.DecodeRuneInString(line[i:])
		if strings.ContainsRune("[](){}|&:;,+", r) {
			add(i, i+n, editor.TokenPunctuation)
			i += n
			continue
		}
		if unicode.IsDigit(r) {
			j := scanMermaidWord(line, i)
			add(i, j, editor.TokenNumber)
			i = j
			continue
		}
		if unicode.IsLetter(r) || r == '_' {
			j := scanMermaidWord(line, i)
			word := line[i:j]
			kind := editor.TokenVariable
			if mermaidKeywords[strings.ToLower(word)] {
				kind = editor.TokenKeyword
			} else if mermaidDirective(word) {
				kind = editor.TokenType
			}
			add(i, j, kind)
			i = j
			continue
		}
		i += n
	}
	return out
}

func mermaidShapeClose(open byte) (byte, bool) {
	switch open {
	case '[':
		return ']', true
	case '(':
		return ')', true
	case '{':
		return '}', true
	default:
		return 0, false
	}
}

func mermaidMatchingClose(s string, start int, open, close byte) int {
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func mermaidOperatorEnd(line string, at int) int {
	for _, op := range []string{
		"<<->>", "-->>", "<-->", "o--o", "x--x", "o-->", "x-->", "--o", "--x",
		"-.->", "==>", "-->", "---", "-.-", "<<--", "<->", "<--", "->>", "<-", "->", "-.", "--", "==", "-x", "x-",
	} {
		if strings.HasPrefix(line[at:], op) {
			return at + len(op)
		}
	}
	return at
}

func scanMermaidWord(s string, at int) int {
	for at < len(s) {
		r, n := utf8.DecodeRuneInString(s[at:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
			break
		}
		at += n
	}
	return at
}

var mermaidKeywords = map[string]bool{
	"graph": true, "flowchart": true, "sequencediagram": true, "classdiagram": true,
	"statediagram": true, "statediagram-v2": true, "erdiagram": true, "journey": true,
	"gantt": true, "pie": true, "gitgraph": true, "requirementdiagram": true,
	"subgraph": true, "end": true, "direction": true, "classdef": true, "class": true,
	"style": true, "linkstyle": true, "click": true, "participant": true, "actor": true,
	"autonumber": true, "activate": true, "deactivate": true, "loop": true, "alt": true,
	"else": true, "opt": true, "par": true, "and": true, "rect": true, "critical": true,
	"break": true, "section": true, "title": true, "note": true, "over": true,
}

func mermaidDirective(word string) bool {
	return strings.EqualFold(word, "TD") || strings.EqualFold(word, "TB") ||
		strings.EqualFold(word, "BT") || strings.EqualFold(word, "LR") || strings.EqualFold(word, "RL")
}

func latexSourceTokens(src string) []sourceToken {
	var out []sourceToken
	add := func(start, end int, kind editor.TokenKind) {
		if end > start {
			out = append(out, sourceToken{start: start, end: end, kind: kind})
		}
	}
	for i := 0; i < len(src); {
		if src[i] == '%' {
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				j = len(src)
			} else {
				j += i
			}
			add(i, j, editor.TokenComment)
			i = j
			continue
		}
		if src[i] == '\\' {
			j := i + 1
			for j < len(src) {
				r, n := utf8.DecodeRuneInString(src[j:])
				if !unicode.IsLetter(r) {
					break
				}
				j += n
			}
			if j == i+1 && j < len(src) {
				_, n := utf8.DecodeRuneInString(src[j:])
				j += n
				add(i, j, editor.TokenEscape)
			} else {
				kind := editor.TokenFunction
				if src[i+1:j] == "begin" || src[i+1:j] == "end" {
					kind = editor.TokenKeyword
				}
				add(i, j, kind)
			}
			i = j
			continue
		}
		r, n := utf8.DecodeRuneInString(src[i:])
		if unicode.IsDigit(r) {
			j := i + n
			for j < len(src) {
				r, n = utf8.DecodeRuneInString(src[j:])
				if !unicode.IsDigit(r) && r != '.' {
					break
				}
				j += n
			}
			add(i, j, editor.TokenNumber)
			i = j
			continue
		}
		if strings.ContainsRune("{}[]()_^=+-*/<>|&", r) {
			kind := editor.TokenOperator
			if strings.ContainsRune("{}[]()", r) {
				kind = editor.TokenPunctuation
			}
			add(i, i+n, kind)
		}
		i += n
	}
	return out
}
