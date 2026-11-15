package syntax

import (
	"html"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// parser is shared: goldmark's parser is safe for concurrent use and holds
// no per-call state. Footnotes are how Typora writes a note's references;
// without the extension [^1]: … parses as a link definition and its text
// disappears from the model.
var parser = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.Footnote))

// Parse parses Markdown source into a document. It never fails: anything
// the parser cannot place is kept as a paragraph so no text is lost, which
// is what makes the same code path usable for plain .txt files.
func Parse(src string) *Doc {
	b := []byte(src)
	// Emphasis beside CJK punctuation reads as Typora reads it; the text
	// comes from the source itself (see CJKFriendly).
	root := parser.Parser().Parse(text.NewReader([]byte(CJKFriendly(src))))
	doc := &Doc{Source: src, Blocks: parseBlocks(root, b)}
	return doc
}

// ParseFile parses Markdown bytes, as read from disk.
func ParseFile(b []byte) *Doc { return Parse(string(b)) }

func parseBlocks(n ast.Node, src []byte) []Block {
	var out []Block
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if b, ok := parseBlock(c, src); ok {
			out = append(out, b)
		}
	}
	return out
}

func parseBlock(n ast.Node, src []byte) (Block, bool) {
	switch v := n.(type) {
	case *ast.Heading:
		return Block{Kind: Heading, Level: v.Level, Inlines: parseInlines(v, src, style{})}, true
	case *ast.Paragraph:
		if tex, ok := wholeMath(v, src); ok {
			return Block{Kind: Math, Text: tex}, true
		}
		if img, alt, ok := onlyImage(v, src); ok {
			return Block{Kind: Image, Src: string(img.Destination), Alt: alt, Title: string(img.Title)}, true
		}
		return Block{Kind: Paragraph, Inlines: parseInlines(v, src, style{})}, true
	case *ast.TextBlock:
		return Block{Kind: Paragraph, Inlines: parseInlines(v, src, style{})}, true
	case *ast.FencedCodeBlock:
		return Block{Kind: Code, Lang: string(v.Language(src)), Text: linesText(v, src)}, true
	case *ast.CodeBlock:
		return Block{Kind: Code, Text: linesText(v, src)}, true
	case *ast.Blockquote:
		return Block{Kind: Quote, Children: parseBlocks(v, src)}, true
	case *ast.List:
		return parseList(v, src), true
	case *ast.ThematicBreak:
		return Block{Kind: Rule}, true
	case *ast.HTMLBlock:
		html := linesText(v, src)
		if src, alt, title, w, h, ok := htmlOnlyImage(html); ok {
			return Block{Kind: Image, Src: src, Alt: alt, Title: title, ImgWidth: w, ImgHeight: h}, true
		}
		if level, text, ok := htmlOnlyHeading(html); ok {
			return Block{Kind: Heading, Level: level, Inlines: []Span{{Text: text}}}, true
		}
		return Block{Kind: HTML, Text: html}, true
	case *east.Table:
		return parseTable(v, src), true
	case *east.FootnoteList:
		return parseFootnoteList(v, src), true
	}
	// Unknown block: keep its text so nothing is lost.
	if t := nodeText(n, src); t != "" {
		return Block{Kind: Paragraph, Inlines: []Span{{Text: t}}}, true
	}
	return Block{}, false
}

func parseList(n *ast.List, src []byte) Block {
	b := Block{Kind: List, Ordered: n.IsOrdered(), Start: n.Start}
	num := n.Start
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		li, ok := c.(*ast.ListItem)
		if !ok {
			continue
		}
		item := parseItem(li, src)
		if n.IsOrdered() {
			item.Marker = strconv.Itoa(num) + "."
			num++
		} else {
			item.Marker = "•"
		}
		b.Items = append(b.Items, item)
	}
	return b
}

// parseFootnoteList maps footnote definitions to a list: each footnote is
// an item marked [n], its first paragraph the lead-in text and any further
// blocks its children. The shallow model has no footnote kind, and this
// keeps every definition readable in plain text and in the view.
func parseFootnoteList(n *east.FootnoteList, src []byte) Block {
	b := Block{Kind: List}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		fn, ok := c.(*east.Footnote)
		if !ok {
			continue
		}
		item := Item{Marker: "[" + strconv.Itoa(fn.Index) + "]"}
		for d := fn.FirstChild(); d != nil; d = d.NextSibling() {
			if _, ok := d.(*ast.Paragraph); ok && len(item.Inlines) == 0 {
				item.Inlines = parseInlines(d, src, style{})
				continue
			}
			if blk, ok := parseBlock(d, src); ok {
				item.Children = append(item.Children, blk)
			}
		}
		b.Items = append(b.Items, item)
	}
	return b
}

