// Package mdedit is Muse's native WYSIWYG Markdown editor: one editing
// surface for the whole document, drawn by MyGo, with the Markdown source
// as its only state. The source is parsed after every edit and rendered as
// Typora renders it: block syntax (#, list markers, >, fences, pipes) is
// never shown, inline syntax (**, `, [](), $, ![]()) shows only around the
// caret, and everything else is drawn as the document reads.
//
// Every caret, selection and edit is a byte offset into the source, so
// nothing is ever serialized: what the editor saves is exactly what the
// user wrote, and an outside change (an AI tool, a file reload) is just
// another edit of the same string.
package mdedit

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui/markdown/parser"
)

// exts is the Markdown the editor reads: GitHub's flavor, footnotes, math,
// alerts (`> [!NOTE]`, which Obsidian's callouts grow from) and mentions,
// with $…$ as inline math, as Typora writes it.
//
// GitHub's references are left out: `#123` parses as a reference to an
// issue, which would take a numeric Obsidian tag (`#2024`) away from the
// editor's own tag pass.
const exts = (parser.Default &^ parser.ExtReferences) | parser.ExtDollarMath

// leafKind is what a leaf of the document is: the units the editor lays
// out one after another.
type leafKind uint8

const (
	kPara leafKind = iota
	kHeading
	kCode
	kMath
	kHTML
	kRule
	kTable
	// kRaw is source no block holds, as link reference definitions and
	// front matter: shown and edited as plain lines.
	kRaw
	// kPhantom is the empty paragraph the caret sits in before it holds
	// any text (see phantom).
	kPhantom
)

// contKind is what a container around a leaf is.
type contKind uint8

const (
	cQuote contKind = iota
	cItem
	cFootnote
)

// cont is a container a leaf is inside: a quotation, an item of a list or
// a footnote's definition, outermost first in leaf.conts.
type cont struct {
	kind contKind
	// id is the container's block in the parse, which identifies it while
	// the source does not change.
	id int
	// start is the offset of its marker: >, a list item's bullet or
	// number, a footnote's [^.
	start int
	// An item's list: numbered or not, the number this item shows, its
	// bullet (-, *, +) or the delimiter after its number (. or )), whether
	// the list is tight, and how deep the list is nested in other lists.
	ordered bool
	num     int
	marker  byte
	tight   bool
	depth   int
	// task is the item's check box, taskAt the offset of its [.
	task   parser.Task
	taskAt int
	// label is a footnote's.
	label string
}

// line is a line of a code, math, HTML or raw leaf: its text without the
// prefixes of the containers around it, and the offset of that text.
type line struct {
	off  int
	text string
}

// leaf is a block the editor lays out: a paragraph, a heading, a block of
// code or math, a table, a rule, raw lines or the phantom paragraph.
type leaf struct {
	kind leafKind
	// id is the leaf's block in the parse; -1 for raw lines and the
	// phantom.
	id int
	// start and end bound the leaf's editable content in the source: a
	// paragraph's text, a heading's without its #, a code block's lines
	// without the fences, a table from its first cell to its last.
	start, end int
	// lineStart is where the leaf's first line starts, prefixes included,
	// and syntaxStart where the leaf's own syntax starts on it, after the
	// prefixes of its containers: a heading's #, a fence's backticks.
	lineStart   int
	syntaxStart int
	// level is a heading's.
	level int
	// conts are the containers around the leaf, outermost first, and
	// firstOf the indices of those whose first leaf this is, which draw
	// their markers beside it.
	conts   []cont
	firstOf []int
	// lines are a code, math, HTML or raw leaf's.
	lines []line
	// A fenced code block's info string, from infoStart to infoEnd on its
	// opening line, whose fence ends at fenceEnd; the language is the
	// info string's first word.
	fenced             bool
	fenceEnd           int
	infoStart, infoEnd int
	lang               string
	// tab is a table's rows and cells.
	tab *table
	// pending marks a heading, item or quotation still being typed: only
	// its marker, without the space after it, which shows as text until
	// the space comes, so that typing # does not flash a heading.
	pending bool
	// setext marks a heading underlined with = or -.
	setext bool
	// frontMatter marks the YAML front matter at the top of a document,
	// whole a document that is code, all of it this leaf.
	frontMatter bool
	whole       bool

	// Where the last layout put the leaf, in the editor's content.
	x, y, w, h float32
	// view is the leaf as displayed and laid out (see leafView), geom a
	// table's grid, and table the table a cell is in. vp is what the view
	// was made for besides the leaf's own content.
	view  *leafView
	vp    viewParams
	geom  *tableGeom
	table *leaf
}

