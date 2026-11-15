package syntax

// Conformance tests: the CommonMark spec's own examples pin the block
// structure of the model, and the tests below pin what the model keeps of
// each Markdown construct. The spec file is ../testdata/commonmark/spec.json,
// downloaded from https://spec.commonmark.org/0.31.2/spec.json.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// specExample is one example of the CommonMark spec: its Markdown, the
// reference HTML, and where it appears.
type specExample struct {
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
	Example  int    `json:"example"`
	Section  string `json:"section"`
}

func loadSpec(t *testing.T) []specExample {
	t.Helper()
	b, err := os.ReadFile("../../testdata/commonmark/spec.json")
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	var examples []specExample
	if err := json.Unmarshal(b, &examples); err != nil {
		t.Fatalf("decode spec: %v", err)
	}
	// The spec has 652 examples; a smaller file means a truncated download.
	if len(examples) < 600 {
		t.Fatalf("spec has %d examples, want at least 600", len(examples))
	}
	return examples
}

// TestCommonMarkSpecBlockStructure runs every spec example through the
// model and checks that it keeps the parser's block structure: one model
// block per parser block, of a kind that source can produce. A missing
// block is text silently dropped; a wrong kind is a construct shown as
// something else.
func TestCommonMarkSpecBlockStructure(t *testing.T) {
	for _, ex := range loadSpec(t) {
		ex := ex
		name := fmt.Sprintf("%03d_%s", ex.Example, strings.ReplaceAll(ex.Section, " ", "_"))
		t.Run(name, func(t *testing.T) {
			d := Parse(ex.Markdown)
			// PlainText is what the AI and plain viewers read; no example
			// may make it panic.
			_ = d.PlainText()

			root := parser.Parser().Parse(text.NewReader([]byte(CJKFriendly(ex.Markdown))))
			var nodes []ast.Node
			for c := root.FirstChild(); c != nil; c = c.NextSibling() {
				// A link reference definition produces no block of its own.
				if c.Kind() == ast.KindLinkReferenceDefinition {
					continue
				}
				nodes = append(nodes, c)
			}
			if len(d.Blocks) != len(nodes) {
				t.Fatalf("markdown %q: model has %d blocks, parser has %d",
					ex.Markdown, len(d.Blocks), len(nodes))
			}
			for i, n := range nodes {
				if !kindFits(d.Blocks[i].Kind, n) {
					t.Fatalf("markdown %q: block %d is %v, parser node %T",
						ex.Markdown, i, d.Blocks[i].Kind, n)
				}
			}
		})
	}
}

// kindFits reports whether the model may show parser node n as a block of
// kind k.
func kindFits(k Kind, n ast.Node) bool {
	switch n.(type) {
	case *ast.Heading:
		return k == Heading
	case *ast.Paragraph:
		// A paragraph holding one display formula or one image becomes a
		// block of its own.
		return k == Paragraph || k == Math || k == Image
	case *ast.TextBlock:
		return k == Paragraph
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		return k == Code
	case *ast.Blockquote:
		return k == Quote
	case *ast.List:
		return k == List
	case *ast.ThematicBreak:
		return k == Rule
	case *ast.HTMLBlock:
		// HTML that is one heading or one image keeps its native block.
		return k == HTML || k == Heading || k == Image
	case *east.Table:
		return k == Table
	}
	// A node kind the model deliberately repurposes.
	return true
}

func TestCommonMarkHeadings(t *testing.T) {
	d := Parse("# one\n## two ##\n### three\n\nSetext\n======\n\nSub\n---\n")
	want := []struct {
		level int
		text  string
	}{{1, "one"}, {2, "two"}, {3, "three"}, {1, "Setext"}, {2, "Sub"}}
	if len(d.Blocks) != len(want) {
		t.Fatalf("blocks = %+v", d.Blocks)
	}
	for i, w := range want {
		b := d.Blocks[i]
		if b.Kind != Heading || b.Level != w.level || spansText(b.Inlines) != w.text {
			t.Errorf("block %d = kind %v level %d text %q, want heading %d %q",
				i, b.Kind, b.Level, spansText(b.Inlines), w.level, w.text)
		}
	}
}

