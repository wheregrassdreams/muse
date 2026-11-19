// Package projection turns Markdown syntax into detached editor block values.
package projection

import (
	textnav "github.com/wheregrassdreams/muse-editor/text/navigation"
	sourceview "github.com/wheregrassdreams/muse-editor/markdown/source"
	syntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"
	"strings"
)

type Kind uint8

const (
	Paragraph Kind = iota
	Heading
	Code
	Math
	HTML
	Rule
	TableBlock
	Raw
	Phantom
	Blank
)

type ContainerKind uint8

const (
	Quote ContainerKind = iota
	Item
	Footnote
)

type Container struct {
	Kind      ContainerKind
	Alert     syntax.IncrementalAlert
	Title     string
	ID, Start int
	Ordered   bool
	Number    int
	Marker    byte
	Tight     bool
	Depth     int
	Task      syntax.IncrementalTask
	TaskAt    int
	Label     string
}
type SourceLine struct {
	Offset int
	Text   string
}
type BlockDetails struct {
	Lines              []SourceLine
	Language           string
	FenceEnd           int
	InfoStart, InfoEnd int
}
type Table struct {
	Rows      [][]*Block
	Align     []syntax.IncrementalAlign
	Delimiter [2]int
}
type Block struct {
	Kind                                        Kind
	Fenced, Pending, Setext, FrontMatter, Whole bool
	ID, Start, End, LineStart, SyntaxStart      int
	Level                                       int
	Containers                                  []Container
	FirstOf                                     []int
	Table                                       *Table
	TableOwner                                  *Block
	Details                                     *BlockDetails
}
type leaf = Block
type cont = Container
type line = SourceLine
type table = Table
type leafDetails = BlockDetails
type leafKind = Kind

func (b *Block) detail() *BlockDetails {
	if b == nil {
		return nil
	}
	return b.Details
}
func (b *Block) ensureDetails() *BlockDetails {
	if b.Details == nil {
		b.Details = &BlockDetails{}
	}
	return b.Details
}
func setConts(b *Block, containers []Container) []Container {
	b.Containers = append(b.Containers[:0], containers...)
	return b.Containers
}

func quoteContainer(src string, id int, block syntax.IncrementalBlock) Container {
	c := Container{Kind: Quote, ID: id, Start: block.Start, Alert: block.Alert}
	line := src[block.Start:]
	if n := strings.IndexByte(line, '\n'); n >= 0 {
		line = line[:n]
	}
	line = strings.TrimLeft(line, " >\t")
	if strings.HasPrefix(line, "[!") {
		if n := strings.IndexByte(line, ']'); n >= 0 {
			for alert := syntax.IncrementalAlertNote; alert <= syntax.IncrementalAlertCaution; alert++ {
				if strings.EqualFold(line[2:n], alert.String()) {
					c.Alert = alert
					c.Title = strings.TrimSpace(line[n+1:])
					break
				}
			}
		}
	}
	if c.Title == "" {
		c.Title = c.Alert.String()
	}
	return c
}

func isCalloutContainer(c Container) bool {
	return c.Kind == Quote && c.Alert != syntax.IncrementalNoAlert
}

// QuoteContainer projects parser quote facts and an optional alert title.
func QuoteContainer(src string, id int, block syntax.IncrementalBlock) Container {
	return quoteContainer(src, id, block)
}

// IsCalloutContainer reports whether a projected container is an alert quote.
func IsCalloutContainer(c Container) bool { return isCalloutContainer(c) }

// Projector reuses its block arena across source revisions. Returned blocks
// are borrowed until the next Build call; consumers should copy only host state
// they need to retain.
type Projector struct{ arena leafArena }

func (p *Projector) Build(src string, d sourceview.SyntaxView, frontMatterEnd int) []*Block {
	if p == nil {
		return nil
	}
	return addBlankLeaves(&p.arena, src, buildLeaves(src, d, frontMatterEnd, &p.arena))
}
func (p *Projector) Release() {
	if p != nil {
		p.arena.release()
	}
}

type leafArena struct {
	chunks [][]Block
	n      int
}

const leafArenaChunkSize = 256

