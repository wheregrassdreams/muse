package mdedit

import (
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"muse/mdoc"
)

// Options configure an editor. The app sets them before building the
// editor and changes them as its settings change.
type Options struct {
	// FontSize is the body text's size in DIPs (16 when 0), LineHeight its
	// line height as a multiple of it (1.75 when 0).
	FontSize   float32
	LineHeight float32
	// Font and MonoFont are the families of text and of code: the
	// theme's font and the system's monospaced one when empty.
	Font, MonoFont string
	// MaxWidth caps the column of text (820 when 0); PadX is the least room
	// beside it.
	MaxWidth float32
	PadX     float32
	// BaseDir resolves the relative paths of images.
	BaseDir string
	// Placeholder shows in an empty document.
	Placeholder string
	// Label names the editor to assistive technology.
	Label string
	// Invalidate asks the window for a frame, from any goroutine, as an
	// image loaded in the background does.
	Invalidate func()
	// OnChange is called after every change of the source, the user's and
	// the app's.
	OnChange func()
	// OnLink opens a link the user Cmd-clicked (Ctrl-clicked).
	OnLink func(url string)
	// Code makes the document code rather than Markdown, as a .json or a
	// .go file is: its lines show as they are, highlighted as the language
	// of that name, or plain when the name is "text".
	Code string
	// SaveImage stores an image pasted into the document and returns the
	// path a Markdown image links it by; nil keeps pasted images out.
	SaveImage func(data []byte, ext string) (string, error)
	// OnDropFiles takes the files dropped on the editor that are not
	// images, as a document to open.
	OnDropFiles func(paths []string)
	// ReadClipboard and WriteClipboard replace the system's clipboard, as
	// tests do.
	ReadClipboard  func() transfer.Data
	WriteClipboard func(transfer.Data)
}

