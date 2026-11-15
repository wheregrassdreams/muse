package syntax

import (
	"strconv"
	"strings"

	parser "muse/markdown/syntax/internal/incremental"
)

const defaultExtensions = (parser.Default &^ parser.ExtReferences) | parser.ExtDollarMath

// parseIncremental is the syntax module's single grammar entry point. The
// underlying parser supports block-local edits; this function projects its
// semantic tree into the stable neutral Markdown model.
func parseIncremental(source string) *Doc {
	d := parser.New(defaultExtensions)
	d.Set(source)
	doc := FromIncremental(d)
	closeProjection(doc.Blocks)
	return doc
}

// FromIncremental projects the syntax-owned incremental parser into the
// backend-neutral Markdown model. The input remains owned by its caller.
func FromIncremental(parsed *parser.Document) *Doc {
	if parsed == nil {
		return nil
	}
	return &Doc{Source: parsed.Source(), Blocks: incrementalChildren(parsed, 0, footnoteNumbers(parsed))}
}

func footnoteNumbers(d *parser.Document) map[string]int {
	numbers := make(map[string]int)
	next := 1
	var visit func(int)
	visit = func(parent int) {
		for child := range d.Children(parent) {
			b := d.Block(child)
			if b.Kind == parser.Paragraph || b.Kind == parser.Heading || b.Kind == parser.TableCell {
				for _, in := range d.Inlines(child) {
					if in.Kind == parser.FootnoteRef {
						if _, ok := numbers[in.Text]; !ok {
							numbers[in.Text] = next
							next++
						}
					}
				}
			}
			visit(child)
		}
	}
	visit(0)
	return numbers
}

func incrementalChildren(d *parser.Document, parent int, footnotes map[string]int) []Block {
	var out []Block
	var footnoteItems []Item
	var footnoteStart, footnoteEnd int
	for child := range d.Children(parent) {
		if d.Block(child).Kind == parser.FootnoteDefinition && parent == 0 {
			item, ok := incrementalFootnoteItem(d, child, footnotes)
			if ok {
				if len(footnoteItems) == 0 {
					footnoteStart = d.Block(child).Start
				}
				footnoteEnd = d.Block(child).End
				footnoteItems = append(footnoteItems, item)
			}
			continue
		}
		block, ok := incrementalBlock(d, child, footnotes)
		if ok {
			out = append(out, block)
		}
	}
	if len(footnoteItems) > 0 {
		out = append(out, Block{Kind: List, Items: footnoteItems, SourceStart: footnoteStart, SourceEnd: footnoteEnd})
	}
	return out
}

func closeProjection(blocks []Block) {
	for i := range blocks {
		blocks[i].Open = false
		blocks[i].Healed = false
		closeProjection(blocks[i].Children)
		for j := range blocks[i].Items {
			closeProjection(blocks[i].Items[j].Children)
		}
	}
}

func incrementalBlock(d *parser.Document, index int, footnotes map[string]int) (Block, bool) {
	b := d.Block(index)
	base := Block{SourceStart: b.Start, SourceEnd: b.End, Open: b.Open, Healed: b.Healed(), Alert: Alert(b.Alert)}
	switch b.Kind {
	case parser.Paragraph:
		in := d.Inlines(index)
		if src, alt, title, width, height, ok := inlineOnlyImage(in); ok {
			base.Kind, base.Src, base.Alt, base.Title = Image, src, alt, title
			base.ImgWidth, base.ImgHeight = width, height
			return base, true
		}
		base.Kind = Paragraph
		base.Inlines = incrementalInlines(in, 0, len(in), inlineStyle{}, footnotes)
		return base, true
	case parser.Heading:
		in := d.Inlines(index)
		base.Kind, base.Level = Heading, b.Level
		base.Inlines = incrementalInlines(in, 0, len(in), inlineStyle{}, footnotes)
		return base, true
	case parser.CodeBlock:
		base.Kind, base.Lang, base.Text = Code, b.Language(), d.Lines(index)
		return base, true
	case parser.HTMLBlock:
		raw := d.Lines(index)
		if src, alt, title, width, height, ok := htmlOnlyImage(raw); ok {
			base.Kind, base.Src, base.Alt, base.Title = Image, src, alt, title
			base.ImgWidth, base.ImgHeight = width, height
			return base, true
		}
		if level, text, ok := htmlOnlyHeading(raw); ok {
			base.Kind, base.Level = Heading, level
			base.Inlines = []Span{{Text: text, SourceStart: b.Start, SourceEnd: b.End}}
			return base, true
		}
		base.Kind, base.Text = HTML, raw
		return base, true
	case parser.Quote:
		base.Kind = Quote
		for child := range d.Children(index) {
			if base.Alert != NoAlert && d.Block(child).Kind == parser.Paragraph && len(d.Inlines(child)) == 0 {
				continue // the standalone [!NOTE] marker is a model property
			}
			if block, ok := incrementalBlock(d, child, footnotes); ok {
				base.Children = append(base.Children, block)
			}
		}
		return base, true
	case parser.List:
		base.Kind, base.Ordered, base.Start = List, b.Ordered, b.Number
		for child := range d.Children(index) {
			itemBlock := d.Block(child)
			if itemBlock.Kind != parser.Item {
				continue
			}
			item := Item{Task: itemBlock.Task != parser.NoTask, Checked: itemBlock.Task == parser.Checked, SourceStart: itemBlock.Start, SourceEnd: itemBlock.End, Marker: "•"}
			if itemBlock.Ordered {
				item.Marker = strconv.Itoa(itemBlock.Number) + "."
			}
			for itemChild := range d.Children(child) {
				itemChildBlock := d.Block(itemChild)
				if itemChildBlock.Kind == parser.Paragraph && len(item.Inlines) == 0 {
					in := d.Inlines(itemChild)
					item.Inlines = incrementalInlines(in, 0, len(in), inlineStyle{}, footnotes)
					continue
				}
				if nested, ok := incrementalBlock(d, itemChild, footnotes); ok {
					item.Children = append(item.Children, nested)
				}
			}
			base.Items = append(base.Items, item)
		}
		return base, true
	case parser.ThematicBreak:
		base.Kind = Rule
		return base, true
	case parser.Table:
		base.Kind = Table
		for _, align := range b.Align {
			base.Align = append(base.Align, Align(align))
		}
		for row := range d.Children(index) {
			var cells []Cell
			for cell := range d.Children(row) {
				in := d.Inlines(cell)
				cells = append(cells, Cell(incrementalInlines(in, 0, len(in), inlineStyle{}, footnotes)))
			}
			base.Rows = append(base.Rows, cells)
		}
		return base, true
	case parser.MathBlock:
		base.Kind, base.Text = Math, strings.TrimSpace(d.Lines(index))
		return base, true
	case parser.FootnoteDefinition:
		return Block{}, false
	default:
		return Block{}, false
	}
}

