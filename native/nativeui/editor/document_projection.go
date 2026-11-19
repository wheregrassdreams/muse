// Package editor is Muse's native WYSIWYG Markdown editor: one editing
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
package editor

import (
	"slices"
	"strings"

	editorprojection "github.com/wheregrassdreams/muse-editor/markdown/render"
	sourceview "github.com/wheregrassdreams/muse-editor/markdown/source"
	syntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"
	"muse/ui/scrollbar"
)

// exts is the Markdown the editor reads: GitHub's flavor, footnotes, math,
// alerts (`> [!NOTE]`, which Obsidian's callouts grow from) and mentions,
// with $…$ as inline math, as Typora writes it.
//
// GitHub's references are left out: `#123` parses as a reference to an
// issue, which would take a numeric Obsidian tag (`#2024`) away from the
// editor's own tag pass.
const exts = (syntax.IncrementalDefault &^ syntax.IncrementalExtReferences) | syntax.IncrementalExtDollarMath

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
	// kBlank is a blank line of the source: a line, as every other leaf is,
	// with no text of its own. The caret stands on it, Enter opens another
	// below it and Backspace takes it away, because a blank line is a line
	// of the document and not the space between two blocks.
	kBlank
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
	kind  contKind
	alert syntax.IncrementalAlert
	title string
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
	task   syntax.IncrementalTask
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
// blockHorizontalState is toolkit-owned transient state for one rendered
// block. Scroll policy itself stays in the backend-neutral scroll package.
type blockHorizontalState struct {
	Offset    float32
	Shown     bool
	Indicator scrollbar.Indicator
}

type leaf struct {
	kind leafKind
	// Flags describing syntax owned by the leaf.
	fenced      bool
	pending     bool
	setext      bool
	frontMatter bool
	whole       bool
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
	// tab is a table's rows and cells.
	tab *table
	// details are the cold fields used by code, HTML, raw and similar blocks.
	details *leafDetails

	// Where the last layout put the leaf, in the editor's content.
	x, y, w, h float32
	// view is the leaf as displayed and laid out (see leafView), geom a
	// table's grid, and table the table a cell is in. vp is what the view
	// was made for besides the leaf's own content.
	view       *leafView
	geom       *tableGeom
	table      *leaf
	horizontal blockHorizontalState
}

// leafDetails keeps line arrays and code-fence metadata out of the common
// prose leaf. It is allocated only for leaves that need those fields.
type leafDetails struct {
	lines              []line
	lang               string
	fenceEnd           int
	infoStart, infoEnd int
}

func (l *leaf) detail() *leafDetails {
	if l == nil {
		return nil
	}
	return l.details
}

func (l *leaf) ensureDetails() *leafDetails {
	if l.details == nil {
		l.details = &leafDetails{}
	}
	return l.details
}

func (l *leaf) codeLines() []line {
	if d := l.detail(); d != nil {
		return d.lines
	}
	return nil
}

func (l *leaf) language() string {
	if d := l.detail(); d != nil {
		return d.lang
	}
	return ""
}

func (l *leaf) fenceEndOffset() int {
	if d := l.detail(); d != nil {
		return d.fenceEnd
	}
	return 0
}

func (l *leaf) infoStartOffset() int {
	if d := l.detail(); d != nil {
		return d.infoStart
	}
	return 0
}

func (l *leaf) infoEndOffset() int {
	if d := l.detail(); d != nil {
		return d.infoEnd
	}
	return 0
}

// isText reports whether the leaf is edited as text with inline syntax.
func (l *leaf) isText() bool {
	return l.kind == kPara || l.kind == kHeading || l.kind == kPhantom || l.kind == kBlank
}