// Editor is the state of a Markdown editor, kept by the app across frames:
// the source and its parse, the selection, the history of undo and what
// the editor laid out. View builds it.
type Editor struct {
	opts Options

	src    string
	leaves []*leaf
	// p is what the parser reads, and what the editor derives from the
	// source for it: the document, the masked source it holds, the comments
	// that mask hides, where the front matter ends. Only p.set and p.edit
	// write it (see parsed.go); everything else reads it.
	p parserSource
	// arena holds the leaves of the parse before and the one being built:
	// a leaf the edit did not change keeps its view, so the leaves of the
	// parse before have to stay whole while the next is built.
	arena    [2]leafArena
	arenaCur int
	// leafSrc is the source the leaves were built from, which an edit has
	// usually moved on from by the time they are compared with the next.
	leafSrc string
	// leafComment is the comment the leaves were built for: a comment's own
	// leaves are dropped from the layout unless the caret is in it, and the
	// caret moving in or out rebuilds the leaves (never the parse).
	leafComment   cmt
	leafCommentOK bool
	// pending is the one source edit not yet given to the parser;
	// pendingMulti says a committed step changed more than one range, which
	// has to be replayed from the whole source.
	pending      *pendingEdit
	pendingMulti bool
	// rev counts changes of the source; parsed is the rev leaves reflect.
	rev, parsed uint64

	// The selection: its anchor and its caret, byte offsets of the source.
	anchor, caret int
	// phantom is the empty paragraph the caret is in, if it is in one.
	phantom *phantom
	// goalX is the x a run of Up and Down keeps the caret at.
	goalX   float32
	hasGoal bool
	// upstream draws the caret at the end of the line before when it sits
	// where a line wraps.
	upstream bool

	// An input method's composition, shown at the caret until committed;
	// composeSel is its selection, in UTF-16 units of it.
	compose    string
	composeSel ui.TextInputRange
	composing  bool

	hist history

	lk       look
	views    map[viewKey]*leafView
	frame    uint64
	assetGen atomic.Uint64

	// The element's size and where the column of text is in it.
	elemW, viewH float32
	colX, colW   float32
	padTop       float32
	padBottom    float32
	docH         float32
	// viewTop and viewBottom bound what the last frame showed, in the
	// content's coordinates: a large document shapes only the leaves near
	// them (see layoutDoc). They are the top of the document until a frame
	// has given them a size.
	viewTop, viewBottom float32
	// estW, estSize and estLH are the width and text size the heights of
	// the leaves that are not shaped were last estimated at. A layout at
	// another width or size measures them again: their text wraps
	// differently, and what is below them would sit where the old look put
	// it.
	estW, estSize, estLH float32

	scroll       ui.ScrollState
	scrollHandle ui.Handle
	handle       ui.Handle
	focused      bool
	blinkStart   time.Time
	// wantReveal scrolls the caret into view at the next frame.
	wantReveal bool
	// scrollTo, when set, scrolls an offset to the top of the view at the
	// next frame; scrollCenter centers it there instead, as the rail does.
	scrollTo     int
	hasScrollTo  bool
	scrollCenter bool

	// The pointer: a drag selecting by characters, words or blocks, from
	// the range it started on; the syntax shown is frozen meanwhile, so
	// that the text does not move under it.
	dragging  bool
	dragUnit  int
	dragFrom  [2]int
	frozen    bool
	frozenAt  int
	pointerX  float32
	pointerY  float32
	hoverLink string
	// diagDrag is a diagram's scroll bar being dragged, if one is; diagPan
	// is the pointer dragging a rendered diagram itself sideways.
	diagDrag   *diagDrag
	diagPan    *diagPan
	services   ui.Services
	lastChange time.Time

	// highlights are ranges of the source drawn as found, as a search's
	// matches; current is the one chosen.
	highlights []Range
	current    int

	// elemRect is the element's box in the window, as the last frame drew it.
	elemRect ui.Rect
	// cursor is the pointer's shape where it rests.
	cursor ui.Cursor

	// phantomStays keeps the phantom an edit made through its commit.
	phantomStays bool
	u16ix        u16Index
	laidKey      laid
	leavesLaid   bool
	// unitList is the leaves and table cells the caret moves through,
	// built again when the source changes (see units).
	unitList []*leaf
	unitRev  uint64

	// The navigation rail: the headings of the parse, in order, and the
	// first block of each one's section for the preview; railRev is the rev
	// they were collected at. railHover is the tick the pointer is on (-1
	// none) and railActive the one the view is in; railRect is the rail's
	// box and railYs its ticks' centers as the last frame drew them.
	// railScales are the ticks' widths, animated toward their distances
	// from the highlighted tick, and railCardY the preview's center,
	// animated the same way; railCardOn says whether it shows.
	railLeaves []*leaf
	railNext   []*leaf
	railRev    uint64
	// hoverBlock is the start of the block the pointer is on, or -1: a
	// block shows its source control there (see sourceChip).
	hoverBlock int
	// sourceAt is the block the source control was last used on, while it
	// shows its Markdown source instead of the block it draws, and sourceOK
	// says it does. The caret leaving the block stops it.
	sourceAt    int
	sourceOK    bool
	railHover   int
	railActive  int
	railRect    ui.Rect
	railYs      []float32
	railPitch   float32
	railScales  []float32
	railAnim    bool
	railCardY   float32
	railCardOn  bool
	railText    string
	railDesc    string
	railTextRev uint64
	railTextIdx int
}

// Range is a range of the source, in bytes.
type Range struct{ Start, End int }

// pendingEdit is a change of the source not yet given to the parser: the
// bytes from to were replaced by text.
type pendingEdit struct {
	from, to int
	text     string
}

// diagDrag is the dragging of a diagram's scroll bar: the leaf it belongs
// to, the pointer's x as the drag began, and the scroll offset then.
type diagDrag struct {
	leaf  *leaf
	start float32
	from  float32
}

// diagPan is the dragging of a rendered diagram itself: the leaf, the
// pointer's x as the drag began, and the scroll offset then.
type diagPan struct {
	leaf  *leaf
	start float32
	from  float32
}

// phantom is an empty paragraph the caret is in: Markdown has no empty
// paragraphs, so it is not in the source until text is typed in it, which
// goes in at at with pre before it and post after it.
type phantom struct {
	at        int
	pre, post string
	conts     []cont
	// before places it above the leaf starting at at, rather than below
	// the one ending there.
	before bool
}

