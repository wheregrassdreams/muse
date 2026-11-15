package syntax

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// SourceTokenKind is a backend-neutral syntax class used when a source-backed
// block is temporarily revealed instead of rendered as a visual object.
type SourceTokenKind uint8

const (
	SourceTokenKeyword SourceTokenKind = iota + 1
	SourceTokenType
	SourceTokenFunction
	SourceTokenVariable
	SourceTokenNumber
	SourceTokenString
	SourceTokenEscape
	SourceTokenComment
	SourceTokenOperator
	SourceTokenPunctuation
)

// SourceToken is a byte range in the original UTF-8 source. Ranges never
// split a valid rune; gaps between tokens remain ordinary source text.
type SourceToken struct {
	Start, End int
	Kind       SourceTokenKind
}

// HighlightMermaidLine returns lightweight source styling ranges for one
// Mermaid line. It does not parse or layout the diagram.
func HighlightMermaidLine(line string) []SourceToken {
	var out []SourceToken
	add := func(start, end int, kind SourceTokenKind) {
		if end > start {
			out = append(out, SourceToken{Start: start, End: end, Kind: kind})
		}
	}
	for i := 0; i < len(line); {
		if strings.HasPrefix(line[i:], "%%") {
			add(i, len(line), SourceTokenComment)
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
			add(i, j, SourceTokenString)
			i = j
			continue
		}
		if close, ok := mermaidShapeClose(line[i]); ok {
			if end := mermaidMatchingClose(line, i, line[i], close); end > i {
				add(i, i+1, SourceTokenPunctuation)
				add(i+1, end, SourceTokenString)
				add(end, end+1, SourceTokenPunctuation)
				i = end + 1
				continue
			}
		}
		if line[i] == '|' {
			if j := strings.IndexByte(line[i+1:], '|'); j >= 0 {
				j += i + 1
				add(i, i+1, SourceTokenPunctuation)
				add(i+1, j, SourceTokenString)
				add(j, j+1, SourceTokenPunctuation)
				i = j + 1
				continue
			}
		}
		if opEnd := mermaidOperatorEnd(line, i); opEnd > i {
			add(i, opEnd, SourceTokenOperator)
			i = opEnd
			continue
		}
		r, n := utf8.DecodeRuneInString(line[i:])
		if strings.ContainsRune("[](){}|&:;,+", r) {
			add(i, i+n, SourceTokenPunctuation)
			i += n
			continue
		}
		if unicode.IsDigit(r) {
			j := scanMermaidWord(line, i)
			add(i, j, SourceTokenNumber)
			i = j
			continue
		}
		if unicode.IsLetter(r) || r == '_' {
			j := scanMermaidWord(line, i)
			word := line[i:j]
			kind := SourceTokenVariable
			if mermaidKeywords[strings.ToLower(word)] {
				kind = SourceTokenKeyword
			} else if mermaidDirective(word) {
				kind = SourceTokenType
			}
			add(i, j, kind)
			i = j
			continue
		}
		i += n
	}
	return out
}

// HighlightTeXSource returns source styling ranges for a TeX expression.
func HighlightTeXSource(src string) []SourceToken {
	var out []SourceToken
	add := func(start, end int, kind SourceTokenKind) {
		if end > start {
			out = append(out, SourceToken{Start: start, End: end, Kind: kind})
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
			add(i, j, SourceTokenComment)
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
				add(i, j, SourceTokenEscape)
			} else {
				kind := SourceTokenFunction
				if src[i+1:j] == "begin" || src[i+1:j] == "end" {
					kind = SourceTokenKeyword
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
			add(i, j, SourceTokenNumber)
			i = j
			continue
		}
		if strings.ContainsRune("{}[]()_^=+-*/<>|&", r) {
			kind := SourceTokenOperator
			if strings.ContainsRune("{}[]()", r) {
				kind = SourceTokenPunctuation
			}
			add(i, i+n, kind)
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

func mermaidDirective(word string) bool {
	return strings.EqualFold(word, "TD") || strings.EqualFold(word, "TB") ||
		strings.EqualFold(word, "BT") || strings.EqualFold(word, "LR") || strings.EqualFold(word, "RL")
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
