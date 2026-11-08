package mdedit

// Conformance tests for the editor's Markdown parser (MyGo's
// ui/markdown/parser), against the CommonMark spec's 652 examples in
// ../testdata/commonmark/spec.json.
//
// The layers are tested separately: the engine with no extensions, the
// GFM extensions the editor always enables, and the editor's own math
// dialect, where \[...\] is display math but an escaped \[ must stay text
// (mdedit masks escaped brackets before parsing; see editMask).

import (
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/markdown/parser"
)

type specExample struct {
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
	Example  int    `json:"example"`
	Section  string `json:"section"`
}

func loadSpec(t *testing.T) []specExample {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "commonmark", "spec.json"))
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	var examples []specExample
	if err := json.Unmarshal(b, &examples); err != nil {
		t.Fatalf("decode spec: %v", err)
	}
	if len(examples) < 600 {
		t.Fatalf("spec has %d examples, want at least 600", len(examples))
	}
	return examples
}

// TestCommonMarkSpecHTML is the engine's conformance: every example,
// parsed with no extensions, must render exactly the spec's reference
// HTML. This is the base every dialect is built on.
func TestCommonMarkSpecHTML(t *testing.T) {
	for _, ex := range loadSpec(t) {
		d := parser.New(0)
		d.Set(ex.Markdown)
		if got := d.HTML(); got != ex.HTML {
			t.Errorf("example %d (%s):\nmarkdown %q\nwant %q\ngot  %q",
				ex.Example, ex.Section, ex.Markdown, ex.HTML, got)
		}
	}
}

// gfmLinkifyExamples are the CommonMark examples where GFM's linkify
// extension adds links the spec does not: bare URLs and addresses, and
// URLs that sit inside angle brackets that do not form a valid autolink.
// The text stays the same; only the <a> wrapper is added.
var gfmLinkifyExamples = map[int]bool{602: true, 606: true, 608: true, 611: true, 612: true}

func TestGFMSpecHTML(t *testing.T) {
	seen := map[int]bool{}
	for _, ex := range loadSpec(t) {
		d := parser.New(parser.GFM)
		d.Set(ex.Markdown)
		got := d.HTML()
		if got == ex.HTML {
			continue
		}
		if gfmLinkifyExamples[ex.Example] {
			seen[ex.Example] = true
			if stripAnchorTags(got) != ex.HTML {
				t.Errorf("example %d: GFM changed more than its links:\nmarkdown %q\nwant %q\ngot  %q",
					ex.Example, ex.Markdown, ex.HTML, got)
			}
			continue
		}
		t.Errorf("example %d (%s) differs under GFM:\nmarkdown %q\nwant %q\ngot  %q",
			ex.Example, ex.Section, ex.Markdown, ex.HTML, got)
	}
	// If an example stops differing, the allowlist is stale, not wrong.
	for n := range gfmLinkifyExamples {
		if !seen[n] {
			t.Errorf("example %d no longer differs under GFM; drop it from gfmLinkifyExamples", n)
		}
	}
}

// TestEditorEscapedBracketsAreNotMath pins the editor's math dialect on
// the three spec examples that contain escaped brackets. Adding ExtMath
// makes the bare parser read \[...\] as display math and swallow the rest
// of the example, so mdedit masks escaped brackets before parsing; what
// the reader sees must still be the spec's text, not a formula.
func TestEditorEscapedBracketsAreNotMath(t *testing.T) {
	for _, ex := range loadSpec(t) {
		switch ex.Example {
		case 12, 14, 563:
		default:
			continue
		}
		e := shownEditor(t, ex.Markdown)
		if doc := e.p.doc.HTML(); strings.Contains(doc, `class="math`) {
			t.Errorf("example %d parsed as math:\n%s", ex.Example, doc)
		}
		got := normalizeSpace(displayPlainText(e))
		want := normalizeSpace(htmlText(ex.HTML))
		if got != want {
			t.Errorf("example %d (%s) displays %q, want %q", ex.Example, ex.Section, got, want)
		}
	}
}

var (
	anchorTag   = regexp.MustCompile(`<a href="[^"]*">|</a>`)
	tagRE       = regexp.MustCompile(`<[^>]*>`)
	styleMarker = regexp.MustCompile(`«[a-z,]*:`)
	spaces      = regexp.MustCompile(`\s+`)
)

func stripAnchorTags(s string) string { return anchorTag.ReplaceAllString(s, "") }

// htmlText is the text a reference HTML shows: its tags dropped first,
// then its character references resolved.
func htmlText(s string) string {
	return html.UnescapeString(tagRE.ReplaceAllString(s, ""))
}

// displayPlainText is what the editor draws as text, with the style
// markers of the test helpers removed.
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