// New returns an editor of src.
func New(src string, opts Options) *Editor {
	e := &Editor{opts: opts, views: map[viewKey]*leafView{}, current: -1, cursor: ui.CursorText,
		railHover: -1, railActive: -1,
		hoverBlock: -1}
	// The window's theme comes with the first frame; until then, the light
	// one, so that a document opened before it lays out.
	e.setLook(ui.LightTheme())
	e.SetSource(src)
	return e
}

// SetOptions changes the editor's options.
func (e *Editor) SetOptions(opts Options) {
	if opts.Code != e.opts.Code {
		e.rev++
		e.parsed = 0
		e.p.clear()
		e.pending, e.pendingMulti = nil, false
	}
	e.opts = opts
}

// Options returns the editor's options.
func (e *Editor) Options() Options { return e.opts }

// Source returns the Markdown being edited.
func (e *Editor) Source() string { return e.src }

// SetSource replaces the document, as opening another file does: the
// caret goes to its start and the history of undo is cleared.
func (e *Editor) SetSource(src string) {
	e.src = src
	e.p.clear()
	e.rev++
	e.u16ix.rev = 0
	e.pending, e.pendingMulti = nil, false
	e.anchor, e.caret = 0, 0
	e.phantom = nil
	e.compose, e.composing = "", false
	e.hist = history{}
	e.highlights, e.current = nil, -1
	e.scroll.Y = 0
	e.hasGoal = false
	e.railHover = -1
	e.railCardOn = false
	e.sync()
	if src == "" {
		// The empty document is the one line the source does not hold: a
		// line break is content, and gives the document its lines.
		e.phantom = &phantom{at: 0}
		e.sync()
	}
	e.caret = e.firstStop()
	e.anchor = e.caret
}

// sync parses the source again when it changed, and makes the leaves.
func (e *Editor) sync() {
	if e.parsed == e.rev && e.leaves != nil {
		// The caret may have moved into or out of a comment: the leaves are
		// built for the comment it is in (its own show, the others do not),
		// so that a caret moving there rebuilds leaves, not the parse.
		if f, ok := commentAt(e.p.comments, e.caret); ok != e.leafCommentOK || (ok && f != e.leafComment) {
			e.rev++
		} else {
			return
		}
	}
	if e.opts.Code != "" {
		e.arenaCur ^= 1
		a := &e.arena[e.arenaCur]
		a.reset()
		e.leaves = []*leaf{codeDocLeaf(a, e.src, e.opts.Code)}
		e.leafSrc = e.src
		e.parsed = e.rev
		e.p.clear()
		e.pending, e.pendingMulti = nil, false
		return
	}
	// The parser's input is made here and nowhere else: one edit it can
	// follow, or the whole source again. The choice is the same for every
	// field of it, so a fast path cannot leave one of them behind.
	if e.p.src != e.src {
		fm := frontMatterEnd(e.src)
		if e.pending == nil || e.pendingMulti || fm != e.p.fm || !e.p.edit(e.src, e.pending) {
			e.p.set(e.src, fm)
		}
	}
	e.pending, e.pendingMulti = nil, false
	old := e.leaves
	e.arenaCur ^= 1
	e.leaves = buildLeaves(e.src, e.p.doc, e.p.fm, &e.arena[e.arenaCur])
	e.leaves = addBlankLeaves(&e.arena[e.arenaCur], e.src, e.leaves)
	// The block shown as its source is a block of lines, not the block it
	// draws: a table stops being cells and becomes the Markdown it is
	// written as. It stays open while the caret is in it.
	if e.sourceOK {
		if l := e.leafAt(e.caret); l == nil || l.lineStart != e.sourceAt {
			e.sourceOK = false
		}
	}
	if e.sourceOK {
		i := e.blockAt(e.sourceAt)
		switch {
		case i < 0:
			e.sourceOK = false
		case e.leaves[i].kind == kTable:
			// A table draws as a grid, not as the lines it is written in: it
			// becomes a block of lines to show its source at all.
			l := e.leaves[i]
			e.leaves[i] = rawLeaf(&e.arena[e.arenaCur], e.src, l.lineStart, blockEnd(e.src, l), false)
		case !e.pictureBlock(e.leaves[i]):
			e.sourceOK = false
		}
		// A picture opens its source in place: the display shows the tag
		// where it drew the picture (see builder.objectOpen).
	}
	// A comment covers its lines whole: the parser reads blanks there, which
	// used to become raw leaves showing the comment's own text. With the
	// comment blanked, its lines are a gap the caret reaches through the
	// phantom — and the comment shows when the caret is in it.
	if len(e.p.comments) > 0 {
		kept := e.leaves[:0]
		e.leafComment, e.leafCommentOK = commentAt(e.p.comments, e.caret)
		for _, l := range e.leaves {
			if e.insideComment(l.start, l.end) {
				continue
			}
			kept = append(kept, l)
		}
		e.leaves = kept
	}
	if p := e.phantom; p != nil {
		// A line of the source at the phantom's offset is that line: the
		// phantom was the line while the source held none.
		if lineAt(e.leaves, p.at) != nil {
			e.phantom = nil
		} else {
			e.leaves = insertPhantom(e.leaves, p)
		}
	}
	carryViews(e.leafSrc, e.src, old, e.leaves)
	e.leafSrc = e.src
	e.parsed = e.rev
}