func TestCommonMarkThematicBreaks(t *testing.T) {
	d := Parse("***\n\n___\n\n- - -\n")
	if len(d.Blocks) != 3 {
		t.Fatalf("blocks = %+v", d.Blocks)
	}
	for i, b := range d.Blocks {
		if b.Kind != Rule {
			t.Errorf("block %d is %v, want rule", i, b.Kind)
		}
	}
}

func TestCommonMarkNestedQuotes(t *testing.T) {
	d := Parse("> outer\n>\n> > inner\n")
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != Quote {
		t.Fatalf("blocks = %+v", d.Blocks)
	}
	children := d.Blocks[0].Children
	if len(children) != 2 ||
		children[0].Kind != Paragraph || spansText(children[0].Inlines) != "outer" ||
		children[1].Kind != Quote || len(children[1].Children) != 1 ||
		spansText(children[1].Children[0].Inlines) != "inner" {
		t.Fatalf("quote children = %+v", children)
	}
}

func TestCommonMarkLists(t *testing.T) {
	// Ordered lists keep their first number; items keep their markers.
	d := Parse("3. three\n4. four\n")
	l := d.Blocks[0]
	if l.Kind != List || !l.Ordered || l.Start != 3 {
		t.Fatalf("ordered list = %+v", l)
	}
	if len(l.Items) != 2 || l.Items[0].Marker != "3." || l.Items[1].Marker != "4." {
		t.Fatalf("markers = %q", []string{l.Items[0].Marker, l.Items[1].Marker})
	}

	// A nested list is a child block of its item.
	d = Parse("- a\n  - b\n")
	l = d.Blocks[0]
	if len(l.Items) != 1 || spansText(l.Items[0].Inlines) != "a" {
		t.Fatalf("outer list = %+v", l)
	}
	if len(l.Items[0].Children) != 1 || l.Items[0].Children[0].Kind != List {
		t.Fatalf("nested blocks = %+v", l.Items[0].Children)
	}

	// A second paragraph of an item is also a child block.
	d = Parse("- a\n\n  second\n")
	l = d.Blocks[0]
	if len(l.Items) != 1 || len(l.Items[0].Children) != 1 ||
		spansText(l.Items[0].Children[0].Inlines) != "second" {
		t.Fatalf("loose item = %+v", l.Items[0])
	}

	// A loose list still yields every item.
	d = Parse("- a\n\n- b\n")
	if l = d.Blocks[0]; l.Kind != List || len(l.Items) != 2 {
		t.Fatalf("loose list = %+v", l)
	}
}

func TestCommonMarkCodeBlocks(t *testing.T) {
	d := Parse("```go\nfmt.Println(1)\n```\n\n~~~js\nlet x = 1;\n~~~\n\n    indented\n    code\n")
	if len(d.Blocks) != 3 {
		t.Fatalf("blocks = %+v", d.Blocks)
	}
	if b := d.Blocks[0]; b.Kind != Code || b.Lang != "go" || b.Text != "fmt.Println(1)\n" {
		t.Errorf("backtick fence = %+v", b)
	}
	if b := d.Blocks[1]; b.Kind != Code || b.Lang != "js" || b.Text != "let x = 1;\n" {
		t.Errorf("tilde fence = %+v", b)
	}
	if b := d.Blocks[2]; b.Kind != Code || b.Lang != "" || b.Text != "indented\ncode\n" {
		t.Errorf("indented code = %+v", b)
	}
}

func TestCommonMarkHTMLBlocks(t *testing.T) {
	d := Parse("<div>\nraw <b>html</b>\n</div>\n\n<!-- note -->\n\n<h3>Title</h3>\n")
	if len(d.Blocks) != 3 {
		t.Fatalf("blocks = %+v", d.Blocks)
	}
	if b := d.Blocks[0]; b.Kind != HTML || !strings.Contains(b.Text, "<div>") || !strings.Contains(b.Text, "<b>html</b>") {
		t.Errorf("div block = %+v", b)
	}
	if b := d.Blocks[1]; b.Kind != HTML || !strings.Contains(b.Text, "note") {
		t.Errorf("comment block = %+v", b)
	}
	if b := d.Blocks[2]; b.Kind != Heading || b.Level != 3 || spansText(b.Inlines) != "Title" {
		t.Errorf("html heading = %+v", b)
	}
}