func (a *leafArena) reset() {
	for i := 0; i < a.n; i++ {
		b := a.blockAt(i)
		clear(b.Containers)
		b.Containers = b.Containers[:0]
		clear(b.FirstOf)
		b.FirstOf = b.FirstOf[:0]
		b.Table = nil
		if d := b.Details; d != nil {
			clear(d.Lines)
			d.Lines = d.Lines[:0]
			d.Language = ""
			d.FenceEnd, d.InfoStart, d.InfoEnd = 0, 0, 0
		}
	}
	a.n = 0
}
func (a *leafArena) release() { a.reset(); a.chunks = nil }
func (a *leafArena) get() *Block {
	ci := a.n / leafArenaChunkSize
	if ci == len(a.chunks) {
		a.chunks = append(a.chunks, make([]Block, leafArenaChunkSize))
	}
	b := a.blockAt(a.n)
	a.n++
	containers, firstOf, details := b.Containers[:0], b.FirstOf[:0], b.Details
	*b = Block{}
	b.Containers, b.FirstOf, b.Details = containers, firstOf, details
	return b
}
func (a *leafArena) blockAt(i int) *Block {
	return &a.chunks[i/leafArenaChunkSize][i%leafArenaChunkSize]
}

func lineStartAt(src string, off int) int {
	off = min(max(off, 0), len(src))
	return strings.LastIndexByte(src[:off], '\n') + 1
}

// lineEndAt returns the offset of the end of the line holding off, before
// its newline.
func lineEndAt(src string, off int) int {
	off = min(max(off, 0), len(src))
	if i := strings.IndexByte(src[off:], '\n'); i >= 0 {
		return off + i
	}
	return len(src)
}

// frontMatterEnd returns where the YAML front matter at the top of src
// ends, after its closing line, or 0 when it has none: a first line of
// ---, up to a line of --- or ....
func frontMatterEnd(src string) int {
	if !strings.HasPrefix(src, "---\n") && !strings.HasPrefix(src, "---\r\n") {
		return 0
	}
	for off := strings.IndexByte(src, '\n') + 1; off < len(src); {
		end := lineEndAt(src, off)
		l := strings.TrimRight(src[off:end], " \t\r")
		if l == "---" || l == "..." {
			if off == 4 || off == 5 {
				// An empty front matter is two rules: leave them be.
				return 0
			}
			return end
		}
		off = end + 1
	}
	return 0
}

// hideFrontMatterEnd includes empty front matter's delimiters for the
// display-only projection. frontMatterEnd deliberately leaves `---\n---`
// as two horizontal rules in the default editing mode.
func hideFrontMatterEnd(src string) int {
	if end := frontMatterEnd(src); end > 0 {
		return end
	}
	if !strings.HasPrefix(src, "---\n") && !strings.HasPrefix(src, "---\r\n") {
		return 0
	}
	off := strings.IndexByte(src, '\n') + 1
	if off != 4 && off != 5 {
		return 0
	}
	end := lineEndAt(src, off)
	if line := strings.TrimRight(src[off:end], " \t\r"); line == "---" || line == "..." {
		return end
	}
	return 0
}

// maskFrontMatter returns src with its front matter blanked out, its
// newlines kept, so that the parser reads blank lines there and every
// offset after it stays where it was.
func maskFrontMatter(src string, end int) string {
	if end == 0 {
		return src
	}
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < end; i++ {
		switch src[i] {
		case '\n', '\r':
			b.WriteByte(src[i])
		default:
			b.WriteByte(' ')
		}
	}
	b.WriteString(src[end:])
	return b.String()
}

// escapedAt reports whether the character at i is escaped by a backslash.
func escapedAt(s string, i int) bool {
	n := 0
	for i-n-1 >= 0 && s[i-n-1] == '\\' {
		n++
	}
	return n%2 == 1
}