// carryViews gives the leaves of the new parse the views of the leaves of
// the parse before where the two show the same thing. Without it a
// document laid out after every keystroke would look up the view of every
// leaf, hashing all of its text to find it.
func carryViews(oldSrc, src string, old, next []*leaf) {
	if len(old) != len(next) {
		return
	}
	for i, l := range next {
		o := old[i]
		if !sameLeaf(o, l) || !sameText(oldSrc, src, o, l) {
			continue
		}
		l.view, l.vp, l.geom = o.view, o.vp, o.geom
		// The height is an estimate until the leaf is shaped again: it is
		// what the leaf took where it was last laid out.
		l.h = o.h
		if l.tab == nil {
			continue
		}
		for r, row := range l.tab.rows {
			for c, cell := range row {
				oc := o.tab.rows[r][c]
				cell.view, cell.vp = oc.view, oc.vp
			}
		}
	}
}

// commentsStay reports whether an edit leaves the comments of the source as
// they were, moved by it at most. A comment is written with `%`, escaped
// with a `\`, and hidden inside a fence of ` or ~: an edit that keeps away
// from all of them can neither make a comment nor unmake one, nor change
// which fences hide the ones after it.
func commentsStay(oldSrc, newSrc string, from, to, newLen int) bool {
	marker := func(b byte) bool {
		return b == '%' || b == '\\' || b == '`' || b == '~'
	}
	for i := from; i < to && i < len(oldSrc); i++ {
		if marker(oldSrc[i]) {
			return false
		}
	}
	for i := from; i < from+newLen && i < len(newSrc); i++ {
		if marker(newSrc[i]) {
			return false
		}
	}
	if from > 0 && marker(oldSrc[from-1]) {
		return false
	}
	if to < len(oldSrc) && marker(oldSrc[to]) {
		return false
	}
	return true
}

// insideComment reports whether a source range lies wholly in a comment the
// caret is not in: that comment's leaves are dropped, so it takes no room
// and its source shows only while the caret is in it.
func (e *Editor) insideComment(start, end int) bool {
	for _, c := range e.p.comments {
		if e.leafCommentOK && c == e.leafComment {
			continue
		}
		if c.from <= start && end <= c.to {
			return true
		}
	}
	return false
}

// touchesComment reports whether the edit range reached a comment.
func touchesComment(cmts []cmt, from, to int) bool {
	for _, c := range cmts {
		if from <= c.to && to >= c.from {
			return true
		}
	}
	return false
}

// parserText returns whole lines of the source as the parser reads them.
func parserText(lines string) string { return mdoc.CJKFriendly(bracketMath(lines)) }