func parseItem(li *ast.ListItem, src []byte) Item {
	var item Item
	for c := li.FirstChild(); c != nil; c = c.NextSibling() {
		switch c.(type) {
		case *ast.Paragraph, *ast.TextBlock:
			if len(item.Inlines) == 0 {
				item.Task, item.Checked = taskState(c)
				item.Inlines = parseInlines(c, src, style{})
				continue
			}
		}
		if b, ok := parseBlock(c, src); ok {
			item.Children = append(item.Children, b)
		}
	}
	return item
}

// onlyImage reports the one image of a paragraph that holds nothing else,
// which is how a standalone image is drawn as a block.
func onlyImage(n ast.Node, src []byte) (*ast.Image, string, bool) {
	var img *ast.Image
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok && strings.TrimSpace(string(t.Segment.Value(src))) == "" {
			continue
		}
		if i, ok := c.(*ast.Image); ok && img == nil {
			img = i
			continue
		}
		return nil, "", false
	}
	if img == nil {
		return nil, "", false
	}
	return img, nodeText(img, src), true
}

// taskState finds a GFM task checkbox in a list item's first paragraph.
func taskState(n ast.Node) (task, checked bool) {
	_ = ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if cb, ok := node.(*east.TaskCheckBox); ok {
			task, checked = true, cb.IsChecked
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return task, checked
}

func parseTable(n *east.Table, src []byte) Block {
	b := Block{Kind: Table}
	for _, a := range n.Alignments {
		b.Align = append(b.Align, alignOf(a))
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch row := c.(type) {
		case *east.TableHeader:
			b.Rows = append(b.Rows, tableRowCells(row, src))
		case *east.TableRow:
			b.Rows = append(b.Rows, tableRowCells(row, src))
		}
	}
	return b
}

// htmlOnlyImage recognises an HTML block that holds nothing but one image, as
// READMEs often do with <p align="center"><img ...></p>, and turns it into a
// native image block. Text around the image makes it a normal HTML block.
func htmlOnlyImage(s string) (src, alt, title string, w, h float32, ok bool) {
	low := strings.ToLower(s)
	start := strings.Index(low, "<img")
	if start < 0 {
		return "", "", "", 0, 0, false
	}
	end := strings.Index(s[start:], ">")
	if end < 0 {
		return "", "", "", 0, 0, false
	}
	tag := s[start : start+end]
	src = HTMLAttribute(tag, "src")
	if src == "" {
		return "", "", "", 0, 0, false
	}
	w = cssSize(HTMLAttribute(tag, "width"))
	h = cssSize(HTMLAttribute(tag, "height"))
	// Everything outside the tag must be whitespace or other tags.
	rest := s[:start] + s[start+end+1:]
	for i := 0; i < len(rest); {
		if rest[i] == '<' {
			j := strings.IndexByte(rest[i:], '>')
			if j < 0 {
				return "", "", "", 0, 0, false
			}
			i += j + 1
			continue
		}
		if !strings.ContainsRune(" \t\r\n", rune(rest[i])) {
			return "", "", "", 0, 0, false
		}
		i++
	}
	return src, HTMLAttribute(tag, "alt"), HTMLAttribute(tag, "title"), w, h, true
}

// HTMLOnlyImage reports whether an HTML fragment contains only an image and
// surrounding tags/whitespace.
func HTMLOnlyImage(s string) (src, alt, title string, width, height float32, ok bool) {
	return htmlOnlyImage(s)
}

// cssSize reads an HTML width/height such as 128, "128px" or "12.5em".
func cssSize(v string) float32 {
	v = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(v), "px"))
	if v == "" {
		return 0
	}
	n, err := strconv.ParseFloat(v, 32)
	if err != nil || n < 0 {
		return 0
	}
	return float32(n)
}

// htmlOnlyHeading recognises a block that is just one h1–h6 tag, so an HTML
// README title keeps the heading scale instead of falling back to body text.
func htmlOnlyHeading(s string) (level int, text string, ok bool) {
	low := strings.ToLower(s)
	for l := 1; l <= 6; l++ {
		open := "<h" + strconv.Itoa(l)
		i := strings.Index(low, open)
		if i < 0 {
			continue
		}
		j := i + len(open)
		if j < len(s) && s[j] != '>' && s[j] != ' ' && s[j] != '\t' && s[j] != '\n' {
			continue
		}
		openEnd := strings.IndexByte(s[i:], '>')
		if openEnd < 0 {
			continue
		}
		bodyStart := i + openEnd + 1
		closeTag := "</h" + strconv.Itoa(l) + ">"
		closeAt := strings.Index(strings.ToLower(s[bodyStart:]), closeTag)
		if closeAt < 0 {
			continue
		}
		inner := s[bodyStart : bodyStart+closeAt]
		outside := s[:i] + s[bodyStart+closeAt+len(closeTag):]
		if strings.TrimSpace(HTMLText(outside)) != "" {
			continue
		}
		text = strings.TrimSpace(HTMLText(inner))
		if text == "" {
			continue
		}
		return l, text, true
	}
	return 0, "", false
}