// isText reports whether the leaf is edited as text with inline syntax.
func (l *leaf) isText() bool { return l.kind == kPara || l.kind == kHeading || l.kind == kPhantom }

// innerItem returns the innermost list item around the leaf, if any.
func (l *leaf) innerItem() (cont, bool) {
	for i := len(l.conts) - 1; i >= 0; i-- {
		if l.conts[i].kind == cItem {
			return l.conts[i], true
		}
	}
	return cont{}, false
}

// opensItem reports whether the leaf is the first of its innermost item.
func (l *leaf) opensItem() bool {
	n := len(l.conts)
	return n > 0 && l.conts[n-1].kind == cItem && slices.Contains(l.firstOf, n-1)
}

// table is a table leaf's model: its rows of cells, each cell a text leaf
// of its own, and how its columns align.
type table struct {
	rows  [][]*leaf
	align []parser.Align
	// delim is the source range of the delimiter row, below the header.
	delim [2]int
}

// cells returns the table's cells in reading order.
func (t *table) cells() []*leaf {
	var out []*leaf
	for _, r := range t.rows {
		out = append(out, r...)
	}
	return out
}

// cols returns the number of columns.
func (t *table) cols() int {
	n := len(t.align)
	for _, r := range t.rows {
		n = max(n, len(r))
	}
	return n
}

// lineStartAt returns the offset of the start of the line holding off.
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

// maskFrontMatter returns src with its front matter blanked out, its
// newlines kept, so that the parser reads blank lines there and every
// offset after it stays where it was.
func maskFrontMatter(src string, end int) string {
	if end == 0 {
		return src
	}
	b := []byte(src)
	for i := 0; i < end; i++ {
		if b[i] != '\n' && b[i] != '\r' {
			b[i] = ' '
		}
	}
	return string(b)
}

// bracketMath returns src with the [ of a \[ swapped for a letter where
// the parser would take it for the start of display math, as LaTeX writes
// it, but it reads as a bracket escaped, as CommonMark writes one: a \[
// with text after it at the start of a line and no \] on the line, which
// would make a block of math of the rest of the document, and a \[…\]
// holding no TeX, as \[1\] or \[注\]. Its result depends on each line
// alone, and is as long as src.
func bracketMath(src string) string {
	var b []byte
	for ls := 0; ls < len(src); {
		le := len(src)
		if i := strings.IndexByte(src[ls:], '\n'); i >= 0 {
			le = ls + i
		}
		line := src[ls:le]
		for i := strings.Index(line, `\[`); i >= 0; i = nextBracket(line, i) {
			if escapedAt(line, i) {
				continue
			}
			rest := line[i+2:]
			end := strings.Index(rest, `\]`)
			math := true
			switch {
			case end >= 0:
				math = texLike(rest[:end])
			case blockLead(line[:i]) && strings.TrimSpace(rest) != "":
				math = texLike(rest)
			}
			if !math {
				if b == nil {
					b = []byte(src)
				}
				b[ls+i+1] = 'x'
			}
		}
		ls = le + 1
	}
	if b == nil {
		return src
	}
	return string(b)
}

// nextBracket returns the offset of the next \[ in line after the one at
// i, or -1.
func nextBracket(line string, i int) int {
	if j := strings.Index(line[i+2:], `\[`); j >= 0 {
		return i + 2 + j
	}
	return -1
}

// escapedAt reports whether the character at i is escaped by a backslash.
func escapedAt(s string, i int) bool {
	n := 0
	for i-n-1 >= 0 && s[i-n-1] == '\\' {
		n++
	}
	return n%2 == 1
}

// blockLead reports whether s, the start of a line, is nothing but the
// markers of containers: spaces, quotation markers, and list items'
// bullets and numbers.
func blockLead(s string) bool {
	for i := 0; i < len(s); {
		switch c := s[i]; {
		case c == ' ' || c == '\t' || c == '>':
			i++
		case c == '-' || c == '*' || c == '+':
			if i+1 >= len(s) || s[i+1] != ' ' && s[i+1] != '\t' {
				return false
			}
			i += 2
		case c >= '0' && c <= '9':
			j := i
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			if j-i > 9 || j+1 >= len(s) || s[j] != '.' && s[j] != ')' || s[j+1] != ' ' && s[j+1] != '\t' {
				return false
			}
			i = j + 2
		default:
			return false
		}
	}
	return true
}