// blockAt returns the index of the leaf whose first line starts at lineStart,
// or -1: the block a source control was used on, found again by its line.
func (e *Editor) blockAt(lineStart int) int {
	for i, l := range e.leaves {
		if l.lineStart == lineStart {
			return i
		}
	}
	return -1
}

// lineAt returns the line leaf that starts at off, if the source holds one
// there: a blank line, or the phantom paragraph another pass put there.
func lineAt(leaves []*leaf, off int) *leaf {
	for _, l := range leaves {
		if l.isLine() && l.start == off {
			return l
		}
	}
	return nil
}

// insertPhantom puts the phantom's leaf among the leaves, by its offset.
func insertPhantom(leaves []*leaf, p *phantom) []*leaf {
	pl := &leaf{kind: kPhantom, id: -1, start: p.at, end: p.at, lineStart: p.at, conts: p.conts}
	i := sort.Search(len(leaves), func(i int) bool {
		l := leaves[i]
		if p.before {
			return l.lineStart >= p.at
		}
		return l.lineStart > p.at || (l.lineStart >= p.at && l.end > p.at)
	})
	if !p.before {
		// After every leaf ending at or before it.
		for i < len(leaves) && leaves[i].end <= p.at && leaves[i].lineStart <= p.at && !leaves[i].isLine() {
			i++
		}
	}
	out := make([]*leaf, 0, len(leaves)+1)
	out = append(out, leaves[:i]...)
	out = append(out, pl)
	return append(out, leaves[i:]...)
}

// leafLo returns the first offset the caret can take in a leaf: a fenced
// code block's info string comes before its lines.
func leafLo(l *leaf) int {
	if l.kind == kCode && l.fenced {
		return l.infoStart
	}
	return l.start
}

// leafIndex returns the index of the leaf the caret at off is in: the
// phantom when it is in it, else the leaf whose content holds off, else
// -1.
func (e *Editor) leafIndex(off int) int {
	// A line of its own owns its offset: a blank line, or the phantom
	// paragraph the caret opens below a block. It may come after the leaf
	// that ends where it starts, so it is found before that leaf is taken.
	line, cover := -1, -1
	for i, l := range e.leaves {
		if l.isLine() {
			if line < 0 && off == l.start {
				line = i
			}
			continue
		}
		if cover >= 0 {
			continue
		}
		if off >= leafLo(l) && off <= l.end {
			if l.kind == kCode && l.fenced && off > l.infoEnd && off < l.start {
				continue
			}
			cover = i
		}
	}
	if line >= 0 {
		return line
	}
	return cover
}

// leafAt returns the leaf the caret at off is in, or nil.
func (e *Editor) leafAt(off int) *leaf {
	if i := e.leafIndex(off); i >= 0 {
		return e.leaves[i]
	}
	return nil
}

// cellAt returns the cell of a table holding off, with its row and column.
func cellAt(l *leaf, off int) (*leaf, int, int) {
	if l.tab == nil {
		return nil, -1, -1
	}
	for r, row := range l.tab.rows {
		for c, cell := range row {
			if off >= cell.start && off <= cell.end {
				return cell, r, c
			}
		}
	}
	return nil, -1, -1
}

// selection returns the selection's start and end.
func (e *Editor) selection() (int, int) {
	if e.anchor < e.caret {
		return e.anchor, e.caret
	}
	return e.caret, e.anchor
}

// SelectionRange returns the selected range of the source.
func (e *Editor) SelectionRange() Range {
	a, z := e.selection()
	return Range{a, z}
}

// SelectedText returns the source the selection holds.
func (e *Editor) SelectedText() string {
	a, z := e.selection()
	return e.src[a:z]
}

// revealCaret returns the offset whose syntax shows: the caret's, or where
// it was when a drag began. -1 while the editor does not have the focus.
func (e *Editor) revealCaret() int {
	if !e.focused {
		return -1
	}
	if e.frozen {
		return e.frozenAt
	}
	return e.caret
}

