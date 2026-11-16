package source

import "strings"

func CommentsFor(src string) []Comment {
	var out []Comment
	open, inFence := -1, false
	for lineStart := 0; lineStart <= len(src); {
		lineEnd := strings.IndexByte(src[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(src)
		} else {
			lineEnd += lineStart
		}
		line := src[lineStart:lineEnd]
		trim := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~") {
			inFence = !inFence
		} else if !inFence {
			for p := lineStart; p+1 < lineEnd; {
				next := strings.IndexByte(src[p:lineEnd], '%')
				if next < 0 {
					break
				}
				p += next
				if p+1 >= lineEnd || src[p+1] != '%' || escapedAt(src, p) {
					p++
					continue
				}
				if open < 0 {
					open = p
				} else {
					out = append(out, Comment{Start: open, End: p + 2})
					open = -1
				}
				p += 2
			}
		}
		if lineEnd >= len(src) {
			break
		}
		lineStart = lineEnd + 1
	}
	return out
}

func FindComment(comments []Comment, offset int) (Comment, bool) {
	for _, comment := range comments {
		if comment.Start <= offset && offset < comment.End {
			return comment, true
		}
	}
	return Comment{}, false
}

func maskLines(src string, from, to int, comments []Comment) string {
	to = min(to, len(src))
	if from < 0 || to <= from {
		return ""
	}
	b := []byte(src[from:to])
	for _, comment := range comments {
		if comment.End <= from || comment.Start >= to {
			continue
		}
		cf, ct := max(comment.Start-from, 0), min(comment.End-from, len(b))
		for i := cf; i < ct; i++ {
			if b[i] != '\n' && b[i] != '\r' {
				b[i] = ' '
			}
		}
		for p := max(comment.Start, from); p < min(comment.End, to); p = lineEndAt(src, p) + 1 {
			start := max(p, lineStartAt(src, p))
			end := min(comment.End, lineEndAt(src, p))
			sf, ef := start-from, end-from
			if sf < 0 || ef > len(b) || ef < sf || !opensContent(b, sf) || !hasTextOnLine(b, ef) {
				continue
			}
			if sf+2 <= ef {
				b[sf], b[sf+1] = 0xC2, 0xA0
			}
		}
	}
	return string(b)
}

func blankComments(src string, comments []Comment) string {
	if len(comments) == 0 {
		return src
	}
	return maskLines(src, 0, len(src), comments)
}

func opensContent(b []byte, at int) bool {
	for i := at - 1; i >= 0; i-- {
		switch ch := b[i]; {
		case ch == '\n':
			return true
		case ch == ' ' || ch == '\t':
		case ch == '>' || ch == '#' || ch == '-' || ch == '*' || ch == '+' || ch == '.' || ch == ')' || ch == '[' || ch == ']' || ch == '^' || ch == ':' || ch == '!' || ch == '|' || ch == '~':
		case ch >= '0' && ch <= '9':
		default:
			return false
		}
	}
	return true
}

func hasTextOnLine(b []byte, at int) bool {
	for i := at; i < len(b) && b[i] != '\n'; i++ {
		if b[i] != ' ' && b[i] != '\t' && b[i] != '\r' {
			return true
		}
	}
	return false
}

func touchesComment(comments []Comment, from, to int) bool {
	for _, c := range comments {
		if from <= c.End && to >= c.Start {
			return true
		}
	}
	return false
}

func commentsStay(oldSource, newSource string, from, to, newLength int) bool {
	marker := func(b byte) bool { return b == '%' || b == '\\' || b == '`' || b == '~' }
	for i := from; i < to && i < len(oldSource); i++ {
		if marker(oldSource[i]) {
			return false
		}
	}
	for i := from; i < from+newLength && i < len(newSource); i++ {
		if marker(newSource[i]) {
			return false
		}
	}
	if from > 0 && marker(oldSource[from-1]) {
		return false
	}
	if to < len(oldSource) && marker(oldSource[to]) {
		return false
	}
	return true
}

func escapedAt(s string, i int) bool {
	n := 0
	for i-n-1 >= 0 && s[i-n-1] == '\\' {
		n++
	}
	return n%2 == 1
}
