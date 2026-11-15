package markdown

import (
	"strings"
	"testing"
)

func TestParseBlocks(t *testing.T) {
	src := "# Title\n\nHello **world** and `code`.\n\n- one\n- two\n\n> quoted\n\n```go\nfmt.Println(1)\n```\n\n---\n"
	d := Parse(src)

	want := []Kind{Heading, Paragraph, List, Quote, Code, Rule}
	if len(d.Blocks) != len(want) {
		t.Fatalf("got %d blocks, want %d: %+v", len(d.Blocks), len(want), d.Blocks)
	}
	for i, k := range want {
		if d.Blocks[i].Kind != k {
			t.Errorf("block %d kind = %v, want %v", i, d.Blocks[i].Kind, k)
		}
	}

	if h := d.Blocks[0]; h.Level != 1 || spansText(h.Inlines) != "Title" {
		t.Errorf("heading = level %d %q", h.Level, spansText(h.Inlines))
	}
	if p := d.Blocks[1]; !hasBold(p.Inlines, "world") || !hasCode(p.Inlines, "code") {
		t.Errorf("paragraph lost styling: %+v", p.Inlines)
	}
	if l := d.Blocks[2]; l.Ordered || len(l.Items) != 2 || spansText(l.Items[0].Inlines) != "one" {
		t.Errorf("list = %+v", l)
	}
	if q := d.Blocks[3]; len(q.Children) != 1 || spansText(q.Children[0].Inlines) != "quoted" {
		t.Errorf("quote = %+v", q)
	}
	if cd := d.Blocks[4]; cd.Lang != "go" || cd.Text != "fmt.Println(1)\n" {
		t.Errorf("code = %q lang %q", cd.Text, cd.Lang)
	}
}

func TestParseOrderedListAndTable(t *testing.T) {
	d := Parse("1. first\n2. second\n\n| a | b |\n|---|---|\n| 1 | 2 |\n")

	l := d.Blocks[0]
	if !l.Ordered || l.Items[0].Marker != "1." || l.Items[1].Marker != "2." {
		t.Errorf("ordered list = %+v", l)
	}
	tbl := d.Blocks[1]
	if tbl.Kind != Table || len(tbl.Rows) != 2 || spansText(tbl.Rows[0][0]) != "a" {
		t.Errorf("table = %+v", tbl)
	}
}

// Markdown semantics: a wrapped source line is one paragraph, joined by a
// space rather than a break.
func TestParseSoftBreakKeepsLine(t *testing.T) {
	d := Parse("just text\nand more\n")
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != Paragraph {
		t.Fatalf("blocks = %+v", d.Blocks)
	}
	if got := d.PlainText(); got != "just text\nand more\n" {
		t.Errorf("markdown soft break = %q", got)
	}
}

// A plain .txt file must open too: ParsePlainText keeps every line and
// shows Markdown syntax literally, so the same viewer serves both types.
func TestParsePlainTextKeepsLines(t *testing.T) {
	d := ParsePlainText("# not a heading\nplain line\r\n")
	if len(d.Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2: %+v", len(d.Blocks), d.Blocks)
	}
	if got := d.PlainText(); got != "# not a heading\nplain line\n" {
		t.Errorf("plain text = %q", got)
	}
}

func TestPlainTextKeepsNesting(t *testing.T) {
	d := Parse("- outer\n  - inner\n")
	got := d.PlainText()
	if !strings.Contains(got, "• outer") || !strings.Contains(got, "  • inner") {
		t.Errorf("nested list text = %q", got)
	}
}

func hasBold(spans []Span, text string) bool {
	for _, s := range spans {
		if s.Bold && strings.Contains(s.Text, text) {
			return true
		}
	}
	return false
}

func hasCode(spans []Span, text string) bool {
	for _, s := range spans {
		if s.Code && strings.Contains(s.Text, text) {
			return true
		}
	}
	return false
}

func TestParseTaskListAndStandaloneImage(t *testing.T) {
	d := Parse("- [ ] todo\n- [x] done\n\n![cover](assets/cover.png)\n")
	list := d.Blocks[0]
	if list.Kind != List || len(list.Items) != 2 {
		t.Fatalf("list = %+v", list)
	}
	if !list.Items[0].Task || list.Items[0].Checked {
		t.Errorf("first task = %+v", list.Items[0])
	}
	if !list.Items[1].Task || !list.Items[1].Checked {
		t.Errorf("second task = %+v", list.Items[1])
	}
	img := d.Blocks[1]
	if img.Kind != Image || img.Src != "assets/cover.png" || img.Alt != "cover" {
		t.Errorf("image = %+v", img)
	}
}

func TestParseHTMLImageBlock(t *testing.T) {
	d := Parse("<p align=\"center\"><img src=\"resources/icon.png\" alt=\"Muse\" width=\"128\"></p>\n")
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != Image {
		t.Fatalf("blocks = %+v", d.Blocks)
	}
	img := d.Blocks[0]
	if img.Src != "resources/icon.png" || img.Alt != "Muse" || img.ImgWidth != 128 {
		t.Fatalf("image = %+v", img)
	}
	mixed := Parse("<p>Hello <img src=\"x.png\" alt=\"x\"></p>\n")
	if mixed.Blocks[0].Kind != HTML {
		t.Fatalf("mixed HTML became %v", mixed.Blocks[0].Kind)
	}
}

func TestParseHTMLHeading(t *testing.T) {
	d := Parse("<h1 align=\"center\">Muse</h1>\n")
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != Heading || d.Blocks[0].Level != 1 {
		t.Fatalf("blocks = %+v", d.Blocks)
	}
	if got := spansText(d.Blocks[0].Inlines); got != "Muse" {
		t.Fatalf("heading text = %q", got)
	}
}

func TestParseInlineAndBlockMath(t *testing.T) {
	d := Parse("energy $E = mc^2$ stays.\n\n$$\n\\frac{a}{b} + \\alpha\n$$\n")
	if len(d.Blocks) != 2 {
		t.Fatalf("blocks = %+v", d.Blocks)
	}
	p := d.Blocks[0]
	found := false
	for _, sp := range p.Inlines {
		if sp.Math {
			found = true
			if sp.Text != "E = mc^2" {
				t.Errorf("math text = %q", sp.Text)
			}
		}
	}
	if !found {
		t.Fatalf("inline math missing: %+v", p.Inlines)
	}
	m := d.Blocks[1]
	if m.Kind != Math || !strings.Contains(m.Text, "frac") {
		t.Fatalf("math block = %+v", m)
	}
}