// tableRowCells reads a row's cells. It accepts either a TableRow or a
// TableHeader, whose children are the cells directly.
func tableRowCells(n ast.Node, src []byte) []Cell {
	var cells []Cell
	for cell := n.FirstChild(); cell != nil; cell = cell.NextSibling() {
		if tc, ok := cell.(*east.TableCell); ok {
			cells = append(cells, Cell(parseInlines(tc, src, style{})))
		}
	}
	return cells
}

func alignOf(a east.Alignment) Align {
	switch a {
	case east.AlignLeft:
		return AlignLeft
	case east.AlignCenter:
		return AlignCenter
	case east.AlignRight:
		return AlignRight
	}
	return AlignNone
}

type style struct {
	bold, italic, code, strike bool
	link                       string
}

func parseInlines(n ast.Node, src []byte, st style) []Span {
	var out []Span
	var walk func(ast.Node, style)
	walk = func(node ast.Node, st style) {
		for c := node.FirstChild(); c != nil; c = c.NextSibling() {
			switch v := c.(type) {
			case *ast.Text:
				t := string(v.Segment.Value(src))
				if !v.IsRaw() {
					t = unescape(t)
				}
				out = appendSpan(out, Span{Text: t,
					Bold: st.bold, Italic: st.italic, Code: st.code, Strike: st.strike, Link: st.link})
				if v.SoftLineBreak() {
					out = appendSpan(out, Span{Text: "\n", SoftBreak: true, Bold: st.bold, Italic: st.italic, Link: st.link})
				}
				if v.HardLineBreak() {
					out = appendSpan(out, Span{Text: "\n", Bold: st.bold, Italic: st.italic, Link: st.link})
				}
			case *ast.String:
				out = appendSpan(out, Span{Text: string(v.Value),
					Bold: st.bold, Italic: st.italic, Code: st.code, Strike: st.strike, Link: st.link})
			case *ast.CodeSpan:
				out = appendSpan(out, Span{Text: nodeText(v, src), Code: true,
					Bold: st.bold, Italic: st.italic, Strike: st.strike, Link: st.link})
			case *ast.Emphasis:
				ns := st
				if v.Level == 2 {
					ns.bold = true
				} else {
					ns.italic = true
				}
				walk(v, ns)
			case *east.Strikethrough:
				ns := st
				ns.strike = true
				walk(v, ns)
			case *east.FootnoteLink:
				// The marker replaces the reference syntax; its definition
				// follows as a footnote item.
				out = appendSpan(out, Span{Text: "[" + strconv.Itoa(v.Index) + "]",
					Bold: st.bold, Italic: st.italic, Link: st.link})
			case *ast.Link:
				ns := st
				ns.link = string(v.Destination)
				walk(v, ns)
			case *ast.AutoLink:
				link := string(v.URL(src))
				// The parser leaves the mailto: prefix to the renderer;
				// the model carries the link target itself.
				if v.AutoLinkType == ast.AutoLinkEmail && !strings.HasPrefix(strings.ToLower(link), "mailto:") {
					link = "mailto:" + link
				}
				out = appendSpan(out, Span{Text: string(v.Label(src)), Link: link,
					Bold: st.bold, Italic: st.italic})
			case *ast.Image:
				out = appendSpan(out, Span{Text: "🖼 " + nodeText(v, src), Italic: true, Link: string(v.Destination)})
			case *ast.RawHTML:
				// Inline HTML: keep its readable text, drop the tags.
				for i := 0; i < v.Segments.Len(); i++ {
					seg := v.Segments.At(i)
					if txt := HTMLText(string(seg.Value(src))); strings.TrimSpace(txt) != "" {
						out = appendSpan(out, Span{Text: txt, Bold: st.bold, Italic: st.italic, Strike: st.strike, Link: st.link})
					}
				}
			default:
				walk(c, st)
			}
		}
	}
	walk(n, st)
	return splitMathSpans(out)
}

// wholeMath recognises a paragraph that is one display-math block.
func wholeMath(n ast.Node, src []byte) (string, bool) {
	raw := strings.TrimSpace(linesText(n, src))
	if len(raw) < 4 {
		return "", false
	}
	if strings.HasPrefix(raw, "$$") && strings.HasSuffix(raw, "$$") {
		inner := strings.TrimSpace(raw[2 : len(raw)-2])
		if inner != "" {
			return inner, true
		}
	}
	if strings.HasPrefix(raw, "\\[") && strings.HasSuffix(raw, "\\]") {
		inner := strings.TrimSpace(raw[2 : len(raw)-2])
		if inner != "" {
			return inner, true
		}
	}
	return "", false
}