func buildLeaves(src string, d sourceview.SyntaxView, fm int, a *leafArena) []*leaf {
	// out grows through the arena's leaves; its length is the output's.
	a.reset()
	var out []*leaf
	if fm > 0 {
		out = append(out, rawLeaf(a, src, 0, fm, true))
	}
	var walk func(i int, conts []cont, listDepth int)
	walk = func(i int, conts []cont, listDepth int) {
		prevEnd := fm
		// The children of block i, walked by the block's own Next: the
		// iterator Document.Children returns allocates a closure on every
		// call, and this walk visits every block of the document.
		for c, end := i+1, d.Block(i).Next; c < end; c = d.Block(c).Next {
			b := d.Block(c)
			if i == 0 {
				// Source between top-level blocks that no block holds, as a
				// link reference definition, shows as raw lines.
				out = appendRaw(a, out, src, prevEnd, b.Start)
				prevEnd = max(prevEnd, spanEnd(src, d, c))
			}
			switch b.Kind {
			case syntax.IncrementalQuote:
				first := len(out)
				n := len(conts)
				conts = append(conts, quoteContainer(src, c, b))
				ct := conts[len(conts)-1]
				if isCalloutContainer(ct) && b.Alert != syntax.IncrementalNoAlert {
					h := rawLeaf(a, src, lineStartAt(src, b.Start), lineEndAt(src, b.Start), false)
					setConts(h, conts)
					out = append(out, h)
				}
				walk(c, conts, listDepth)
				conts = conts[:n]
				if len(out) == first {
					out = append(out, emptyLeaf(a, src, b, conts, quoteContainer(src, c, b)))
				}
				markFirst(out[first:], len(conts))
			case syntax.IncrementalList:
				n := 0
				for it, end := c+1, d.Block(c).Next; it < end; it = d.Block(it).Next {
					ib := d.Block(it)
					ct := cont{
						Kind: Item, ID: it, Start: ib.Start,
						Ordered: b.Ordered, Number: b.Number + n, Marker: b.Marker, Tight: b.Tight,
						Depth: listDepth, Task: ib.Task, TaskAt: -1,
					}
					if ib.Task != syntax.IncrementalNoTask {
						ct.TaskAt = taskBoxAt(src, ib.Start)
					}
					first := len(out)
					depth := len(conts)
					conts = append(conts, ct)
					walk(it, conts, listDepth+1)
					conts = conts[:depth]
					if len(out) == first {
						out = append(out, emptyItemLeaf(a, src, ib, conts, ct))
					}
					markFirst(out[first:], len(conts))
					n++
				}
			case syntax.IncrementalFootnoteDefinition:
				first := len(out)
				ct := cont{Kind: Footnote, ID: c, Start: b.Start, Label: b.Label()}
				depth := len(conts)
				conts = append(conts, ct)
				walk(c, conts, listDepth)
				conts = conts[:depth]
				if len(out) == first {
					out = append(out, emptyLeaf(a, src, b, conts, ct))
				}
				markFirst(out[first:], len(conts))
			case syntax.IncrementalParagraph:
				l := a.get()
				l.Kind, l.ID, l.Start, l.End = Paragraph, c, b.Start, paraEnd(src, b.End)
				l.LineStart, l.SyntaxStart = lineStartAt(src, b.Start), b.Start
				setConts(l, conts)
				out = append(out, l)
			case syntax.IncrementalHeading:
				out = append(out, headingLeaf(a, src, d, c, conts))
			case syntax.IncrementalThematicBreak:
				l := a.get()
				l.Kind, l.ID, l.Start, l.End = Rule, c, b.Start, b.End
				l.LineStart, l.SyntaxStart = lineStartAt(src, b.Start), b.Start
				setConts(l, conts)
				out = append(out, l)
			case syntax.IncrementalCodeBlock:
				out = append(out, codeLeaf(a, src, d, c, conts))
			case syntax.IncrementalMathBlock:
				out = append(out, linesLeaf(a, src, d, c, Math, conts))
			case syntax.IncrementalHTMLBlock:
				out = append(out, linesLeaf(a, src, d, c, HTML, conts))
			case syntax.IncrementalTable:
				out = append(out, tableLeaf(a, src, d, c, conts))
			}
		}
		if i == 0 {
			out = appendRaw(a, out, src, prevEnd, len(src))
		}
	}
	walk(0, nil, 0)
	return out
}

// markFirst records that the first of leaves opens the container at depth,
// unless the leaf is outside it: a marker still being typed.
func markFirst(leaves []*leaf, depth int) {
	if len(leaves) > 0 && depth < len(leaves[0].Containers) {
		leaves[0].FirstOf = append(leaves[0].FirstOf, depth)
	}
}

