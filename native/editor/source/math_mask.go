package source

import "strings"

// ParserText returns source with escaped display-math brackets disambiguated
// for the Markdown parser while preserving every byte offset.
func ParserText(src string) string { return bracketMath(src) }

// bracketMath returns src with the [ of a \[ swapped for a letter where
// the parser would take it for the start of display math, as LaTeX writes
// it, but it reads as a bracket escaped, as CommonMark writes one: a \[
// with text after it at the start of a line and no \] on the line, which
// would make a block of math of the rest of the document, and a \[…\]
// holding no TeX, as \[1\] or \[注\]. Its result depends on each line
// alone, and is as long as src.
func bracketMath(src string) string {
	if !strings.Contains(src, `\[`) {
		return src
	}
	var b []byte
	for ls := 0; ls < len(src); {
		le := len(src)
		if i := strings.IndexByte(src[ls:], '\n'); i >= 0 {
			le = ls + i
		}
		line := src[ls:le]
		for i := strings.Index(line, `\[`); i >= 0; i = nextBracket(line, i) {
			if escapedAt(line, i) {
				continue
			}
			rest := line[i+2:]
			end := strings.Index(rest, `\]`)
			math := true
			switch {
			case end >= 0:
				math = texLike(rest[:end])
			case blockLead(line[:i]) && strings.TrimSpace(rest) != "":
				math = texLike(rest)
			}
			if !math {
				if b == nil {
					b = []byte(src)
				}
				b[ls+i+1] = 'x'
			}
		}
		ls = le + 1
	}
	if b == nil {
		return src
	}
	return string(b)
}

func nextBracket(line string, i int) int {
	if j := strings.Index(line[i+2:], `\[`); j >= 0 {
		return i + 2 + j
	}
	return -1
}

func blockLead(s string) bool {
	for i := 0; i < len(s); {
		switch c := s[i]; {
		case c == ' ' || c == '\t' || c == '>':
			i++
		case c == '-' || c == '*' || c == '+':
			if i+1 >= len(s) || s[i+1] != ' ' && s[i+1] != '\t' {
				return false
			}
			i += 2
		case c >= '0' && c <= '9':
			j := i
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			if j-i > 9 || j+1 >= len(s) || s[j] != '.' && s[j] != ')' || s[j+1] != ' ' && s[j+1] != '\t' {
				return false
			}
			i = j + 2
		default:
			return false
		}
	}
	return true
}

func texLike(s string) bool {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '^', '_', '{', '}', '=', '+', '<', '>', '|':
			return true
		case '\\':
			if i+1 < len(s) && (s[i+1]|0x20 >= 'a' && s[i+1]|0x20 <= 'z') {
				return true
			}
		}
	}
	return false
}