func TestCommonMarkTable(t *testing.T) {
	d := Parse("| a | b | c |\n|:--|:-:|--:|\n| 1 | **2** | 3 |\n")
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != Table {
		t.Fatalf("blocks = %+v", d.Blocks)
	}
	tb := d.Blocks[0]
	wantAlign := []Align{AlignLeft, AlignCenter, AlignRight}
	if len(tb.Align) != 3 {
		t.Fatalf("align = %v", tb.Align)
	}
	for i, a := range wantAlign {
		if tb.Align[i] != a {
			t.Errorf("align %d = %v, want %v", i, tb.Align[i], a)
		}
	}
	if len(tb.Rows) != 2 || len(tb.Rows[1]) != 3 {
		t.Fatalf("rows = %+v", tb.Rows)
	}
	if spansText(tb.Rows[0][0]) != "a" || spansText(tb.Rows[1][0]) != "1" || spansText(tb.Rows[1][2]) != "3" {
		t.Errorf("cells = %+v", tb.Rows)
	}
	if !hasBold(tb.Rows[1][1], "2") {
		t.Errorf("bold cell = %+v", tb.Rows[1][1])
	}
}

func TestCommonMarkImages(t *testing.T) {
	d := Parse("![alt](img.png \"Title\")\n\nText ![icon](i.png) end\n")
	if len(d.Blocks) != 2 {
		t.Fatalf("blocks = %+v", d.Blocks)
	}
	img := d.Blocks[0]
	if img.Kind != Image || img.Src != "img.png" || img.Alt != "alt" || img.Title != "Title" {
		t.Fatalf("image block = %+v", img)
	}
	var got *Span
	for i, sp := range d.Blocks[1].Inlines {
		if strings.Contains(sp.Text, "icon") {
			got = &d.Blocks[1].Inlines[i]
		}
	}
	if got == nil || got.Link != "i.png" {
		t.Fatalf("inline image span = %+v", d.Blocks[1].Inlines)
	}
}

func TestCommonMarkEmphasis(t *testing.T) {
	d := Parse("plain *it* **bold** ***both*** _u_ __ub__ ~~gone~~ `code`\n")
	if len(d.Blocks) != 1 {
		t.Fatalf("blocks = %+v", d.Blocks)
	}
	spans := d.Blocks[0].Inlines
	for _, c := range []struct {
		text                       string
		bold, italic, strike, code bool
	}{
		{"it", false, true, false, false},
		{"bold", true, false, false, false},
		{"both", true, true, false, false},
		{"u", false, true, false, false},
		{"ub", true, false, false, false},
		{"gone", false, false, true, false},
		{"code", false, false, false, true},
	} {
		if !hasSpan(spans, c.text, func(s Span) bool {
			return s.Bold == c.bold && s.Italic == c.italic &&
				s.Strike == c.strike && s.Code == c.code
		}) {
			t.Errorf("%q missing with bold=%v italic=%v strike=%v code=%v: %+v",
				c.text, c.bold, c.italic, c.strike, c.code, spans)
		}
	}
}

func TestCommonMarkLinks(t *testing.T) {
	src := "[inline](https://a.example \"t\") [ref][id] [shortcut] <https://auto.example> https://bare.example user@example.com\n\n" +
		"[id]: https://ref.example\n" +
		"[shortcut]: https://short.example\n"
	d := Parse(src)
	if len(d.Blocks) != 1 {
		t.Fatalf("blocks = %+v", d.Blocks)
	}
	spans := d.Blocks[0].Inlines
	for _, link := range []string{
		"https://a.example",
		"https://ref.example",
		"https://short.example",
		"https://auto.example",
		"https://bare.example",
		"mailto:user@example.com",
	} {
		if !hasSpan(spans, "", func(s Span) bool { return s.Link == link }) {
			t.Errorf("link %q missing: %+v", link, spans)
		}
	}
	if !hasSpan(spans, "inline", func(s Span) bool { return s.Link == "https://a.example" }) {
		t.Errorf("inline link text lost: %+v", spans)
	}
}