// paraEnd extends a paragraph's end over the spaces after its last
// character on its line, which the parser leaves out: a space just typed
// at the end of a paragraph shows, and the caret after it.
func paraEnd(src string, end int) int {
	for end < len(src) && (src[end] == ' ' || src[end] == '\t') {
		end++
	}
	return end
}

// headingLeaf makes the leaf of a heading: an ATX heading's text after its
// #s, or a setext heading's text above its underline.
func headingLeaf(a *leafArena, src string, d sourceview.SyntaxView, c int, conts []cont) *leaf {
	b := d.Block(c)
	l := a.get()
	l.Kind, l.ID, l.Level = Heading, c, b.Level
	l.LineStart, l.SyntaxStart = lineStartAt(src, b.Start), b.Start
	setConts(l, conts)
	// An ATX heading's line starts with its #s, after the indentation.
	p := b.Start
	for p < len(src) && src[p] == ' ' {
		p++
	}
	if p < len(src) && src[p] == '#' {
		marks := p
		for p < len(src) && src[p] == '#' {
			p++
		}
		hashes := p - marks
		if p >= len(src) || src[p] == '\n' || src[p] == '\r' {
			// # alone: still being typed.
			l.Pending = true
			l.Start, l.End = marks, p
			_ = hashes
			return l
		}
		for p < len(src) && (src[p] == ' ' || src[p] == '\t') {
			p++
		}
		l.Start = p
		if ins := d.Inlines(c); len(ins) > 0 {
			l.End = max(b.End, p)
			if !closingHashes(src[l.End:lineEndAt(src, l.End)]) {
				l.End = paraEnd(src, l.End)
			}
		} else {
			l.End = max(p, lineEndAt(src, p))
		}
		if l.End < l.Start {
			l.End = l.Start
		}
		return l
	}
	// A setext heading: its text is its paragraph's.
	l.Setext = true
	l.Start, l.End = b.Start, b.End
	return l
}

// closingHashes reports whether the rest of a heading's line after its
// text is a closing sequence of #s, which the text does not run into.
func closingHashes(rest string) bool {
	t := strings.TrimSpace(rest)
	return t != "" && strings.Trim(t, "#") == "" && (rest[0] == ' ' || rest[0] == '\t')
}

// codeLeaf makes the leaf of a fenced or indented block of code: its
// lines, and its fence's info string.
func codeLeaf(a *leafArena, src string, d sourceview.SyntaxView, c int, conts []cont) *leaf {
	b := d.Block(c)
	l := linesLeaf(a, src, d, c, Code, conts)
	l.Fenced = b.Fenced
	if b.Fenced {
		details := l.ensureDetails()
		// The opening fence: ``` or ~~~ and the info string after it.
		p := b.Start
		for p < len(src) && src[p] == ' ' {
			p++
		}
		fc := byte('`')
		if p < len(src) {
			fc = src[p]
		}
		for p < len(src) && src[p] == fc {
			p++
		}
		details.FenceEnd = p
		e := lineEndAt(src, p)
		details.InfoStart, details.InfoEnd = p, e
		for details.InfoStart < e && (src[details.InfoStart] == ' ' || src[details.InfoStart] == '\t') {
			details.InfoStart++
		}
		for details.InfoEnd > details.InfoStart && (src[details.InfoEnd-1] == ' ' || src[details.InfoEnd-1] == '\t' || src[details.InfoEnd-1] == '\r') {
			details.InfoEnd--
		}
		details.Language = b.Language()
	}
	return l
}