// firstStop returns the first offset the caret can take.
func (e *Editor) firstStop() int {
	for _, l := range e.leaves {
		if s, ok := e.edgeStop(l, false); ok {
			return s
		}
	}
	return 0
}

// lastStop returns the last offset the caret can take.
func (e *Editor) lastStop() int {
	for i := len(e.leaves) - 1; i >= 0; i-- {
		if s, ok := e.edgeStop(e.leaves[i], true); ok {
			return s
		}
	}
	return len(e.src)
}

// setCaret moves the caret to off, keeping the anchor when extend is set,
// and leaves the phantom unless the caret stays in it.
func (e *Editor) setCaret(off int, extend bool) {
	off = max(0, min(off, len(e.src)))
	if e.phantom != nil && !(off == e.phantom.at && e.leafIsPhantom(off)) {
		e.dropPhantom()
	}
	e.caret = off
	if !extend {
		e.anchor = off
	}
	e.wantReveal = true
	e.compose = ""
	e.blinkStart = time.Now()
}

// leafIsPhantom reports whether the caret at off stays in the phantom.
func (e *Editor) leafIsPhantom(off int) bool {
	i := e.leafIndex(off)
	return i >= 0 && e.leaves[i].kind == kPhantom
}

// dropPhantom takes the phantom away, as the caret leaves it.
func (e *Editor) dropPhantom() {
	if e.phantom == nil {
		return
	}
	e.phantom = nil
	e.rev++
	e.parsed = 0
	e.sync()
}

// setPhantom puts the caret in a phantom paragraph.
func (e *Editor) setPhantom(p *phantom) {
	e.phantom = p
	e.rev++
	e.parsed = 0
	e.sync()
	e.caret, e.anchor = p.at, p.at
	e.wantReveal = true
	e.compose = ""
	e.blinkStart = time.Now()
}

// Select selects a range of the source, as a search's match, and scrolls
// to it.
func (e *Editor) Select(start, end int) {
	e.dropPhantom()
	start = max(0, min(start, len(e.src)))
	end = max(start, min(end, len(e.src)))
	e.anchor, e.caret = start, end
	e.wantReveal = true
	e.hasGoal = false
	e.invalidate()
}

// ScrollToOffset scrolls the source offset off to the top of the view and
// puts the caret there, as an outline's heading is gone to.
func (e *Editor) ScrollToOffset(off int) {
	e.dropPhantom()
	off = e.normalize(max(0, min(off, len(e.src))), 1)
	e.anchor, e.caret = off, off
	e.scrollTo, e.hasScrollTo, e.scrollCenter = off, true, false
	e.invalidate()
}

// scrollToOffsetCentered scrolls off to the middle of the view, as the
// rail's ticks do.
func (e *Editor) scrollToOffsetCentered(off int) {
	e.dropPhantom()
	off = e.normalize(max(0, min(off, len(e.src))), 1)
	e.anchor, e.caret = off, off
	e.scrollTo, e.hasScrollTo, e.scrollCenter = off, true, true
	e.invalidate()
}

// SetHighlights draws ranges of the source as found, current the one
// chosen (or -1).
func (e *Editor) SetHighlights(rs []Range, current int) {
	e.highlights, e.current = rs, current
	e.invalidate()
}

// Focus gives the editor the keyboard focus.
func (e *Editor) Focus() { e.handle.Focus() }

// Focused reports whether the editor has the keyboard focus.
func (e *Editor) Focused() bool { return e.focused }

func (e *Editor) invalidate() {
	if e.opts.Invalidate != nil {
		e.opts.Invalidate()
	}
}

// Headings returns the document's headings, in order: their level, text
// and offset.
func (e *Editor) Headings() []Heading {
	e.sync()
	var out []Heading
	for _, l := range e.leaves {
		if l.kind != kHeading || l.pending {
			continue
		}
		text := strings.TrimSpace(e.plainText(e.p.doc.Inlines(l.id)))
		out = append(out, Heading{Level: l.level, Text: text, Offset: l.start})
	}
	return out
}

// Heading is a heading of the document.
type Heading struct {
	Level  int
	Text   string
	Offset int
}