func TestCommonMarkEscapesAndEntities(t *testing.T) {
	d := Parse(`\*x\* &amp; &#35; &copy;` + "\n")
	if got := strings.TrimSpace(d.PlainText()); got != "*x* & # ©" {
		t.Fatalf("plain text = %q", got)
	}
}

func TestCommonMarkLineBreaks(t *testing.T) {
	for _, src := range []string{"a\nb\n", "a  \nb\n", "a\\\nb\n"} {
		d := Parse(src)
		if got := d.PlainText(); got != "a\nb\n" {
			t.Errorf("%q plain text = %q", src, got)
		}
	}
}

func TestCommonMarkInlineHTML(t *testing.T) {
	d := Parse("a <b>bold</b> c\n")
	if got := strings.TrimSpace(d.PlainText()); got != "a bold c" {
		t.Fatalf("plain text = %q", got)
	}
}

func TestCommonMarkInlineMath(t *testing.T) {
	d := Parse("energy $E = mc^2$ stays.\n")
	if len(d.Blocks) != 1 {
		t.Fatalf("blocks = %+v", d.Blocks)
	}
	if !hasSpan(d.Blocks[0].Inlines, "E = mc^2", func(s Span) bool { return s.Math }) {
		t.Fatalf("inline math missing: %+v", d.Blocks[0].Inlines)
	}
	// An escaped dollar is text, not math.
	d = Parse(`costs \$5 here` + "\n")
	if !hasSpan(d.Blocks[0].Inlines, "$5", func(s Span) bool { return !s.Math }) {
		t.Fatalf("escaped dollar = %+v", d.Blocks[0].Inlines)
	}
}

func TestGFMTaskList(t *testing.T) {
	d := Parse("- [ ] todo\n- [x] done\n")
	l := d.Blocks[0]
	if len(l.Items) != 2 || !l.Items[0].Task || l.Items[0].Checked || !l.Items[1].Task || !l.Items[1].Checked {
		t.Fatalf("task items = %+v", l.Items)
	}
}

func TestFootnoteDefinitions(t *testing.T) {
	d := Parse("正文[^a]与[^b]。\n\n[^a]: 第一条。\n\n[^b]: 第二条\n\n    第二段。\n")
	if len(d.Blocks) != 2 || d.Blocks[1].Kind != List {
		t.Fatalf("blocks = %+v", d.Blocks)
	}
	p := d.Blocks[0]
	for _, marker := range []string{"[1]", "[2]"} {
		if !hasSpan(p.Inlines, marker, func(Span) bool { return true }) {
			t.Errorf("reference %s missing: %+v", marker, p.Inlines)
		}
	}
	defs := d.Blocks[1]
	if len(defs.Items) != 2 ||
		defs.Items[0].Marker != "[1]" || spansText(defs.Items[0].Inlines) != "第一条。" ||
		defs.Items[1].Marker != "[2]" || spansText(defs.Items[1].Inlines) != "第二条" {
		t.Fatalf("footnote items = %+v", defs.Items)
	}
	if len(defs.Items[1].Children) != 1 || spansText(defs.Items[1].Children[0].Inlines) != "第二段。" {
		t.Fatalf("second footnote blocks = %+v", defs.Items[1].Children)
	}
	// The definition text must survive into plain text: this is what the
	// AI layer reads, and [^a]: … is a link definition to a CommonMark
	// parser without the footnote extension.
	if plain := d.PlainText(); !strings.Contains(plain, "第一条。") || !strings.Contains(plain, "第二段。") {
		t.Fatalf("plain text = %q", plain)
	}
}

// hasSpan reports whether spans holds a span whose text contains text and
// satisfies pred. An empty text matches any text.
func hasSpan(spans []Span, text string, pred func(Span) bool) bool {
	for _, s := range spans {
		if pred(s) && strings.Contains(s.Text, text) {
			return true
		}
	}
	return false
}

func hasBold(spans []Span, text string) bool {
	return hasSpan(spans, text, func(span Span) bool { return span.Bold })
}