// linesLeaf makes the leaf of a block of Lines: code, math or HTML.
func linesLeaf(a *leafArena, src string, d sourceview.SyntaxView, c int, kind leafKind, conts []cont) *leaf {
	b := d.Block(c)
	l := a.get()
	l.Kind, l.ID = kind, c
	l.LineStart, l.SyntaxStart = lineStartAt(src, b.Start), b.Start
	setConts(l, conts)
	n := d.LineCount(c)
	details := l.ensureDetails()
	for k := 0; k < n; k++ {
		text, off := d.Line(c, k)
		// The parser's line is of the source it was given, which may have
		// characters swapped (see Editor.mask): the line is the source's.
		text = src[off : off+len(sourceLine(d.Source(), text, off))]
		details.Lines = append(details.Lines, line{Offset: off, Text: strings.TrimSuffix(text, "\r")})
	}
	if kind == Math || (kind == Code && b.Fenced) {
		// A fenced block's content lines end before its closing fence; a
		// block opened without content gets the line after its fence, so
		// the caret has somewhere to be.
		if len(details.Lines) == 0 {
			open := lineEndAt(src, b.Start)
			l.Start, l.End = open, open
			details.Lines = details.Lines[:0]
			return l
		}
	}
	if len(details.Lines) == 0 {
		l.Start, l.End = b.Start, b.Start
		return l
	}
	l.Start = details.Lines[0].Offset
	last := details.Lines[len(details.Lines)-1]
	l.End = last.Offset + len(last.Text)
	return l
}

// sourceLine returns the source of a line the parser gives as text from
// off. A tab that the indentation of a container took part of leaves the
// rest of its width as spaces before the text, which are not in the
// source: they are left out, so that the line is the source's bytes.
func sourceLine(src, text string, off int) string {
	for p := 0; p < tabStop && p <= len(text); p++ {
		if p > 0 && text[p-1] != ' ' {
			break
		}
		if e := off + len(text) - p; e <= len(src) && src[off:e] == text[p:] {
			return text[p:]
		}
	}
	return src[off:min(off+len(text), lineEndAt(src, off))]
}

// tabStop is the width of a tab, in columns.
const tabStop = 4

// spanEnd returns where the source of block c ends: the end of its last
// line, past what the parser's End leaves out (a table's last pipe, a
// heading's closing #s or its underline, the closing $$ of math), and of
// the last line of its descendants.
func spanEnd(src string, d sourceview.SyntaxView, c int) int {
	b := d.Block(c)
	end := lineEndAt(src, b.End)
	switch b.Kind {
	case syntax.IncrementalHeading:
		p := b.Start
		for p < len(src) && (src[p] == ' ' || src[p] == '\t') {
			p++
		}
		if (p >= len(src) || src[p] != '#') && end < len(src) {
			// A setext heading's underline.
			end = lineEndAt(src, end+1)
		}
	case syntax.IncrementalMathBlock:
		if !b.Open {
			// The closing $$ (or \]) ends the content's line, or the next.
			if rest := src[b.End:end]; !strings.Contains(rest, "$$") && !strings.Contains(rest, `\]`) && end < len(src) {
				next := lineEndAt(src, end+1)
				if t := textnav.TrimBlockPrefix(src[end+1 : next]); strings.HasPrefix(t, "$$") || strings.HasPrefix(t, `\]`) {
					end = next
				}
			}
		}
	}
	for ch, last := c+1, d.Block(c).Next; ch < last; ch = d.Block(ch).Next {
		end = max(end, spanEnd(src, d, ch))
	}
	return end
}

// tableLeaf makes the leaf of a table: its rows of cells.
func tableLeaf(a *leafArena, src string, d sourceview.SyntaxView, c int, conts []cont) *leaf {
	b := d.Block(c)
	t := &table{Align: b.Align}
	l := a.get()
	l.Kind, l.ID = TableBlock, c
	l.Start, l.End, l.LineStart, l.SyntaxStart = b.Start, b.End, lineStartAt(src, b.Start), b.Start
	l.Table = t
	setConts(l, conts)
	for r, rowsEnd := c+1, d.Block(c).Next; r < rowsEnd; r = d.Block(r).Next {
		var row []*leaf
		for cell, cellsEnd := r+1, d.Block(r).Next; cell < cellsEnd; cell = d.Block(cell).Next {
			cb := d.Block(cell)
			start, end := cb.Start, cb.End
			if start == end {
				// An empty cell's text goes after the space that follows its
				// pipe, not against the pipe that closes it.
				start, end = emptyCellAt(src, start), emptyCellAt(src, start)
			}
			cl := a.get()
			cl.Kind, cl.ID, cl.Start, cl.End = Paragraph, cell, start, end
			cl.LineStart, cl.SyntaxStart, cl.TableOwner = lineStartAt(src, cb.Start), start, l
			row = append(row, cl)
		}
		t.Rows = append(t.Rows, row)
	}
	// The delimiter row is the line after the header's.
	h := lineEndAt(src, b.Start)
	if h < len(src) {
		t.Delimiter = [2]int{h + 1, lineEndAt(src, h+1)}
	}
	return l
}

