package inline

import (
	"strings"
	"unicode/utf8"

	"muse/editor/navigation"
)

var autoPairClosers = map[rune]rune{
	'(': ')', '[': ']', '{': '}', '"': '"',
	'（': '）', '【': '】', '《': '》', '“': '”', '「': '」',
}

// WrapPairFor returns the closer used when selected text is wrapped by an
// opening delimiter. This policy differs from automatic insertion pairs:
// emphasis markers and symmetric quote delimiters are valid around a range.
func WrapPairFor(open rune, inCode bool) (rune, bool) {
	if close, ok := autoPairClosers[open]; ok {
		return close, true
	}
	switch open {
	case '\'':
		if inCode {
			return open, true
		}
	case '`', '*', '_', '~':
		return open, true
	case '‘':
		return '’', true
	default:
		return 0, false
	}
	return 0, false
}

// pairFor returns the closer inserted for an opening character. Apostrophes
// and backticks are paired only inside code, matching the editor's typing
// behavior.
func pairFor(open rune, inCode bool) (rune, bool) {
	if close, ok := autoPairClosers[open]; ok {
		return close, true
	}
	if inCode && (open == '\'' || open == '`') {
		return open, true
	}
	return 0, false
}

// AutoPairInput contains source facts needed to decide whether typing an
// opener should insert a matching closer. Word classification is supplied by
// the editor's source-navigation policy.
type AutoPairInput struct {
	Open, Next   rune
	PreviousWord bool
	InCode       bool
	InInlineCode bool
}

// PlanAutoPair applies the inline typing policy without mutating a buffer.
func PlanAutoPair(in AutoPairInput) (rune, bool) {
	if in.Open == '[' && (in.InCode || in.InInlineCode) {
		return 0, false
	}
	close, ok := pairFor(in.Open, in.InCode)
	if !ok {
		return 0, false
	}
	if in.Next != 0 && in.Next != ' ' && in.Next != '\n' && in.Next != '\t' && !strings.ContainsRune(")]}>\"'`）】》”」", in.Next) {
		return 0, false
	}
	if (in.Open == '"' || in.Open == '\'' || in.Open == '`') && in.PreviousWord {
		return 0, false
	}
	return close, true
}

// PlanAutoPairAt derives the source facts for an auto-pair decision and then
// applies the inline typing policy. caret is a UTF-8 byte offset; inCode is a
// parser-projected host fact for fenced or code-like blocks.
func PlanAutoPairAt(source string, caret int, open rune, inCode bool) (rune, bool) {
	if caret < 0 || caret > len(source) || caret < len(source) && !utf8.RuneStart(source[caret]) {
		return 0, false
	}
	lineStart := strings.LastIndexByte(source[:caret], '\n') + 1
	previous, _ := utf8.DecodeLastRuneInString(source[:caret])
	var next rune
	if caret < len(source) {
		next, _ = utf8.DecodeRuneInString(source[caret:])
	}
	return PlanAutoPair(AutoPairInput{
		Open: open, Next: next, PreviousWord: navigation.IsWordRune(previous),
		InCode: inCode, InInlineCode: oddBacktickRun(source[lineStart:caret]),
	})
}

// ShouldSkipCloser reports whether typing closer at the caret should move
// over an already inserted closer instead of adding another character. The
// next source rune is supplied by the editor host as a source fact.
func ShouldSkipCloser(lineBefore string, next, closer rune) bool {
	if next != closer {
		return false
	}
	if !strings.ContainsRune(")]}\"'`）】》”」", closer) {
		return false
	}
	open := rune(0)
	for candidate, close := range autoPairClosers {
		if candidate != close && close == closer {
			open = candidate
			break
		}
	}
	if open != 0 && strings.Count(lineBefore, string(open)) <= strings.Count(lineBefore, string(closer)) {
		return false
	}
	if open == 0 && strings.Count(lineBefore, string(closer))%2 == 0 {
		return false
	}
	return true
}

// ShouldSkipCloserAt derives line and lookahead facts from a source snapshot
// before applying the closer-skip rule.
func ShouldSkipCloserAt(source string, caret int, closer rune) bool {
	if caret < 0 || caret > len(source) || caret < len(source) && !utf8.RuneStart(source[caret]) {
		return false
	}
	lineStart := strings.LastIndexByte(source[:caret], '\n') + 1
	next, _ := utf8.DecodeRuneInString(source[caret:])
	return ShouldSkipCloser(source[lineStart:caret], next, closer)
}

func oddBacktickRun(source string) bool {
	count := 0
	for i := 0; i < len(source); {
		if source[i] != '`' {
			i++
			continue
		}
		j := i + 1
		for j < len(source) && source[j] == '`' {
			j++
		}
		count++
		i = j
	}
	return count%2 == 1
}