// texLike reports whether s reads as TeX rather than words: it holds a
// command, as \frac, a script, a brace or an operator.
func texLike(s string) bool {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '^', '_', '{', '}', '=', '+', '<', '>', '|':
			return true
		case '\\':
			if i+1 < len(s) && (s[i+1]|0x20 >= 'a' && s[i+1]|0x20 <= 'z') {
				return true
			}
		}
	}
	return false
}

// leafArena reuses the leaves of the last parse. A document of a few
// thousand blocks would otherwise allocate a leaf, its containers and its
// lines on every keystroke; the arena keeps their memory and fills it
// again, so editing costs no allocations for the leaves it does not
// change.
type leafArena struct {
	leaves []*leaf
	n      int
}

// reset makes the arena empty, keeping the leaves to fill again. The
// lines and containers go first: they hold slices of the source, and a
// source no leaf holds can be freed even while its leaves are kept for
// their views.
func (a *leafArena) reset() {
	for _, l := range a.leaves[:a.n] {
		clear(l.conts)
		clear(l.lines)
		l.conts, l.lines = l.conts[:0], l.lines[:0]
	}
	a.n = 0
}

// get returns an empty leaf. The slices the leaf had keep their room;
// everything else is cleared, so a stale view or table cannot leak into
// the new parse.
func (a *leafArena) get() *leaf {
	if a.n == len(a.leaves) {
		a.leaves = append(a.leaves, new(leaf))
	}
	l := a.leaves[a.n]
	a.n++
	conts, lines, firstOf := l.conts[:0], l.lines[:0], l.firstOf[:0]
	*l = leaf{}
	l.conts, l.lines, l.firstOf = conts, lines, firstOf
	return l
}

// setConts fills l's containers with conts, reusing the room it has.
func setConts(l *leaf, conts []cont) []cont {
	l.conts = append(l.conts[:0], conts...)
	return l.conts
}

// viewText returns the source text a leaf's view is made of: its lines
// from its first line's start, through its content, and for a code or math
// block through its last line.
func viewText(src string, l *leaf) string {
	end := l.end
	if l.kind == kRule {
		end = max(end, l.lineStart)
	}
	if l.kind == kCode || l.kind == kMath {
		end = max(l.end, lineEndAt(src, l.lineStart))
	}
	return src[l.lineStart:max(end, l.lineStart)]
}

// sameText reports whether a leaf of oldSrc and one of src hold the same
// source text. Their spans sit at different offsets when an edit moved
// them, so the text is compared from each leaf's first line, as the views
// are.
func sameText(oldSrc, src string, a, b *leaf) bool {
	return viewText(oldSrc, a) == viewText(src, b)
}

// sameLeaf reports whether two leaves show the same thing: the same
// block, the same bytes of the source, the same containers and lines. A
// leaf the edit did not change then keeps the view and the height it had,
// so laying the document out again does not look up every leaf's view.
//
// Offsets are compared from the leaf's first line, not from the start of
// the document: an edit moves every offset after it, and the display a
// view holds is relative to that line too.
func sameLeaf(a, b *leaf) bool {
	rel := func(l *leaf, off int) int {
		if off < 0 {
			return off
		}
		return off - l.lineStart
	}
	if a.kind != b.kind || a.id != b.id || a.level != b.level ||
		rel(a, a.start) != rel(b, b.start) || rel(a, a.end) != rel(b, b.end) ||
		rel(a, a.syntaxStart) != rel(b, b.syntaxStart) ||
		a.fenced != b.fenced || a.lang != b.lang ||
		a.pending != b.pending || a.setext != b.setext ||
		a.frontMatter != b.frontMatter || a.whole != b.whole ||
		(a.table != nil) != (b.table != nil) ||
		(a.tab != nil) != (b.tab != nil) ||
		len(a.conts) != len(b.conts) || !slices.Equal(a.firstOf, b.firstOf) ||
		len(a.lines) != len(b.lines) {
		return false
	}
	// A fence's info string is only where its fence is: an unfenced block
	// has zeros in those fields, and offsets from the leaf's line would
	// make them differ for no reason.
	if a.fenced &&
		(rel(a, a.fenceEnd) != rel(b, b.fenceEnd) ||
			rel(a, a.infoStart) != rel(b, b.infoStart) ||
			rel(a, a.infoEnd) != rel(b, b.infoEnd)) {
		return false
	}
	for i, c := range a.conts {
		d := b.conts[i]
		if c.kind != d.kind || c.id != d.id || rel(a, c.start) != rel(b, d.start) ||
			c.ordered != d.ordered || c.num != d.num || c.marker != d.marker ||
			c.tight != d.tight || c.depth != d.depth || c.task != d.task ||
			rel(a, c.taskAt) != rel(b, d.taskAt) || c.label != d.label {
			return false
		}
	}
	for i, ln := range a.lines {
		if rel(a, ln.off) != rel(b, b.lines[i].off) || ln.text != b.lines[i].text {
			return false
		}
	}
	if a.tab == nil {
		return true
	}
	if !slices.Equal(a.tab.align, b.tab.align) ||
		(a.tab.delim == [2]int{}) != (b.tab.delim == [2]int{}) ||
		a.tab.delim != [2]int{} && (rel(a, a.tab.delim[0]) != rel(b, b.tab.delim[0]) ||
			rel(a, a.tab.delim[1]) != rel(b, b.tab.delim[1])) ||
		len(a.tab.rows) != len(b.tab.rows) {
		return false
	}
	for r, row := range a.tab.rows {
		if len(row) != len(b.tab.rows[r]) {
			return false
		}
		for c, cell := range row {
			if !sameLeaf(cell, b.tab.rows[r][c]) {
				return false
			}
		}
	}
	return true
}