// emptyCellAt returns where the text of an empty cell goes, for the parser's
// position of it at: one space after the pipe before it.
func emptyCellAt(src string, at int) int {
	ls := lineStartAt(src, at)
	q := at - 1
	for q >= ls && src[q] != '|' {
		if src[q] != ' ' && src[q] != '\t' {
			return at
		}
		q--
	}
	if q < ls {
		return at
	}
	if q+1 < at && src[q+1] == ' ' {
		return q + 2
	}
	return q + 1
}

// emptyItemLeaf makes the leaf of a list item without content, "- " or
// "1. ", for the caret to type in after its marker.
func emptyItemLeaf(a *leafArena, src string, ib syntax.IncrementalBlock, conts []cont, ct cont) *leaf {
	p := ib.Start
	for p < len(src) && src[p] != ' ' && src[p] != '\t' && src[p] != '\n' && src[p] != '\r' {
		p++
	}
	l := a.get()
	l.Kind, l.ID, l.LineStart = Paragraph, -1, lineStartAt(src, ib.Start)
	l.Containers = append(l.Containers[:0], conts...)
	l.Containers = append(l.Containers, ct)
	if p >= len(src) || src[p] == '\n' || src[p] == '\r' {
		// "-" alone: a marker still being typed, which shows as text,
		// outside the item it would make.
		l.Pending = true
		l.Containers = setConts(l, conts)
		l.Start, l.End = ib.Start, p
		l.SyntaxStart = ib.Start
		return l
	}
	p++ // the space after the marker
	if ct.TaskAt >= 0 {
		p = max(p, ct.TaskAt+4)
	}
	p = min(p, lineEndAt(src, ib.Start))
	l.Start, l.End = p, max(p, lineEndAt(src, p))
	l.SyntaxStart = p
	return l
}

// emptyLeaf makes the leaf of a quotation or footnote without content.
func emptyLeaf(a *leafArena, src string, b syntax.IncrementalBlock, conts []cont, ct cont) *leaf {
	e := lineEndAt(src, b.Start)
	p := b.Start
	if ct.Kind == Quote {
		p++ // past >
		if p < e && src[p] == ' ' {
			p++
		}
	} else if i := strings.Index(src[b.Start:e], "]:"); i >= 0 {
		p = b.Start + i + 2
		for p < e && src[p] == ' ' {
			p++
		}
	}
	l := a.get()
	l.Kind, l.ID, l.LineStart = Paragraph, -1, lineStartAt(src, b.Start)
	l.Containers = append(l.Containers[:0], conts...)
	l.Containers = append(l.Containers, ct)
	if ct.Kind == Quote && p == b.Start+1 {
		// ">" alone: still being typed, and shown as text outside the
		// quotation it would make.
		l.Pending = true
		l.Containers = setConts(l, conts)
		l.Start, l.End = b.Start, p
		l.SyntaxStart = b.Start
		return l
	}
	l.Start, l.End = min(p, e), e
	l.SyntaxStart = l.Start
	return l
}

// codeDocLeaf makes the one leaf of a document that is code: all its
// lines, highlighted as lang.
func codeDocLeaf(a *leafArena, src, lang string) *leaf {
	l := rawLeaf(a, src, 0, len(src), false)
	l.Kind = Code
	l.Whole = true
	details := l.ensureDetails()
	if lang != "text" {
		details.Language = lang
	}
	if len(details.Lines) == 0 {
		details.Lines = []line{{Offset: 0}}
	}
	if n := len(details.Lines); n > 0 && len(src) > 0 && src[len(src)-1] == '\n' && details.Lines[n-1].Offset+len(details.Lines[n-1].Text) < len(src) {
		// The line after the last newline, for the caret.
		details.Lines = append(details.Lines, line{Offset: len(src)})
	}
	l.Start = details.Lines[0].Offset
	last := details.Lines[len(details.Lines)-1]
	l.End = last.Offset + len(last.Text)
	return l
}