// isLine reports whether the leaf is a line of its own with no text the
// caret walks through: the phantom paragraph, or a blank line.
func (l *leaf) isLine() bool { return l.kind == kPhantom || l.kind == kBlank }

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
	align []syntax.IncrementalAlign
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
// hideFrontMatterEnd is a display policy: the editor can hide even an empty
// frontmatter delimiter pair, while the parser correctly treats it as rules.
func hideFrontMatterEnd(src string) int {
	if end := sourceview.FrontmatterEnd(src); end > 0 {
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

// leafArena reuses the leaves of the last parse. A document of a few
// thousand blocks would otherwise allocate a leaf, its containers and its
// lines on every keystroke; the arena keeps their memory and fills it
// again, so editing costs no allocations for the leaves it does not
// change.
type leafArena struct {
	// chunks give leaves stable addresses without one heap allocation per
	// node. The Editor keeps the parse's output order separately.
	chunks [][]leaf
	n      int
}

const leafArenaChunkSize = 256

// reset makes the arena empty, keeping the leaves to fill again. The
// lines and containers go first: they hold slices of the source, and a
// source no leaf holds can be freed even while its leaves are kept for
// their views.
func (a *leafArena) reset() {
	for i := 0; i < a.n; i++ {
		l := a.leafAt(i)
		clear(l.conts)
		l.conts = l.conts[:0]
		// Arenas can outlive the document that filled them. Drop retained
		// layouts and table projections for every used leaf, including leaves
		// the next parse is too small to reuse.
		l.view, l.geom, l.tab, l.table = nil, nil, nil, nil
		if d := l.details; d != nil {
			clear(d.lines)
			d.lines = d.lines[:0]
			d.lang = ""
			d.fenceEnd, d.infoStart, d.infoEnd = 0, 0, 0
		}
	}
	a.n = 0
}

// release drops the arena's backing chunks at a document boundary. Keeping
// them is useful between edits to one document, but can pin a large document's
// peak leaf capacity after the user opens a smaller note.
func (a *leafArena) release() {
	a.reset()
	a.chunks = nil
}

// get returns an empty leaf. The slices the leaf had keep their room;
// everything else is cleared, so a stale view or table cannot leak into
// the new parse.
func (a *leafArena) get() *leaf {
	chunkIndex := a.n / leafArenaChunkSize
	if chunkIndex == len(a.chunks) {
		a.chunks = append(a.chunks, make([]leaf, leafArenaChunkSize))
	}
	l := a.leafAt(a.n)
	a.n++
	conts, firstOf, details := l.conts[:0], l.firstOf[:0], l.details
	*l = leaf{}
	l.conts, l.firstOf, l.details = conts, firstOf, details
	return l
}

func (a *leafArena) leafAt(i int) *leaf {
	return &a.chunks[i/leafArenaChunkSize][i%leafArenaChunkSize]
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
		a.fenced != b.fenced || a.language() != b.language() ||
		a.pending != b.pending || a.setext != b.setext ||
		a.frontMatter != b.frontMatter || a.whole != b.whole ||
		(a.table != nil) != (b.table != nil) ||
		(a.tab != nil) != (b.tab != nil) ||
		len(a.conts) != len(b.conts) || !slices.Equal(a.firstOf, b.firstOf) ||
		len(a.codeLines()) != len(b.codeLines()) {
		return false
	}
	// A fence's info string is only where its fence is: an unfenced block
	// has zeros in those fields, and offsets from the leaf's line would
	// make them differ for no reason.
	if a.fenced &&
		(rel(a, a.fenceEndOffset()) != rel(b, b.fenceEndOffset()) ||
			rel(a, a.infoStartOffset()) != rel(b, b.infoStartOffset()) ||
			rel(a, a.infoEndOffset()) != rel(b, b.infoEndOffset())) {
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
	for i, ln := range a.codeLines() {
		if rel(a, ln.off) != rel(b, b.codeLines()[i].off) || ln.text != b.codeLines()[i].text {
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

// buildLeaves adapts detached Markdown projection values into the editor's
// arena-backed host leaves. The projection package never receives or retains
// Editor, leaf, layout, or MyGo state.
func buildLeaves(src string, d sourceview.SyntaxView, fm int, a *leafArena, projector *editorprojection.SourceProjector) []*leaf {
	return adaptDocumentBlocks(projector.Build(src, d, fm), a)
}

func adaptDocumentBlocks(projected []*editorprojection.SourceBlock, a *leafArena) []*leaf {
	if len(projected) == 0 && projected == nil {
		return nil
	}
	a.reset()
	out := make([]*leaf, 0, len(projected))
	var adapt func(*editorprojection.SourceBlock, *leaf) *leaf
	adapt = func(block *editorprojection.SourceBlock, tableOwner *leaf) *leaf {
		if block == nil {
			return nil
		}
		l := a.get()
		l.kind = hostLeafKind(block.Kind)
		l.fenced, l.pending, l.setext = block.Fenced, block.Pending, block.Setext
		l.frontMatter, l.whole = block.FrontMatter, block.Whole
		l.id, l.start, l.end = block.ID, block.Start, block.End
		l.lineStart, l.syntaxStart, l.level = block.LineStart, block.SyntaxStart, block.Level
		l.conts = l.conts[:0]
		for _, container := range block.Containers {
			l.conts = append(l.conts, cont{
				kind: hostContainerKind(container.Kind), alert: container.Alert, title: container.Title,
				id: container.ID, start: container.Start, ordered: container.Ordered,
				num: container.Number, marker: container.Marker, tight: container.Tight,
				depth: container.Depth, task: container.Task, taskAt: container.TaskAt, label: container.Label,
			})
		}
		l.firstOf = append(l.firstOf[:0], block.FirstOf...)
		l.table = tableOwner
		if details := block.Details; details != nil {
			d := l.ensureDetails()
			d.lang = details.Language
			d.fenceEnd, d.infoStart, d.infoEnd = details.FenceEnd, details.InfoStart, details.InfoEnd
			for _, sourceLine := range details.Lines {
				d.lines = append(d.lines, line{off: sourceLine.Offset, text: sourceLine.Text})
			}
		}
		if tableProjection := block.Table; tableProjection != nil {
			t := &table{align: append([]syntax.IncrementalAlign(nil), tableProjection.Align...), delim: tableProjection.Delimiter}
			l.tab = t
			for _, sourceRow := range tableProjection.Rows {
				row := make([]*leaf, 0, len(sourceRow))
				for _, cell := range sourceRow {
					row = append(row, adapt(cell, l))
				}
				t.rows = append(t.rows, row)
			}
		}
		return l
	}
	for _, block := range projected {
		out = append(out, adapt(block, nil))
	}
	return out
}

func hostLeafKind(kind editorprojection.SourceKind) leafKind {
	switch kind {
	case editorprojection.SourceParagraph:
		return kPara
	case editorprojection.SourceHeading:
		return kHeading
	case editorprojection.SourceCode:
		return kCode
	case editorprojection.SourceMath:
		return kMath
	case editorprojection.SourceHTML:
		return kHTML
	case editorprojection.SourceRule:
		return kRule
	case editorprojection.SourceTableBlock:
		return kTable
	case editorprojection.SourceRaw:
		return kRaw
	case editorprojection.SourcePhantom:
		return kPhantom
	case editorprojection.SourceBlank:
		return kBlank
	default:
		return kRaw
	}
}

func hostContainerKind(kind editorprojection.SourceContainerKind) contKind {
	switch kind {
	case editorprojection.SourceQuote:
		return cQuote
	case editorprojection.SourceItem:
		return cItem
	case editorprojection.SourceFootnote:
		return cFootnote
	default:
		return cQuote
	}
}

// rawLeaf and codeDocLeaf build host-only synthetic leaves used by embedded
// code mode and transient comment display; Markdown AST projection belongs to
// projection.
func rawLeaf(a *leafArena, src string, start, end int, front bool) *leaf {
	l := a.get()
	l.kind, l.id, l.lineStart, l.syntaxStart, l.frontMatter = kRaw, -1, start, start, front
	details := l.ensureDetails()
	for off := start; off <= end && off < len(src); {
		lineEnd := min(lineEndAt(src, off), end)
		details.lines = append(details.lines, line{off: off, text: strings.TrimSuffix(src[off:lineEnd], "\r")})
		off = lineEnd + 1
	}
	if len(details.lines) == 0 {
		l.start, l.end = start, start
		return l
	}
	l.start = details.lines[0].off
	last := details.lines[len(details.lines)-1]
	l.end = last.off + len(last.text)
	return l
}

func codeDocLeaf(a *leafArena, src, language string) *leaf {
	l := rawLeaf(a, src, 0, len(src), false)
	l.kind, l.whole = kCode, true
	details := l.ensureDetails()
	if language != "text" {
		details.lang = language
	}
	if len(details.lines) == 0 {
		details.lines = append(details.lines, line{off: 0})
	}
	if n := len(details.lines); n > 0 && len(src) > 0 && src[len(src)-1] == '\n' && details.lines[n-1].off+len(details.lines[n-1].text) < len(src) {
		details.lines = append(details.lines, line{off: len(src)})
	}
	l.start = details.lines[0].off
	last := details.lines[len(details.lines)-1]
	l.end = last.off + len(last.text)
	return l
}
