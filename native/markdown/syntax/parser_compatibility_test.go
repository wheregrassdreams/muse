package syntax

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/yuin/goldmark/text"
	parser "muse/markdown/syntax/internal/incremental"
)

// This test covers the syntax subset supported by the production incremental
// parser. Goldmark is used only as a test oracle while the common model
// assertions pin the public syntax contract.
func TestParserSharedBlockSemantics(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   []Block
	}{
		{"heading and paragraph", "# title\n\nplain text\n", []Block{
			{Kind: Heading, Level: 1, Inlines: []Span{{Text: "title"}}},
			{Kind: Paragraph, Inlines: []Span{{Text: "plain text"}}},
		}},
		{"thematic break", "---\n", []Block{{Kind: Rule}}},
		{"fenced code", "```go\nfmt.Println(1)\n```\n", []Block{{Kind: Code, Lang: "go", Text: "fmt.Println(1)\n"}}},
		{"blockquote", "> quoted text\n", []Block{{Kind: Quote, Children: []Block{{Kind: Paragraph, Inlines: []Span{{Text: "quoted text"}}}}}}},
		{"GFM task list", "- [x] shipped\n- [ ] pending\n", []Block{{Kind: List, Items: []Item{
			{Marker: "•", Inlines: []Span{{Text: "shipped"}}, Task: true, Checked: true},
			{Marker: "•", Inlines: []Span{{Text: "pending"}}, Task: true},
		}}}},
		{"table and alignment", "| a | b |\n| :--- | ---: |\n| c | d |\n", []Block{{Kind: Table, Align: []Align{AlignLeft, AlignRight}, Rows: [][]Cell{
			{Cell{{Text: "a"}}, Cell{{Text: "b"}}},
			{Cell{{Text: "c"}}, Cell{{Text: "d"}}},
		}}}},
		{"inline math", "value $x^2$\n", []Block{{Kind: Paragraph, Inlines: []Span{{Text: "value "}, {Text: "x^2", Math: true}}}}},
		{"display math", "$$\nx^2\n$$\n", []Block{{Kind: Math, Text: "x^2"}}},
		{"plain HTML block", "<div>\ncontent\n</div>\n", []Block{{Kind: HTML, Text: "<div>\ncontent\n</div>\n"}}},
		{"CJK emphasis", "你好**世界**。\n", []Block{{Kind: Paragraph, Inlines: []Span{{Text: "你好"}, {Text: "世界", Bold: true}, {Text: "。"}}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := parser.New(defaultExtensions)
			d.Set(tt.source)
			gotIncremental := normalizeIncremental(d, 0)
			doc := goldmarkSyntaxDoc(tt.source)
			if !reflect.DeepEqual(gotIncremental, tt.want) {
				t.Fatalf("incremental normalized tree = %#v, want %#v", gotIncremental, tt.want)
			}
			if !reflect.DeepEqual(doc.Blocks, tt.want) {
				t.Fatalf("Goldmark oracle tree = %#v, want %#v", doc.Blocks, tt.want)
			}
		})
	}
}

type auditStyle struct {
	Bold, Italic, Code, Strike bool
	Link                       string
}

func normalizeIncremental(d *parser.Document, parent int) []Block {
	var out []Block
	for i := range d.Children(parent) {
		if normalized, ok := normalizeIncrementalBlock(d, i); ok {
			out = append(out, normalized)
		}
	}
	return out
}

func normalizeIncrementalBlock(d *parser.Document, index int) (Block, bool) {
	b := d.Block(index)
	var normalized Block
	switch b.Kind {
	case parser.Paragraph:
		in := d.Inlines(index)
		normalized = Block{Kind: Paragraph, Inlines: normalizeInlines(in, 0, len(in), auditStyle{})}
	case parser.Heading:
		in := d.Inlines(index)
		normalized = Block{Kind: Heading, Level: b.Level, Inlines: normalizeInlines(in, 0, len(in), auditStyle{})}
	case parser.CodeBlock:
		normalized = Block{Kind: Code, Lang: b.Language(), Text: d.Lines(index)}
	case parser.HTMLBlock:
		normalized = Block{Kind: HTML, Text: d.Lines(index)}
	case parser.Quote:
		normalized = Block{Kind: Quote, Children: normalizeIncremental(d, index)}
	case parser.List:
		normalized = Block{Kind: List, Ordered: b.Ordered, Start: b.Number}
		for child := range d.Children(index) {
			itemBlock := d.Block(child)
			if itemBlock.Kind != parser.Item {
				continue
			}
			item := Item{Marker: "•", Task: itemBlock.Task != parser.NoTask, Checked: itemBlock.Task == parser.Checked}
			if itemBlock.Ordered {
				item.Marker = strconv.Itoa(itemBlock.Number) + string(itemBlock.Marker)
			}
			for itemChild := range d.Children(child) {
				if d.Block(itemChild).Kind == parser.Paragraph && len(item.Inlines) == 0 {
					inline := d.Inlines(itemChild)
					item.Inlines = normalizeInlines(inline, 0, len(inline), auditStyle{})
					continue
				}
				if childBlock, ok := normalizeIncrementalBlock(d, itemChild); ok {
					item.Children = append(item.Children, childBlock)
				}
			}
			normalized.Items = append(normalized.Items, item)
		}
	case parser.Table:
		normalized = Block{Kind: Table}
		for _, align := range b.Align {
			normalized.Align = append(normalized.Align, Align(align))
		}
		for row := range d.Children(index) {
			var cells []Cell
			for cell := range d.Children(row) {
				in := d.Inlines(cell)
				cells = append(cells, Cell(normalizeInlines(in, 0, len(in), auditStyle{})))
			}
			// Doc omits the parser's header-row flag; both models
			// define the first row as the header.
			if len(normalized.Rows) > 0 || d.Block(row).Header {
				normalized.Rows = append(normalized.Rows, cells)
			}
		}
	case parser.ThematicBreak:
		normalized = Block{Kind: Rule}
	case parser.MathBlock:
		normalized = Block{Kind: Math, Text: strings.TrimSpace(d.Lines(index))}
	case parser.FootnoteDefinition:
		// syntax maps footnote definitions to list items. The marker index
		// is assigned in reference order, so this conversion is in the
		// explicit footnote divergence test rather than guessed here.
		return Block{}, false
	default:
		return Block{}, false
	}
	return normalized, true
}