// rawLeaf makes a leaf of the lines of src from start to end.
func rawLeaf(a *leafArena, src string, start, end int, front bool) *leaf {
	l := a.get()
	l.Kind, l.ID, l.LineStart, l.SyntaxStart, l.FrontMatter = Raw, -1, start, start, front
	details := l.ensureDetails()
	for off := start; off <= end && off < len(src); {
		e := min(lineEndAt(src, off), end)
		details.Lines = append(details.Lines, line{Offset: off, Text: strings.TrimSuffix(src[off:e], "\r")})
		off = e + 1
	}
	if len(details.Lines) == 0 {
		l.Start, l.End = start, start
		return l
	}
	l.Start = details.Lines[0].Offset
	last := details.Lines[len(details.Lines)-1]
	l.End = last.Offset + len(last.Text)
	return l
}

// addBlankLeaves puts a leaf on every blank line of the source, so the caret
// can stand on it: a blank line is a line of the document, not the space
// between two blocks.
//
// The lines before the first block and after the last one are lines too. The
// line at the very end of a source that ends with a break is left out: the
// phantom paragraph stands there when the caret is below the last block.
func addBlankLeaves(a *leafArena, src string, leaves []*leaf) []*leaf {
	if len(leaves) == 0 {
		if src == "" {
			// The empty document has no line: the phantom is its one.
			return leaves
		}
		// A document no block holds is lines of its own: nothing at all, or
		// line breaks, which are content as much as a word is — a note of
		// three empty lines has three lines to put the caret on.
		for p := 0; p <= len(src); {
			bl := a.get()
			bl.Kind, bl.ID, bl.LineStart = Blank, -1, p
			bl.Start, bl.End, bl.SyntaxStart = p, p, p
			leaves = append(leaves, bl)
			if e := lineEndAt(src, p); e >= len(src) {
				break
			} else {
				p = e + 1
			}
		}
		return leaves
	}
	out := make([]*leaf, 0, len(leaves)+8)
	out = addBlanks(a, src, out, 0, leaves[0].Start, false, nil, leaves[0])
	for i, l := range leaves {
		out = append(out, l)
		to, next := len(src), (*leaf)(nil)
		if i+1 < len(leaves) {
			to, next = leaves[i+1].Start, leaves[i+1]
		}
		out = addBlanks(a, src, out, l.End, to, true, l, next)
	}
	// A source that ends with a break has one more line, empty: Enter at the
	// end of the document leaves the caret on it, and typing there writes it.
	if n := len(src); n > 0 && src[n-1] == '\n' && (len(out) == 0 || out[len(out)-1].End < n) {
		bl := a.get()
		bl.Kind, bl.ID, bl.LineStart = Blank, -1, n
		bl.Start, bl.End, bl.SyntaxStart = n, n, n
		out = append(out, bl)
	}
	return out
}

// addBlanks appends a leaf for every blank line of src in [from, to). With
// skipFirst the line at from is left out: it is the tail of the leaf above,
// which the break ending that leaf's line starts.
func addBlanks(a *leafArena, src string, out []*leaf, from, to int, skipFirst bool, prev, next *leaf) []*leaf {
	to = min(to, len(src))
	for p := from; p < to; {
		// Whole lines only: a leaf after the gap may start in the middle of
		// a line — after a quotation's marker — and that line is its own,
		// not a blank line of the gap.
		e := lineEndAt(src, p)
		if e > to {
			break
		}
		if start, depth, ok := blankLine(src, p, e); ok && (p > from || !skipFirst) {
			bl := a.get()
			bl.Kind, bl.ID, bl.LineStart = Blank, -1, p
			// A source blank line may contain spaces or tabs. Keep that source
			// span on the leaf so caret normalization and shaping retain the
			// actual insertion position while it remains Markdown-blank.
			bl.Start, bl.End, bl.SyntaxStart = start, e, start
			bl.Containers = quoteChain(prev, next, depth, p)
			out = append(out, bl)
		}
		if e >= to {
			break
		}
		p = e + 1
	}
	return out
}