func incrementalFootnoteItem(d *parser.Document, index int, footnotes map[string]int) (Item, bool) {
	b := d.Block(index)
	number := footnotes[b.Label()]
	if number == 0 {
		return Item{}, false
	}
	item := Item{Marker: "[" + strconv.Itoa(number) + "]", SourceStart: b.Start, SourceEnd: b.End}
	for child := range d.Children(index) {
		if d.Block(child).Kind == parser.Paragraph && len(item.Inlines) == 0 {
			in := d.Inlines(child)
			item.Inlines = incrementalInlines(in, 0, len(in), inlineStyle{}, footnotes)
			continue
		}
		if nested, ok := incrementalBlock(d, child, footnotes); ok {
			item.Children = append(item.Children, nested)
		}
	}
	return item, true
}

type inlineStyle struct {
	bold, italic, code, strike bool
	link                       string
}

func incrementalInlines(in []parser.Inline, from, to int, style inlineStyle, footnotes map[string]int) []Span {
	var out []Span
	appendSpan := func(span Span) {
		if span.Text == "" && !span.SoftBreak {
			return
		}
		if len(out) > 0 {
			prev := &out[len(out)-1]
			if prev.Bold == span.Bold && prev.Italic == span.Italic && prev.Code == span.Code && prev.Strike == span.Strike && prev.Link == span.Link && prev.Math == span.Math && prev.SoftBreak == span.SoftBreak && prev.Mention == span.Mention && prev.Reference == span.Reference {
				prev.Text += span.Text
				prev.SourceEnd = span.SourceEnd
				return
			}
		}
		out = append(out, span)
	}
	for i := from; i < to; {
		n := in[i]
		span := Span{Bold: style.bold, Italic: style.italic, Code: style.code, Strike: style.strike, Link: style.link, SourceStart: n.Start, SourceEnd: n.End}
		switch n.Kind {
		case parser.Text:
			span.Text = n.Text
			appendSpan(span)
		case parser.Mention:
			span.Text, span.Mention = n.Text, true
			appendSpan(span)
		case parser.Reference:
			span.Text, span.Reference = n.Text, true
			appendSpan(span)
		case parser.SoftBreak:
			span.Text, span.SoftBreak = "\n", true
			appendSpan(span)
		case parser.LineBreak:
			span.Text = "\n"
			appendSpan(span)
		case parser.Code:
			span.Text, span.Code = n.Text, true
			appendSpan(span)
		case parser.Math:
			span.Text, span.Math = n.Text, true
			appendSpan(span)
		case parser.HTML:
			span.Text = HTMLText(n.Text)
			appendSpan(span)
		case parser.FootnoteRef:
			span.Text = "[" + strconv.Itoa(footnotes[n.Text]) + "]"
			appendSpan(span)
		case parser.Image:
			alt := incrementalPlainText(in, i+1, n.Next)
			span.Text, span.Italic, span.Link = "🖼 "+alt, true, n.Dest
			appendSpan(span)
			i = n.Next
			continue
		case parser.Emphasis, parser.Strong, parser.Strikethrough, parser.Link:
			nested := style
			switch n.Kind {
			case parser.Emphasis:
				nested.italic = true
			case parser.Strong:
				nested.bold = true
			case parser.Strikethrough:
				nested.strike = true
			case parser.Link:
				nested.link = n.Dest
			}
			out = append(out, incrementalInlines(in, i+1, n.Next, nested, footnotes)...)
			i = n.Next
			continue
		}
		if n.Next > i {
			i = n.Next
		} else {
			i++
		}
	}
	return out
}

func incrementalPlainText(in []parser.Inline, from, to int) string {
	var b strings.Builder
	for i := from; i < to; {
		n := in[i]
		switch n.Kind {
		case parser.Text, parser.Code, parser.HTML, parser.Math, parser.Mention, parser.Reference:
			b.WriteString(n.Text)
		case parser.SoftBreak, parser.LineBreak:
			b.WriteByte('\n')
		}
		if n.Next > i {
			i = n.Next
		} else {
			i++
		}
	}
	return b.String()
}

func inlineOnlyImage(in []parser.Inline) (src, alt, title string, width, height float32, ok bool) {
	first := 0
	for first < len(in) && (in[first].Kind == parser.Text && strings.TrimSpace(in[first].Text) == "") {
		first = in[first].Next
	}
	if first >= len(in) || in[first].Kind != parser.Image || in[first].Next != len(in) {
		return "", "", "", 0, 0, false
	}
	n := in[first]
	return n.Dest, incrementalPlainText(in, first+1, n.Next), n.Title, 0, 0, true
}
