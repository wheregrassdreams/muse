package inline

import "strings"

var autoPairClosers = map[rune]rune{
	'(': ')', '[': ']', '{': '}', '"': '"',
	'（': '）', '【': '】', '《': '》', '“': '”', '「': '」',
}

// PairFor returns the closer inserted for an opening character. Apostrophes
// and backticks are paired only inside code, matching the editor's typing
// behavior.
func PairFor(open rune, inCode bool) (rune, bool) {
	if close, ok := autoPairClosers[open]; ok {
		return close, true
	}
	if inCode && (open == '\'' || open == '`') {
		return open, true
	}
	return 0, false
}

// PlanAutoPair applies the inline typing policy without mutating a buffer.
// afterWord is supplied by the editor's navigation policy so quote handling
// uses the same word definition as cursor movement.
func PlanAutoPair(open, next rune, afterWord, inCode bool) (rune, bool) {
	close, ok := PairFor(open, inCode)
	if !ok {
		return 0, false
	}
	if next != 0 && next != ' ' && next != '\n' && next != '\t' && !strings.ContainsRune(")]}>\"'`）】》”」", next) {
		return 0, false
	}
	if (open == '"' || open == '\'' || open == '`') && afterWord {
		return 0, false
	}
	return close, true
}

// ShouldSkipCloser reports whether typing closer at the caret should move
// over an already inserted closer instead of adding another character.
func ShouldSkipCloser(lineBefore string, closer rune) bool {
	if !strings.ContainsRune(")]}\"'`）】》”」", closer) {
		return false
	}
	open := map[rune]rune{')': '(', ']': '[', '}': '{', '）': '（', '】': '【', '》': '《', '”': '“', '」': '「'}[closer]
	if open != 0 && strings.Count(lineBefore, string(open)) <= strings.Count(lineBefore, string(closer)) {
		return false
	}
	if open == 0 && strings.Count(lineBefore, string(closer))%2 == 0 {
		return false
	}
	return true
}