// blankLine reads the line at p, ending at e, as a blank line: it holds
// nothing but the quotation markers it is inside, and the caret belongs
// after them. depth is how many `>` it carries, and start where its content
// would go — the line's own start when it carries none.
func blankLine(src string, p, e int) (start, depth int, ok bool) {
	i := p
	for {
		j := i
		for j < e && (src[j] == ' ' || src[j] == '\t') {
			j++
		}
		if j < e && src[j] == '>' {
			j++
			if j < e && src[j] == ' ' {
				j++
			}
			depth, i = depth+1, j
			continue
		}
		i = j
		break
	}
	for k := i; k < e; k++ {
		if src[k] != ' ' && src[k] != '\t' {
			return 0, 0, false
		}
	}
	if depth == 0 {
		return p, 0, true
	}
	return i, depth, true
}

// quoteChain returns the quotation containers a blank line carrying depth
// markers is inside: the quotations of the block the parse gives the lines
// around it, since a `>` line belongs to the quotation above and below it.
func quoteChain(prev, next *leaf, depth, at int) []cont {
	if depth == 0 {
		return nil
	}
	var short []cont
	for _, l := range []*leaf{next, prev} {
		if l == nil {
			continue
		}
		// A quotation may sit inside an item or a footnote: the chain up to
		// the depth-th quotation is the line's, item and all.
		n := 0
		valid := l.Containers
		for i, c := range l.Containers {
			// The following block may belong to a new quotation/callout.
			// Its containers cannot own an empty line before they begin.
			if c.Start > at {
				valid = l.Containers[:i]
				break
			}
			if c.Kind != Quote {
				continue
			}
			if n++; n == depth {
				return l.Containers[:i+1]
			}
		}
		if len(valid) > len(short) {
			short = valid
		}
	}
	// A standalone marker-only quotation can precede any parsed content.
	// Retain its indentation without borrowing a later callout's identity.
	out := append([]cont(nil), short...)
	n := 0
	for _, c := range out {
		if c.Kind == Quote {
			n++
		}
	}
	for ; n < depth; n++ {
		out = append(out, cont{Kind: Quote, ID: -at - n - 1, Start: at})
	}
	return out
}

// appendRaw adds the non-blank lines of src between from and to, which no
// block holds, as a raw leaf.
func appendRaw(a *leafArena, out []*leaf, src string, from, to int) []*leaf {
	if from < 0 || to <= from || from >= len(src) {
		return out
	}
	to = min(to, len(src))
	// Trim blank lines on both sides: an empty line, or one that holds
	// nothing but the markers of the containers it is in — a `>` line the
	// parse left out because the quotation ended there. Those are blank
	// lines, and addBlankLeaves gives them their leaves.
	for from < to {
		e := min(lineEndAt(src, from), to)
		if _, _, ok := blankLine(src, from, e); !ok {
			break
		}
		if e >= to {
			from = to
			break
		}
		from = e + 1
	}
	if from >= to {
		return out
	}
	for to > from {
		s := lineStartAt(src, to-1)
		if _, _, ok := blankLine(src, s, to); !ok {
			break
		}
		if s == 0 {
			to = 0
			break
		}
		to = s - 1
	}
	if to <= from {
		return out
	}
	from = lineStartAt(src, from)
	return append(out, rawLeaf(a, src, from, to, false))
}

// taskBoxAt returns the offset of the [ of a task item's check box, on the
// item's first line after its marker.
func taskBoxAt(src string, itemStart int) int {
	e := lineEndAt(src, itemStart)
	if i := strings.IndexByte(src[itemStart:e], '['); i >= 0 {
		return itemStart + i
	}
	return -1
}

// FrontmatterEnd returns the byte offset after non-empty YAML front matter.
func FrontmatterEnd(src string) int { return frontMatterEnd(src) }

// HideFrontmatterEnd includes empty front matter delimiters for display projection.
func HideFrontmatterEnd(src string) int { return hideFrontMatterEnd(src) }

// MaskFrontmatter blanks front matter while preserving every source byte offset.
func MaskFrontmatter(src string, end int) string { return maskFrontMatter(src, end) }