// splitMathSpans finds inline $...$ runs in ordinary text spans. Code spans
// are left alone, and escaped dollars do not open math.
func splitMathSpans(in []Span) []Span {
	var out []Span
	for _, sp := range in {
		if sp.Code || sp.Math || sp.SoftBreak {
			out = append(out, sp)
			continue
		}
		text := sp.Text
		for {
			i := findDollar(text, 0)
			if i < 0 {
				if text != "" {
					out = appendSpan(out, Span{Text: text, Bold: sp.Bold, Italic: sp.Italic, Strike: sp.Strike, Link: sp.Link})
				}
				break
			}
			if i+1 < len(text) && text[i+1] == '$' {
				// Display math is handled at block level; keep a stray $$.
				out = appendSpan(out, Span{Text: text[:i+2], Bold: sp.Bold, Italic: sp.Italic, Strike: sp.Strike, Link: sp.Link})
				text = text[i+2:]
				continue
			}
			j := findDollar(text, i+1)
			if j < 0 {
				out = appendSpan(out, Span{Text: text, Bold: sp.Bold, Italic: sp.Italic, Strike: sp.Strike, Link: sp.Link})
				break
			}
			if i > 0 {
				out = appendSpan(out, Span{Text: text[:i], Bold: sp.Bold, Italic: sp.Italic, Strike: sp.Strike, Link: sp.Link})
			}
			inner := text[i+1 : j]
			if strings.TrimSpace(inner) == "" {
				out = appendSpan(out, Span{Text: text[i : j+1], Bold: sp.Bold, Italic: sp.Italic, Strike: sp.Strike, Link: sp.Link})
			} else {
				out = appendSpan(out, Span{Text: inner, Math: true, Bold: sp.Bold, Italic: sp.Italic, Strike: sp.Strike, Link: sp.Link})
			}
			text = text[j+1:]
		}
	}
	return out
}

func findDollar(s string, from int) int {
	for i := from; i < len(s); i++ {
		if s[i] == '$' && (i == 0 || s[i-1] != '\\') {
			return i
		}
	}
	return -1
}

// appendSpan adds s, merging it into the previous span when their styles
// match, so the model carries runs rather than characters.
func appendSpan(out []Span, s Span) []Span {
	if s.Text == "" {
		return out
	}
	if n := len(out); n > 0 {
		p := &out[n-1]
		if p.Bold == s.Bold && p.Italic == s.Italic && p.Code == s.Code &&
			p.Strike == s.Strike && p.Link == s.Link && p.Math == s.Math && p.SoftBreak == s.SoftBreak {
			p.Text += s.Text
			return out
		}
	}
	return append(out, s)
}

// nodeText is the plain text of a subtree, used where styling is dropped
// (code spans, image alt text).
func nodeText(n ast.Node, src []byte) string {
	var b []byte
	_ = ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := node.(type) {
		case *ast.Text:
			b = append(b, v.Segment.Value(src)...)
		case *ast.String:
			b = append(b, v.Value...)
		}
		return ast.WalkContinue, nil
	})
	return string(b)
}

// unescape resolves the backslash escapes and the character references of
// text, which goldmark leaves in its segments for its renderer to resolve.
func unescape(s string) string {
	if !strings.ContainsAny(s, `\&`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) && isASCIIPunct(s[i+1]) {
			b.WriteByte(s[i+1])
			i++
			continue
		}
		if c == '&' {
			if j := strings.IndexByte(s[i:], ';'); j > 1 && j <= 32 && isRefName(s[i+1:i+j]) {
				if r := html.UnescapeString(s[i : i+j+1]); r != s[i:i+j+1] {
					b.WriteString(r)
					i += j
					continue
				}
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

// isRefName reports whether s can be what a character reference holds
// between & and ;: a name, or # and a number.
func isRefName(s string) bool {
	for _, c := range []byte(strings.TrimPrefix(s, "#")) {
		if !(c >= '0' && c <= '9' || c|0x20 >= 'a' && c|0x20 <= 'z') {
			return false
		}
	}
	return true
}

// linesText is the raw text of a node's lines (code blocks, HTML blocks).
func linesText(n ast.Node, src []byte) string {
	var b []byte
	l := n.Lines()
	for i := 0; i < l.Len(); i++ {
		seg := l.At(i)
		b = append(b, seg.Value(src)...)
	}
	return string(b)
}