// buildLeaves turns the parse of src into the leaves the editor lays out,
// in document order, from the arena's memory.
func buildLeaves(src string, d *parser.Document, fm int, a *leafArena) []*leaf {
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
			case parser.Quote:
				first := len(out)
				n := len(conts)
				conts = append(conts, cont{kind: cQuote, id: c, start: b.Start})
				walk(c, conts, listDepth)
				conts = conts[:n]
				if len(out) == first {
					out = append(out, emptyLeaf(a, src, b, conts, cont{kind: cQuote, id: c, start: b.Start}))
				}
				markFirst(out[first:], len(conts))
			case parser.List:
				n := 0
				for it, end := c+1, d.Block(c).Next; it < end; it = d.Block(it).Next {
					ib := d.Block(it)
					ct := cont{
						kind: cItem, id: it, start: ib.Start,
						ordered: b.Ordered, num: b.Number + n, marker: b.Marker, tight: b.Tight,
						depth: listDepth, task: ib.Task, taskAt: -1,
					}
					if ib.Task != parser.NoTask {
						ct.taskAt = taskBoxAt(src, ib.Start)
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
			case parser.FootnoteDefinition:
				first := len(out)
				ct := cont{kind: cFootnote, id: c, start: b.Start, label: b.Label()}
				depth := len(conts)
				conts = append(conts, ct)
				walk(c, conts, listDepth)
				conts = conts[:depth]
				if len(out) == first {
					out = append(out, emptyLeaf(a, src, b, conts, ct))
				}
				markFirst(out[first:], len(conts))
			case parser.Paragraph:
				l := a.get()
				l.kind, l.id, l.start, l.end = kPara, c, b.Start, paraEnd(src, b.End)
				l.lineStart, l.syntaxStart = lineStartAt(src, b.Start), b.Start
				setConts(l, conts)
				out = append(out, l)
			case parser.Heading:
				out = append(out, headingLeaf(a, src, d, c, conts))
			case parser.ThematicBreak:
				l := a.get()
				l.kind, l.id, l.start, l.end = kRule, c, b.Start, b.End
				l.lineStart, l.syntaxStart = lineStartAt(src, b.Start), b.Start
				setConts(l, conts)
				out = append(out, l)
			case parser.CodeBlock:
				out = append(out, codeLeaf(a, src, d, c, conts))
			case parser.MathBlock:
				out = append(out, linesLeaf(a, src, d, c, kMath, conts))
			case parser.HTMLBlock:
				out = append(out, linesLeaf(a, src, d, c, kHTML, conts))
			case parser.Table:
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
	if len(leaves) > 0 && depth < len(leaves[0].conts) {
		leaves[0].firstOf = append(leaves[0].firstOf, depth)
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
func headingLeaf(a *leafArena, src string, d *parser.Document, c int, conts []cont) *leaf {
	b := d.Block(c)
	l := a.get()
	l.kind, l.id, l.level = kHeading, c, b.Level
	l.lineStart, l.syntaxStart = lineStartAt(src, b.Start), b.Start
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
			l.pending = true
			l.start, l.end = marks, p
			_ = hashes
			return l
		}
		for p < len(src) && (src[p] == ' ' || src[p] == '\t') {
			p++
		}
		l.start = p
		if ins := d.Inlines(c); len(ins) > 0 {
			l.end = max(b.End, p)
			if !closingHashes(src[l.end:lineEndAt(src, l.end)]) {
				l.end = paraEnd(src, l.end)
			}
		} else {
			l.end = max(p, lineEndAt(src, p))
		}
		if l.end < l.start {
			l.end = l.start
		}
		return l
	}
	// A setext heading: its text is its paragraph's.
	l.setext = true
	l.start, l.end = b.Start, b.End
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
func codeLeaf(a *leafArena, src string, d *parser.Document, c int, conts []cont) *leaf {
	b := d.Block(c)
	l := linesLeaf(a, src, d, c, kCode, conts)
	l.fenced = b.Fenced
	if b.Fenced {
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
		l.fenceEnd = p
		e := lineEndAt(src, p)
		l.infoStart, l.infoEnd = p, e
		for l.infoStart < e && (src[l.infoStart] == ' ' || src[l.infoStart] == '\t') {
			l.infoStart++
		}
		for l.infoEnd > l.infoStart && (src[l.infoEnd-1] == ' ' || src[l.infoEnd-1] == '\t' || src[l.infoEnd-1] == '\r') {
			l.infoEnd--
		}
		l.lang = b.Language()
	}
	return l
}

// linesLeaf makes the leaf of a block of lines: code, math or HTML.
func linesLeaf(a *leafArena, src string, d *parser.Document, c int, kind leafKind, conts []cont) *leaf {
	b := d.Block(c)
	l := a.get()
	l.kind, l.id = kind, c
	l.lineStart, l.syntaxStart = lineStartAt(src, b.Start), b.Start
	setConts(l, conts)
	n := d.LineCount(c)
	for k := 0; k < n; k++ {
		text, off := d.Line(c, k)
		// The parser's line is of the source it was given, which may have
		// characters swapped (see Editor.mask): the line is the source's.
		text = src[off : off+len(sourceLine(d.Source(), text, off))]
		l.lines = append(l.lines, line{off: off, text: strings.TrimSuffix(text, "\r")})
	}
	if kind == kMath || (kind == kCode && b.Fenced) {
		// A fenced block's content lines end before its closing fence; a
		// block opened without content gets the line after its fence, so
		// the caret has somewhere to be.
		if len(l.lines) == 0 {
			open := lineEndAt(src, b.Start)
			l.start, l.end = open, open
			l.lines = nil
			return l
		}
	}
	if len(l.lines) == 0 {
		l.start, l.end = b.Start, b.Start
		return l
	}
	l.start = l.lines[0].off
	last := l.lines[len(l.lines)-1]
	l.end = last.off + len(last.text)
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
func spanEnd(src string, d *parser.Document, c int) int {
	b := d.Block(c)
	end := lineEndAt(src, b.End)
	switch b.Kind {
	case parser.Heading:
		p := b.Start
		for p < len(src) && (src[p] == ' ' || src[p] == '\t') {
			p++
		}
		if (p >= len(src) || src[p] != '#') && end < len(src) {
			// A setext heading's underline.
			end = lineEndAt(src, end+1)
		}
	case parser.MathBlock:
		if !b.Open {
			// The closing $$ (or \]) ends the content's line, or the next.
			if rest := src[b.End:end]; !strings.Contains(rest, "$$") && !strings.Contains(rest, `\]`) && end < len(src) {
				next := lineEndAt(src, end+1)
				if t := trimPrefixChars(src[end+1 : next]); strings.HasPrefix(t, "$$") || strings.HasPrefix(t, `\]`) {
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
func tableLeaf(a *leafArena, src string, d *parser.Document, c int, conts []cont) *leaf {
	b := d.Block(c)
	t := &table{align: b.Align}
	l := a.get()
	l.kind, l.id = kTable, c
	l.start, l.end, l.lineStart, l.syntaxStart = b.Start, b.End, lineStartAt(src, b.Start), b.Start
	l.tab = t
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
			cl.kind, cl.id, cl.start, cl.end = kPara, cell, start, end
			cl.lineStart, cl.syntaxStart, cl.table = lineStartAt(src, cb.Start), start, l
			row = append(row, cl)
		}
		t.rows = append(t.rows, row)
	}
	// The delimiter row is the line after the header's.
	h := lineEndAt(src, b.Start)
	if h < len(src) {
		t.delim = [2]int{h + 1, lineEndAt(src, h+1)}
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
func emptyItemLeaf(a *leafArena, src string, ib *parser.Block, conts []cont, ct cont) *leaf {
	p := ib.Start
	for p < len(src) && src[p] != ' ' && src[p] != '\t' && src[p] != '\n' && src[p] != '\r' {
		p++
	}
	l := a.get()
	l.kind, l.id, l.lineStart = kPara, -1, lineStartAt(src, ib.Start)
	l.conts = append(l.conts[:0], conts...)
	l.conts = append(l.conts, ct)
	if p >= len(src) || src[p] == '\n' || src[p] == '\r' {
		// "-" alone: a marker still being typed, which shows as text,
		// outside the item it would make.
		l.pending = true
		l.conts = setConts(l, conts)
		l.start, l.end = ib.Start, p
		l.syntaxStart = ib.Start
		return l
	}
	p++ // the space after the marker
	if ct.taskAt >= 0 {
		p = max(p, ct.taskAt+4)
	}
	p = min(p, lineEndAt(src, ib.Start))
	l.start, l.end = p, max(p, lineEndAt(src, p))
	l.syntaxStart = p
	return l
}

// emptyLeaf makes the leaf of a quotation or footnote without content.
func emptyLeaf(a *leafArena, src string, b *parser.Block, conts []cont, ct cont) *leaf {
	e := lineEndAt(src, b.Start)
	p := b.Start
	if ct.kind == cQuote {
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
	l.kind, l.id, l.lineStart = kPara, -1, lineStartAt(src, b.Start)
	l.conts = append(l.conts[:0], conts...)
	l.conts = append(l.conts, ct)
	if ct.kind == cQuote && p == b.Start+1 {
		// ">" alone: still being typed, and shown as text outside the
		// quotation it would make.
		l.pending = true
		l.conts = setConts(l, conts)
		l.start, l.end = b.Start, p
		l.syntaxStart = b.Start
		return l
	}
	l.start, l.end = min(p, e), e
	l.syntaxStart = l.start
	return l
}

// codeDocLeaf makes the one leaf of a document that is code: all its
// lines, highlighted as lang.
func codeDocLeaf(a *leafArena, src, lang string) *leaf {
	l := rawLeaf(a, src, 0, len(src), false)
	l.kind = kCode
	l.whole = true
	if lang != "text" {
		l.lang = lang
	}
	if len(l.lines) == 0 {
		l.lines = []line{{off: 0}}
	}
	if n := len(l.lines); n > 0 && len(src) > 0 && src[len(src)-1] == '\n' && l.lines[n-1].off+len(l.lines[n-1].text) < len(src) {
		// The line after the last newline, for the caret.
		l.lines = append(l.lines, line{off: len(src)})
	}
	l.start = l.lines[0].off
	last := l.lines[len(l.lines)-1]
	l.end = last.off + len(last.text)
	return l
}

// rawLeaf makes a leaf of the lines of src from start to end.
func rawLeaf(a *leafArena, src string, start, end int, front bool) *leaf {
	l := a.get()
	l.kind, l.id, l.lineStart, l.syntaxStart, l.frontMatter = kRaw, -1, start, start, front
	for off := start; off <= end && off < len(src); {
		e := min(lineEndAt(src, off), end)
		l.lines = append(l.lines, line{off: off, text: strings.TrimSuffix(src[off:e], "\r")})
		off = e + 1
	}
	if len(l.lines) == 0 {
		l.start, l.end = start, start
		return l
	}
	l.start = l.lines[0].off
	last := l.lines[len(l.lines)-1]
	l.end = last.off + len(last.text)
	return l
}

// appendRaw adds the non-blank lines of src between from and to, which no
// block holds, as a raw leaf.
func appendRaw(a *leafArena, out []*leaf, src string, from, to int) []*leaf {
	if from < 0 || to <= from || from >= len(src) {
		return out
	}
	to = min(to, len(src))
	// Trim blank lines on both sides.
	for from < to {
		e := lineEndAt(src, from)
		if strings.TrimSpace(src[from:min(e, to)]) != "" {
			break
		}
		from = e + 1
	}
	if from >= to {
		return out
	}
	for to > from {
		s := lineStartAt(src, to-1)
		if strings.TrimSpace(src[s:to]) != "" {
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