func normalizeInlines(in []parser.Inline, from, to int, style auditStyle) []Span {
	var out []Span
	appendSpan := func(span Span) {
		if span.Text == "" && !span.SoftBreak {
			return
		}
		if len(out) > 0 {
			last := &out[len(out)-1]
			if last.Bold == span.Bold && last.Italic == span.Italic && last.Code == span.Code && last.Strike == span.Strike && last.Link == span.Link && last.Math == span.Math && last.SoftBreak == span.SoftBreak {
				last.Text += span.Text
				return
			}
		}
		out = append(out, span)
	}
	for i := from; i < to; {
		n := in[i]
		switch n.Kind {
		case parser.Text, parser.Mention, parser.Reference:
			appendSpan(Span{Text: n.Text, Bold: style.Bold, Italic: style.Italic, Code: style.Code, Strike: style.Strike, Link: style.Link})
		case parser.SoftBreak:
			appendSpan(Span{Text: "\n", SoftBreak: true, Bold: style.Bold, Italic: style.Italic, Link: style.Link})
		case parser.LineBreak:
			appendSpan(Span{Text: "\n", Bold: style.Bold, Italic: style.Italic, Link: style.Link})
		case parser.Code:
			appendSpan(Span{Text: n.Text, Bold: style.Bold, Italic: style.Italic, Code: true, Strike: style.Strike, Link: style.Link})
		case parser.Math:
			appendSpan(Span{Text: n.Text, Bold: style.Bold, Italic: style.Italic, Strike: style.Strike, Link: style.Link, Math: true})
		case parser.Emphasis, parser.Strong, parser.Strikethrough, parser.Link:
			nested := style
			switch n.Kind {
			case parser.Emphasis:
				nested.Italic = true
			case parser.Strong:
				nested.Bold = true
			case parser.Strikethrough:
				nested.Strike = true
			case parser.Link:
				nested.Link = n.Dest
			}
			out = append(out, normalizeInlines(in, i+1, n.Next, nested)...)
			i = n.Next
			continue
		case parser.Image:
			appendSpan(Span{Text: "🖼 " + n.Text, Italic: true, Link: n.Dest})
		case parser.HTML:
			appendSpan(Span{Text: n.Text, Bold: style.Bold, Italic: style.Italic, Strike: style.Strike, Link: style.Link})
		case parser.FootnoteRef:
			appendSpan(Span{Text: "[" + n.Text + "]", Bold: style.Bold, Italic: style.Italic, Link: style.Link})
		}
		if n.Next > i {
			i = n.Next
		} else {
			i++
		}
	}
	return out
}

func TestParserSharedInlineSemantics(t *testing.T) {
	source := "**bold** *italic* ~~strike~~ `code` [link](https://example.test)"
	d := parser.New(defaultExtensions)
	d.Set(source)
	var incrementalID int
	for i := range d.Children(0) {
		if d.Block(i).Kind == parser.Paragraph {
			incrementalID = i
			break
		}
	}
	features := map[string]bool{"bold": false, "italic": false, "strike": false, "code": false, "link": false}
	for _, in := range d.Inlines(incrementalID) {
		switch in.Kind {
		case parser.Strong:
			features["bold"] = true
		case parser.Emphasis:
			features["italic"] = true
		case parser.Strikethrough:
			features["strike"] = true
		case parser.Code:
			features["code"] = true
		case parser.Link:
			features["link"] = true
		}
	}
	for name, found := range features {
		if !found {
			t.Errorf("incremental parser missed %s inlines", name)
		}
	}

	doc := goldmarkSyntaxDoc(source)
	full := map[string]bool{"bold": false, "italic": false, "strike": false, "code": false, "link": false}
	for _, span := range doc.Blocks[0].Inlines {
		full["bold"] = full["bold"] || span.Bold
		full["italic"] = full["italic"] || span.Italic
		full["strike"] = full["strike"] || span.Strike
		full["code"] = full["code"] || span.Code
		full["link"] = full["link"] || span.Link != ""
	}
	for name, found := range full {
		if !found {
			t.Errorf("Goldmark oracle missed %s inlines", name)
		}
	}
}

