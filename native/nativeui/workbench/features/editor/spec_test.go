package editor

// This pins the editor's math dialect: escaped brackets remain text even
// though the syntax parser also supports \[...\] display math.

import (
	"regexp"
	"strings"
	"testing"

	syntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"
)

func TestEditorEscapedBracketsAreNotMath(t *testing.T) {
	tests := []struct {
		example  int
		markdown string
		plain    string
	}{
		{
			example:  12,
			markdown: "\\!\\\"\\#\\$\\%\\&\\'\\(\\)\\*\\+\\,\\-\\.\\/\\:\\;\\<\\=\\>\\?\\@\\[\\\\\\]\\^\\_\\`\\{\\|\\}\\~\n",
			plain:    "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~",
		},
		{
			example: 14,
			markdown: "\\*not emphasized*\n" +
				"\\<br/> not a tag\n" +
				"\\[not a link](/foo)\n" +
				"\\`not code`\n" +
				"1\\. not a list\n" +
				"\\* not a list\n" +
				"\\# not a heading\n" +
				"\\[foo]: /url \"not a reference\"\n" +
				"\\&ouml; not a character entity\n",
			plain: "*not emphasized* <br/> not a tag [not a link](/foo) `not code` " +
				"1. not a list * not a list # not a heading [foo]: /url \"not a reference\" " +
				"&ouml; not a character entity",
		},
		{
			example:  563,
			markdown: "\\[foo]\n\n[foo]: /url \"title\"\n",
			plain:    "[foo]",
		},
	}
	for _, ex := range tests {
		e := shownEditor(t, ex.markdown)
		parser := e.p.Parser()
		parsedMath := false
		for blockIndex := 0; blockIndex < parser.Len(); blockIndex++ {
			block := parser.Block(blockIndex)
			parsedMath = parsedMath || block.Kind == syntax.IncrementalMathBlock
			for _, inline := range parser.Inlines(blockIndex) {
				parsedMath = parsedMath || inline.Kind == syntax.IncrementalMath
			}
		}
		if parsedMath {
			t.Errorf("example %d parsed escaped brackets as math", ex.example)
		}
		if got := normalizeSpace(displayPlainText(e)); got != normalizeSpace(ex.plain) {
			t.Errorf("example %d displays %q, want %q", ex.example, got, normalizeSpace(ex.plain))
		}
	}
}

var (
	styleMarker = regexp.MustCompile(`«[a-z,]*:`)
	spaces      = regexp.MustCompile(`\s+`)
)

// displayPlainText is what the editor draws as text, with style markers
// removed from these host-integration examples.
func displayPlainText(e *Editor) string {
	var parts []string
	for _, u := range e.units() {
		if u.isText() && u.view != nil {
			s := styleMarker.ReplaceAllString(styledText(e, u.view.disp), "")
			parts = append(parts, strings.ReplaceAll(s, "»", ""))
		}
	}
	return strings.Join(parts, "\n")
}

func normalizeSpace(s string) string { return strings.TrimSpace(spaces.ReplaceAllString(s, " ")) }