// This records intentional differences between Muse's syntax model and
// Goldmark's standard AST projection. It is not a second production parser.
func TestSyntaxGoldmarkOracleKnownSemanticDifferences(t *testing.T) {
	mathSource := "$x^2$"
	d := parser.New(defaultExtensions)
	d.Set(mathSource)
	var editorMath bool
	for i := range d.Children(0) {
		for _, in := range d.Inlines(i) {
			editorMath = editorMath || in.Kind == parser.Math
		}
	}
	fullMath := false
	for _, span := range goldmarkSyntaxDoc(mathSource).Blocks[0].Inlines {
		fullMath = fullMath || span.Math
	}
	if !editorMath || !fullMath {
		t.Fatalf("inline dollar math missing: incremental=%v oracle=%v", editorMath, fullMath)
	}
	t.Log("production syntax and Goldmark oracle recognize inline dollar math")

	alertSource := "> [!NOTE]\n> text\n"
	d = parser.New(defaultExtensions)
	d.Set(alertSource)
	var alert parser.Alert
	for i := range d.Children(0) {
		if b := d.Block(i); b.Kind == parser.Quote {
			alert = b.Alert
		}
	}
	if alert != parser.AlertNote {
		t.Fatalf("incremental parser alert = %v, want Note", alert)
	}
	if len(goldmarkSyntaxDoc(alertSource).Blocks) == 0 || goldmarkSyntaxDoc(alertSource).Blocks[0].Kind != Quote {
		t.Fatal("Goldmark oracle lost the quoted alert block")
	}
	t.Log("Muse syntax classifies the alert; Goldmark's standard AST exposes an ordinary quote")

	footnoteSource := "See[^n].\n\n[^n]: note text\n"
	d = parser.New(defaultExtensions)
	d.Set(footnoteSource)
	if _, ok := d.Footnote("n"); !ok {
		t.Fatal("incremental parser did not index the footnote definition")
	}
	footnoteDoc := goldmarkSyntaxDoc(footnoteSource)
	if len(footnoteDoc.Blocks) != 2 || footnoteDoc.Blocks[1].Kind != List || len(footnoteDoc.Blocks[1].Items) != 1 {
		t.Fatalf("Goldmark oracle footnote projection = %#v", footnoteDoc.Blocks)
	}
	for _, in := range d.Inlines(1) {
		if in.Kind == parser.FootnoteRef {
			t.Logf("incremental syntax retains authored label %q; public model emits a numbered display marker", in.Text)
			return
		}
	}
	t.Fatal("incremental parser did not expose a footnote reference")
}

func TestParserAuditHTMLConvenienceRewrites(t *testing.T) {
	tests := []struct {
		name   string
		source string
		kind   Kind
	}{
		{"HTML heading", "<h1>Title</h1>\n", Heading},
		{"HTML image", "<p align=\"center\"><img src=\"image.png\" alt=\"diagram\"></p>\n", Image},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Parse(tt.source)
			if len(doc.Blocks) != 1 || doc.Blocks[0].Kind != tt.kind {
				t.Fatalf("production syntax projection = %#v, want kind %v", doc.Blocks, tt.kind)
			}
		})
	}
}

func TestParserAuditStreamingOpenBlock(t *testing.T) {
	source := "unfinished *bold"
	incremental := parser.New(defaultExtensions)
	incremental.SetStreaming(true)
	incremental.Set(source)
	var open, healedEmphasis bool
	for i := range incremental.Children(0) {
		b := incremental.Block(i)
		open = open || b.Open
		in := incremental.Inlines(i)
		for _, n := range in {
			healedEmphasis = healedEmphasis || n.Kind == parser.Emphasis
		}
	}
	if !open || !healedEmphasis {
		t.Fatalf("incremental streaming state: open=%v healed emphasis=%v", open, healedEmphasis)
	}
	oracle := goldmarkSyntaxDoc(source)
	if len(oracle.Blocks) == 0 {
		t.Fatal("Goldmark oracle lost trailing open paragraph")
	}
	t.Log("Muse exposes streaming-open state; Goldmark's AST has no streaming-open contract")
}

func goldmarkSyntaxDoc(source string) *Doc {
	root := goldmarkParser.Parser().Parse(text.NewReader([]byte(CJKFriendly(source))))
	return &Doc{Source: source, Blocks: parseBlocks(root, []byte(source))}
}
