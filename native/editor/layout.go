package editor

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"muse/editor/internal/parser"

	"muse/editor/internal/textlayout"
	"muse/markdown"
	appearance "muse/theme"
	"muse/ui/scrollbar"
)

// look is how the document looks, resolved from the window's theme and the
// editor's options as a frame builds.
type look struct {
	dark          bool
	smart         bool
	softBreaks    bool
	editingBreaks bool
	// size is the body text's, lh its line height as a multiple of it.
	size, lh   float32
	font, mono string
	text       ui.Color
	muted      ui.Color
	accent     ui.Color
	marker     ui.Color
	codeBg     ui.Color
	codeText   ui.Color
	panel      ui.Color
	border     ui.Color
	quoteBar   ui.Color
	selection  ui.Color
	caret      ui.Color
	markBg     ui.Color
	code       *editor.Theme
}

// lookKey is what of the look the shaped leaves depend on.
type lookKey struct {
	dark          bool
	smart         bool
	softBreaks    bool
	editingBreaks bool
	size, lh      float32
	font, mono    string
	text          ui.Color
	accent        ui.Color
}

func (lk *look) key() lookKey {
	return lookKey{dark: lk.dark, smart: lk.smart, softBreaks: lk.softBreaks, editingBreaks: lk.editingBreaks, size: lk.size, lh: lk.lh, font: lk.font, mono: lk.mono, text: lk.text, accent: lk.accent}
}

// setLook resolves the look from the theme and the options.
func (e *Editor) setLook(t *ui.Theme) {
	lk := &e.lk
	lk.dark = t.Dark
	lk.smart = !e.opts.DisableSmartTypography
	lk.softBreaks = e.opts.SoftBreaksAsSpaces
	lk.editingBreaks = e.opts.PreserveTypedLineBreaks
	lk.size = e.opts.FontSize
	if lk.size <= 0 {
		lk.size = 16
	}
	lk.lh = e.opts.LineHeight
	if lk.lh <= 0 {
		lk.lh = 1.7
	}
	lk.font = e.opts.Font
	if lk.font == "" {
		lk.font = t.Font
	}
	lk.mono = e.opts.MonoFont
	if lk.mono == "" {
		lk.mono = "monospace"
	}
	lk.text = markdown.BodyTextColor(t.Dark)
	lk.muted = t.TextMuted
	lk.accent = t.Accent
	lk.marker = markdown.MarkdownMarkerColor(t.Dark)
	lk.border = t.Border
	lk.selection = t.Selection
	lk.caret = t.Accent
	lk.codeBg, lk.codeText = markdown.InlineCodeColorsFor(t.Accent, t.Dark)
	lk.panel = markdown.CodePanelColor(t.Dark)
	lk.quoteBar = markdown.MarkdownRuleColor(t.Dark)
	// Palette.Soft is intentionally faint; lift it enough for Markdown marks
	// to remain visible at paragraph scale across every accent preset.
	lk.markBg = appearance.From(t.Accent, t.Dark).Soft.Alpha(2.1)
	lk.code = editor.DefaultTheme(t)
}

// span returns how text in style st looks, in a leaf of text of size.
func (lk *look) span(st istyle, size float32) ui.Span {
	s := ui.Span{Color: lk.text}
	if st.heading {
		s.Color = markdown.HeadingTextColor(lk.dark)
	}
	if st.muted {
		s.Color = lk.text.Mix(lk.muted, 0.25)
		if st.strike {
			s.Color = lk.muted
		}
	}
	if st.bold {
		s.Weight = 600
	}
	if st.italic {
		s.Italic = true
	}
	if st.strike {
		s.Strikethrough = true
		s.Color = lk.text.Mix(lk.muted, 0.4)
	}
	if st.under {
		s.Underline = true
	}
	if st.link {
		s.Color = markdown.InlineLinkColorFor(lk.accent, lk.dark)
		s.Underline = true
		if st.linkInvalid {
			s.Color = s.Color.Mix(lk.muted, 0.45)
			s.DottedUnderline = true
		}
		if st.linkHover {
			s.Color = appearance.From(lk.accent, lk.dark).Hover
		}
	}
	if st.mark {
		s.Background = lk.markBg
	}
	if st.code {
		s.Font = lk.mono
		s.Size = size * 0.875
		codeBg, codeText := markdown.InlineCodeColorsFor(lk.accent, lk.dark)
		if st.codeJoined {
			// Inline code ranges receive one shared background in paint.go.
			s.Background = ui.Color{}
		} else {
			s.Background = codeBg
			s.Radius = max(3, size/5)
			s.PaddingX = size * 0.2
			s.PaddingY = size * 0.1
		}
		if !st.link {
			s.Color = codeText
		}
	}
	if st.kbd {
		s.Font = lk.mono
		s.Size = size * 0.85
		s.Background = lk.codeBg
		s.Radius = 3
	}
	if st.mono {
		s.Font = lk.mono
		s.Size = size * 0.9
	}
	if st.color.A != 0 {
		s.Color = st.color
	}
	if st.bg.A != 0 {
		s.Background = st.bg
	}
	if st.sup || st.sub {
		s.Size = size * 0.75
		s.BaselineShift = size * 0.35
		if st.sub {
			s.BaselineShift = -size * 0.2
		}
	}
	if st.marker {
		if !st.codeJoined {
			s.Font = lk.font
			s.Background = ui.Color{}
			s.Radius = 0
			s.PaddingX, s.PaddingY = 0, 0
		}
		s.Italic = false
		s.Color = lk.marker
		s.Underline, s.Strikethrough = false, false
	}
	return s
}

// headingSize returns the size of the text of a heading of level.
func (lk *look) headingSize(level int) float32 { return markdown.HeadingSize(level, lk.size) }

func headingWeight(level int) int { return markdown.HeadingWeight(level) }

// Spacing, in ems of the body text.
const (
	quoteIndent = 1.2
	itemIndent  = 1.75
	codePadX    = 1.0
	codePadY    = 0.875
	cellPadX    = 0.7
	cellPadY    = 0.55
)

// leafView is a leaf as the editor shows it: its display, laid out, and
// the boxes around it, relative to the leaf's box.
type leafView struct {
	disp   display
	layout *textlayout.Layout
	params viewParams
	// empty tells a layout of nothing: shaped from a space, for its height.
	empty bool
	// tx and ty place the layout in the leaf's box.
	tx, ty float32
	h      float32
	// panel fills the box behind the text: code, math being edited, front
	// matter.
	panel bool
	// block is an object drawn below the text, or alone: a formula, an
	// image of HTML; bx and by place it.
	block  *object
	bx, by float32
	// label is a code block's language, drawn in its corner.
	label string
	// reveal tells a block showing its source because the caret is in
	// it: a rule, a formula, a fence's info string.
	reveal bool
	// natural is the width of a table cell's text on one line.
	natural float32
	// font is the text's, which the caret is as tall as.
	font ui.Font
	// diagram is a Mermaid diagram drawn instead of its code; diagX is how
	// far it is scrolled sideways when it is wider than the column.
	diagram       *markdown.Diagram
	diagX         float32
	diagIndicator scrollbar.Indicator
	diagBarShown  bool
	used          uint64
}

// hasImageAsset reports whether keeping this layout alive also keeps decoded
// pixels alive. Image assets have their own bounded process-wide cache, so an
// offscreen leaf should not retain another copy through the editor's view LRU.
func (v *leafView) hasImageAsset() bool {
	if v == nil {
		return false
	}
	if v.block != nil && v.block.asset != nil {
		return true
	}
	for _, piece := range v.disp.pieces {
		if piece.obj != nil && piece.obj.asset != nil {
			return true
		}
	}
	return false
}

// viewKey is what a leaf's view is made of: views of the same key are the
// same, and the editor keeps them across frames.
type viewKey struct {
	typedBreaks         string
	kind                leafKind
	level               int
	flags               uint8
	width               float32
	reveal              int
	headingMarkerReveal bool
	mathRange           [2]int
	selection           [2]int
	sourceReveal        uint64
	blockRev            uint64
	compose             string
	composeAt           int
	text                string
	look                lookKey
	assets              uint64
	linkRev             uint64
	linkContext         string
	hoverLink           Range
	align               parser.Align
}

// viewParams is what a view depends on besides the leaf's text. A leaf
// keeps its view while these stay equal and its content does not change
// (see sameLeaf), which is what makes laying out an edited document cost
// nothing for the leaves the edit did not touch.
type viewParams struct {
	typedBreaks         string
	kind                leafKind
	level               int
	flags               uint8
	width               float32
	reveal              int
	headingMarkerReveal bool
	mathRange           [2]int
	selection           [2]int
	sourceReveal        uint64
	blockRev            uint64
	compose             string
	composeAt           int
	look                lookKey
	assets              uint64
	linkRev             uint64
	linkContext         string
	hoverLink           Range
	align               parser.Align
}

func (k *viewKey) params() viewParams {
	return viewParams{
		typedBreaks: k.typedBreaks,
		kind:        k.kind, level: k.level, flags: k.flags, width: k.width,
		reveal: k.reveal, headingMarkerReveal: k.headingMarkerReveal, mathRange: k.mathRange, selection: k.selection, sourceReveal: k.sourceReveal, blockRev: k.blockRev, compose: k.compose, composeAt: k.composeAt,
		look: k.look, assets: k.assets, linkRev: k.linkRev, linkContext: k.linkContext, hoverLink: k.hoverLink, align: k.align,
	}
}

const (
	fPending uint8 = 1 << iota
	fSetext
	fHeader
	fFront
	fQuote
	fNatural
	// fSource is a block showing its source: its view is not the view it
	// draws, so it is not the view to keep across frames.
	fSource
	fFolded
)

// leafFlags returns the flags of a leaf's view key, the editor's own above
// the leaf's.
func (e *Editor) leafFlags(l *leaf, header bool) uint8 {
	f := leafFlags(l, header)
	if e.sourceOK && e.sourceAt == l.lineStart {
		f |= fSource
	}
	if e.isFoldedHeading(l) {
		f |= fFolded
	}
	return f
}

// leafFlags returns the flags of a leaf's key.
func leafFlags(l *leaf, header bool) uint8 {
	var f uint8
	if l.pending {
		f |= fPending
	}
	if l.setext {
		f |= fSetext
	}
	if header {
		f |= fHeader
	}
	if l.frontMatter {
		f |= fFront
	}
	for _, c := range l.conts {
		if c.kind == cQuote {
			f |= fQuote
			break
		}
	}
	return f
}

// revealAt returns the caret's offset relative to the leaf's first line
// when it shows the syntax of the leaf, or -1.
func (e *Editor) revealAt(l *leaf) int {
	return e.revealFor(l, e.revealCaret())
}

// view returns the leaf's view at width, as it shows with the caret where
// it is. The view is kept on the leaf: the leaf the next layout sees with
// the same content and parameters keeps it (see sameLeaf).
func (e *Editor) view(l *leaf, width float32, header bool, align parser.Align) *leafView {
	reveal := e.revealAt(l)
	if _, end, ok := e.calloutHeader(l); ok && e.caret >= l.lineStart && e.caret <= end {
		reveal = e.caret - l.lineStart
	}
	return e.viewAt(l, width, header, align, reveal, true)
}

// viewWith returns the leaf's view at width as it shows with the caret at
// reveal (relative to the leaf's first line; -1 for elsewhere), from the
// cache when it has it. It does not keep the view on the leaf: it may be a
// view for a caret the leaf does not have (see unitView).
func (e *Editor) viewWith(l *leaf, width float32, header bool, align parser.Align, reveal int) *leafView {
	return e.viewAt(l, width, header, align, reveal, false)
}

// viewAt returns the leaf's view, and keeps it on the leaf when settle is
// set.
func (e *Editor) viewAt(l *leaf, width float32, header bool, align parser.Align, reveal int, settle bool) *leafView {
	hoverLink := Range{}
	if e.hoverLink.End > e.hoverLink.Start && e.hoverLink.Start < l.end && e.hoverLink.End > l.lineStart {
		hoverLink = e.hoverLink
	}
	k := viewKey{
		typedBreaks:         e.typedBreakKey(l),
		kind:                l.kind,
		level:               l.level,
		flags:               e.leafFlags(l, header),
		width:               width,
		reveal:              reveal,
		headingMarkerReveal: e.opts.HeadingMarkerReveal,
		look:                e.lk.key(),
		blockRev: func() uint64 {
			if e.block != nil {
				return e.block.revision
			}
			return 0
		}(),
		assets:      e.assetGen.Load(),
		linkRev:     e.opts.LinkRevision,
		linkContext: e.opts.LinkContext,
		hoverLink:   hoverLink,
		align:       align,
	}
	at := -1
	if reveal >= 0 {
		at = reveal + l.lineStart
	}
	if start, end, ok := e.inlineMathRange(l, at); ok {
		k.mathRange = [2]int{start - l.lineStart, end - l.lineStart}
	}
	if a, z := e.textSelectionForUnit(l); a != z && a < l.end && z > l.lineStart {
		k.selection = [2]int{max(a, l.lineStart) - l.lineStart, min(z, l.end) - l.lineStart}
	}
	k.sourceReveal = e.sourceRevealFingerprint(l.lineStart, blockEnd(e.src, l))
	// Leaves can meet at the caret: a heading and the empty paragraph Enter
	// opens after it end and start at the same offset. Only the unit the
	// caret is in shows the composition, just as only it shows the caret.
	withCompose := e.compose != "" && at == e.caret && e.unitAt(e.caret) == l
	if withCompose {
		k.compose, k.composeAt = e.compose, e.caret-l.lineStart
	}
	p := k.params()
	// The leaf's own view is the one it had the last time it was laid out:
	// a leaf the edit did not change keeps it, and its text is not hashed
	// again for the cache below.
	if settle && l.view != nil && l.view.params == p {
		l.view.used = e.frame
		return l.view
	}
	k.text = viewText(e.src, l)
	if v, ok := e.views[k]; ok {
		v.used = e.frame
		v.params = p
		if settle {
			l.view = v
		}
		return v
	}
	var v *leafView
	switch l.kind {
	case kCode:
		v = e.codeView(l, width, at, withCompose)
	case kMath:
		v = e.mathView(l, width, at, withCompose)
	case kHTML:
		v = e.htmlView(l, width, at, withCompose)
	case kRaw:
		if _, _, ok := e.calloutHeader(l); ok {
			v = e.textView(l, width, at, withCompose, header, align)
		} else {
			v = e.linesView(l, width, at, withCompose, l.frontMatter)
		}
	case kRule:
		v = e.ruleView(l, width, at, withCompose)
	default:
		v = e.textView(l, width, at, withCompose, header, align)
	}
	v.used = e.frame
	v.params = p
	if settle {
		l.view = v
	}
	if !v.hasImageAsset() {
		e.views[k] = v
	}
	return v
}

// newBuilder returns a builder of the display of leaf l, with the caret
// at at (-1 for elsewhere), and the composition there when withCompose.
func (e *Editor) newBuilder(l *leaf, at int, withCompose bool, size, width float32) *builder {
	b := &builder{e: e, src: e.src, base: l.lineStart, reveal: at, fontSize: size, width: width, hardBreakAt: -1}
	if _, end, ok := e.calloutHeader(l); ok {
		b.hardBreakAt = end
	}
	if withCompose {
		b.compose, b.composeAt = e.compose, e.caret
	}
	return b
}

// textFont returns the font of a text leaf.
func (e *Editor) textFont(l *leaf) ui.Font {
	f := ui.Font{Family: e.lk.font, Size: e.lk.size, LineHeight: e.lk.lh}
	if l.kind == kHeading && !l.pending {
		f.Size = e.lk.headingSize(l.level)
		f.Weight = headingWeight(l.level)
		f.LineHeight = markdown.HeadingLineHeight(l.level)
	}
	return f
}

// textView lays out a paragraph, a heading, a table cell or the phantom.
func (e *Editor) textView(l *leaf, width float32, at int, withCompose, header bool, align parser.Align) *leafView {
	f := e.textFont(l)
	b := e.newBuilder(l, at, withCompose, f.Size, width)
	b.quoteDepth = quoteDepth(l)
	b.quotePrefixMode = e.quotePrefixOpenAt(l, at-l.lineStart)
	containerPrefixVisible := false
	if s, end, ok := containerPrefixRange(l); ok {
		containerPrefixVisible = b.containerPrefixRevealed(s, end, l.end)
	}
	var quoteFirst Range
	quoteFirstVisible := false
	for _, r := range quoteSourcePrefixes(e, l) {
		if r.Start == l.lineStart {
			quoteFirst = r
			quoteFirstVisible = b.quotePrefixRevealed(r.Start, r.End)
			break
		}
	}
	if b.quotePrefixMode {
		b.prefixIndent = e.quotePrefixShift(l)
	}
	st := istyle{}
	if l.kind == kHeading && !l.pending {
		st.heading = true
	}
	if header {
		st.bold = true
	}
	for _, c := range l.conts {
		if c.kind == cQuote {
			b.preserveSoftBreaks = true
		}
		if c.kind == cQuote && !isCalloutContainer(c) {
			st.muted = true
		}
	}
	if task, ok := l.innerItem(); ok && task.task == parser.Checked && l.opensItem() {
		st.muted, st.strike = true, true
	}
	switch {
	case l.pending || l.isLine() || l.id < 0:
		if l.isLine() && b.quotePrefixMode && quoteFirst.End > quoteFirst.Start {
			if b.quotePrefixRevealed(quoteFirst.Start, quoteFirst.End) {
				b.marker(quoteFirst.Start, quoteFirst.End, st)
			} else {
				b.prefixSpacer(l.start, b.prefixIndent)
			}
			b.finish(l.end)
		} else {
			b.textRun(l.start, l.end, st)
			b.finish(l.end)
		}
	default:
		ins := e.viewInlines(l)
		b.alone = aloneImage(ins, e.src)
		b.starts, b.cell = e.lineStarts(l.id), l.table != nil
		// A heading's `#`s are chrome, hidden like every other block marker —
		// but the caret in its source prefix shows them, so the level can be
		// edited in place. In the optional boundary mode, the first heading
		// character itself stays rendered as a heading; moving left onto the
		// separating space reveals the prefix and exposes its source positions.
		revealHeading := at >= l.lineStart && at <= l.end
		if e.opts.HeadingMarkerReveal {
			emptyHeading := !l.pending && strings.TrimSpace(e.src[l.start:l.end]) == ""
			revealHeading = emptyHeading || at >= l.lineStart && at < l.start
		}
		if l.kind == kHeading && !l.pending && (revealHeading || sourceRangeTargeted(e, l.lineStart, l.end, false)) {
			b.marker(l.lineStart, l.start, st)
		}
		prefixVisible := false
		if s, end, ok := containerPrefixRange(l); ok && e.tableOf(l) == nil {
			prefixVisible = containerPrefixVisible
			if prefixVisible {
				b.marker(s, end, st)
			}
		}
		if !prefixVisible && quoteFirst.End > quoteFirst.Start && quoteFirstVisible {
			b.marker(quoteFirst.Start, quoteFirst.End, st)
			prefixVisible = true
		}
		if b.quotePrefixMode && !prefixVisible {
			if _, _, calloutHeader := e.calloutHeader(l); !calloutHeader {
				b.prefixSpacer(l.start, b.prefixIndent)
			}
		}
		b.inlines(ins, l.start, l.end, st)
		if l.kind == kHeading && e.isFoldedHeading(l) {
			b.deco(l.end, " …", ui.Span{Color: e.lk.muted, Size: f.Size * 0.9}, nil)
		}
	}
	if start, end, ok := e.inlineMathRange(l, at); ok {
		// Parsed formulas already have the correct inherited inline style.
		parsed := false
		if l.id >= 0 {
			for _, in := range e.p.doc.Inlines(l.id) {
				if in.Kind == parser.Math && in.Start == start && in.End == end {
					parsed = true
					break
				}
			}
		}
		if !parsed {
			b.rawRange(start, end, st)
		}
	}
	e.projectCalloutHeader(b, l, at)
	b.ensure(l.start)
	v := &leafView{disp: b.display()}
	shape := ui.TextShape{Width: width}
	switch align {
	case parser.AlignCenter:
		shape.Align = ui.Center
	case parser.AlignRight:
		shape.Align = ui.End
	}
	if b.alone {
		shape.Align = ui.Start
		for _, pc := range v.disp.pieces {
			if pc.obj != nil && pc.obj.block {
				if pc.obj.alignCenter {
					shape.Align = ui.Center
				}
				break
			}
		}
	}
	smart := e.lk.smart && l.kind == kPara && l.table == nil && !b.alone && !e.composing
	var cache *textlayout.Cache
	if smart {
		cache = &e.typographyCache
	}
	v.layout, v.empty = shapeDisplay(&v.disp, f, shape, cache)
	v.font = f
	_, v.h = v.layout.Size()
	if width == 0 {
		v.natural, _ = v.layout.Size()
	}
	return v
}

// shapeDisplay lays out a display, or a space for an empty one.
func shapeDisplay(d *display, f ui.Font, shape ui.TextShape, cache ...*textlayout.Cache) (*textlayout.Layout, bool) {
	if d.text == "" {
		return textlayout.Shape([]ui.Span{{Text: " "}}, f, shape, false), true
	}
	spans := make([]ui.Span, 0, len(d.pieces))
	for _, p := range d.pieces {
		if p.text == "" {
			continue
		}
		sp := p.span
		sp.Text = p.text
		spans = append(spans, sp)
	}
	if len(cache) > 0 && cache[0] != nil {
		return cache[0].Shape(spans, f, shape), false
	}
	return textlayout.Shape(spans, f, shape, false), false
}

// lineStarts returns the offsets of the lines of block id's text, when it
// has more than one.
func (e *Editor) lineStarts(id int) []int {
	n := e.p.doc.LineCount(id)
	if n < 2 {
		return nil
	}
	out := make([]int, n)
	for k := range out {
		_, out[k] = e.p.doc.Line(id, k)
	}
	return out
}

// aloneImage reports whether a paragraph's inlines are one image and
// nothing else but spaces.
func aloneImage(ins []parser.Inline, src string) bool {
	img := false
	for i := 0; i < len(ins); i = max(ins[i].Next, i+1) {
		switch ins[i].Kind {
		case parser.Image:
			if img {
				return false
			}
			img = true
		case parser.Text:
			if strings.TrimSpace(src[ins[i].Start:ins[i].End]) != "" {
				return false
			}
		default:
			return false
		}
	}
	return img
}

// codeFont returns the font of code.
func (e *Editor) codeFont() ui.Font {
	return ui.Font{Family: e.lk.mono, Size: e.lk.size * 0.875, LineHeight: 1.6}
}

// isMermaid reports whether l is a fenced block of Mermaid.
func isMermaid(l *leaf) bool {
	return l.kind == kCode && l.fenced && strings.EqualFold(l.language(), "mermaid")
}

// codeView lays out a block of code: its lines, highlighted, in a panel,
// with its fence's info string above them while the caret is there.
func (e *Editor) codeView(l *leaf, width float32, at int, withCompose bool) *leafView {
	codeLines := l.codeLines()
	codeRanges, codeStart, codeEnd, compactCode := codeLayoutRanges(codeLines, at)
	f := e.codeFont()
	padX, padY := e.lk.size*codePadX, e.lk.size*codePadY
	inner := max(width-2*padX, 40)
	b := e.newBuilder(l, at, withCompose, f.Size, inner)
	caretIn := at >= 0
	if l.whole {
		padX, padY = 0, 0
		inner = max(width, 40)
		b.width = inner
	}
	v := &leafView{panel: !l.whole, tx: padX, ty: padY, label: l.language()}
	if l.whole {
		v.label = ""
	}
	plain := ui.Span{Color: e.lk.text}
	// The block's source shows: its control was used on it, or the caret is
	// on its fence's info string. Then the fence closes the block on screen
	// as it does in the file — a source that stops at the last line of code
	// is half a fence.
	open := e.codeFenceSourceShown(l)
	infoLine := l.fenced && caretIn && b.reveal >= l.infoStartOffset() && b.reveal <= l.infoEndOffset()
	fenceShown := open || infoLine
	var mermaidDiagram *markdown.Diagram
	if !open && !caretIn && isMermaid(l) && !compactCode {
		if d, ok := markdown.ParseDiagramThemed(e.codeText(l), e.sketchOptions()); ok {
			mermaidDiagram = &d
		}
	}
	if fenceShown {
		mark := ui.Span{Color: e.lk.marker}
		fence := strings.TrimLeft(e.src[l.lineStart:l.fenceEndOffset()], " \t>")
		b.deco(l.infoStartOffset(), fence, mark, nil)
		b.spanRun(l.infoStartOffset(), l.infoEndOffset(), ui.Span{Color: e.lk.muted})
		if len(codeLines) > 0 {
			b.spanAtom(l.infoEndOffset(), codeLines[0].off, "\n", plain)
		}
		// Only the caret on the fence itself is the block's editing state;
		// a block opened by its control keeps its tag and its diagram
		// control.
		v.reveal = infoLine
	}
	lang := editor.LanguageNamed(l.language())
	var state uint64
	var toks []editor.Token
	previousEnd, previousLine, wroteCode := codeStart, -1, false
	for k, ln := range codeLines {
		lineEnd := ln.off + len(ln.text)
		if compactCode && !codeLineInRanges(ln, codeRanges) {
			continue
		}
		if !wroteCode {
			if ln.off > codeStart {
				b.spanAtom(codeStart, ln.off, codeOmittedLabel(ln.off-codeStart), ui.Span{Color: e.lk.muted})
			}
		} else if k == previousLine+1 {
			b.spanAtom(previousEnd, ln.off, "\n", plain)
		} else if ln.off > previousEnd {
			b.spanAtom(previousEnd, ln.off, codeOmittedLabel(ln.off-previousEnd), ui.Span{Color: e.lk.muted})
		}
		wroteCode = true
		previousEnd, previousLine = lineEnd, k
		if len(ln.text) > maxShapedCodeLineBytes {
			appendCodeLineWindow(b, ln, at, plain, ui.Span{Color: e.lk.muted})
			continue
		}
		if strings.EqualFold(strings.TrimSpace(l.language()), "mermaid") && (open || caretIn || mermaidDiagram == nil) {
			renderSourceTokens(b, ln.off, ln.text, mermaidSourceTokens(ln.text), plain)
			continue
		}
		if lang == nil || lang.Tokenizer == nil {
			b.codeRun(ln.off, ln.off+len(ln.text), plain)
			continue
		}
		toks, state = lang.Tokenizer.Tokenize(ln.text, state, toks[:0])
		at := 0
		for _, tk := range toks {
			end := min(tk.End, len(ln.text))
			if end <= at {
				continue
			}
			sp := plain
			if c := e.lk.code.TokenColor(tk.Kind); c.A != 0 {
				sp.Color = c
			}
			ts := e.lk.code.Tokens[tk.Kind]
			sp.Italic = ts.Italic
			if ts.Bold {
				sp.Weight = 700
			}
			b.codeRun(ln.off+at, ln.off+end, sp)
			at = end
		}
		b.codeRun(ln.off+at, ln.off+len(ln.text), plain)
	}
	if compactCode && wroteCode && previousEnd < codeEnd {
		b.spanAtom(previousEnd, codeEnd, codeOmittedLabel(codeEnd-previousEnd), ui.Span{Color: e.lk.muted})
	}
	if fenceShown && l.fenced {
		// The closing fence is part of the source while the block is open.
		// Keep its real source range so the caret and selections can occupy
		// this row just as they can occupy the opening fence's info line.
		if at := lineEndAt(e.src, l.end) + 1; at <= len(e.src) {
			if end := blockEnd(e.src, l); end > at {
				b.spanAtom(l.end, at, "\n", plain)
				closeEnd := end
				if closeEnd > at && e.src[closeEnd-1] == '\r' {
					closeEnd--
				}
				if closeEnd > at {
					b.add(piece{kind: pText, s0: at - b.base, s1: closeEnd - b.base,
						text: e.src[at:closeEnd], span: ui.Span{Color: e.lk.marker}})
				}
			}
		}
	}
	b.finish(l.end)
	b.ensure(l.start)
	v.disp = b.display()
	v.layout, v.empty = shapeDisplay(&v.disp, f, ui.TextShape{})
	v.font = f
	_, th := v.layout.Size()
	v.h = th + 2*padY
	if mermaidDiagram != nil {
		v.diagram = mermaidDiagram
		v.h = mermaidDiagram.Height(width) + 2*padY
	}
	return v
}

const maxShapedCodeLineBytes = 4096
const maxShapedCodeBlockBytes = 16 << 10

type codeLayoutRange struct{ start, end int }

func codeLayoutRanges(lines []line, caret int) (ranges []codeLayoutRange, start, end int, compact bool) {
	if len(lines) == 0 {
		return nil, 0, 0, false
	}
	start = lines[0].off
	last := lines[len(lines)-1]
	end = last.off + len(last.text)
	if end-start <= maxShapedCodeBlockBytes {
		return []codeLayoutRange{{start, end}}, start, end, false
	}
	half := maxShapedCodeBlockBytes / 2
	if caret >= start && caret <= end {
		windowStart := max(start, caret-half)
		windowEnd := min(end, windowStart+maxShapedCodeBlockBytes)
		windowStart = max(start, windowEnd-maxShapedCodeBlockBytes)
		return []codeLayoutRange{{windowStart, windowEnd}}, start, end, true
	}
	return []codeLayoutRange{{start, start + half}, {end - half, end}}, start, end, true
}

func codeLineInRanges(ln line, ranges []codeLayoutRange) bool {
	lineEnd := ln.off + len(ln.text)
	for _, r := range ranges {
		if ln.off < r.end && lineEnd > r.start || len(ln.text) == 0 && ln.off >= r.start && ln.off < r.end {
			return true
		}
	}
	return false
}

func codeOmittedLabel(byteCount int) string {
	return fmt.Sprintf("… %d source bytes omitted …", byteCount)
}

// appendCodeLineWindow keeps opaque, unbroken code payloads editable without
// asking CoreText to allocate per-glyph layout state for the entire source.
// The caret gets a window around its source offset; otherwise the display
// keeps both ends and marks the omitted range as one source atom.
func appendCodeLineWindow(b *builder, ln line, caret int, text, muted ui.Span) {
	sourceLen := len(ln.text)
	window := min(sourceLen, maxShapedCodeLineBytes)
	if caret < ln.off || caret > ln.off+sourceLen {
		prefixEnd := window / 2
		suffixStart := sourceLen - (window - prefixEnd)
		for !utf8.RuneStart(ln.text[prefixEnd]) {
			prefixEnd--
		}
		for suffixStart < sourceLen && !utf8.RuneStart(ln.text[suffixStart]) {
			suffixStart++
		}
		b.codeRun(ln.off, ln.off+prefixEnd, text)
		if suffixStart > prefixEnd {
			b.spanAtom(ln.off+prefixEnd, ln.off+suffixStart,
				fmt.Sprintf(" … %d source bytes omitted … ", suffixStart-prefixEnd), muted)
		}
		b.codeRun(ln.off+suffixStart, ln.off+sourceLen, text)
		return
	}
	center := caret - ln.off
	start := max(0, center-window/2)
	end := min(sourceLen, start+window)
	start = max(0, end-window)
	for start > 0 && !utf8.RuneStart(ln.text[start]) {
		start--
	}
	for end < sourceLen && !utf8.RuneStart(ln.text[end]) {
		end++
	}
	if start > 0 {
		b.spanAtom(ln.off, ln.off+start, "…", muted)
	}
	b.codeRun(ln.off+start, ln.off+end, text)
	if end < sourceLen {
		label := fmt.Sprintf(" … %d source bytes omitted … ", sourceLen-end-start)
		b.spanAtom(ln.off+end, ln.off+sourceLen, label, muted)
	}
}

// codeText returns the text of a code or math leaf's lines.
func (e *Editor) codeText(l *leaf) string {
	var sb strings.Builder
	for k, ln := range l.codeLines() {
		if k > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(ln.text)
	}
	return sb.String()
}

// mathView lays out a block of math: typeset, or its TeX above the
// typeset formula while the caret is in it.
func (e *Editor) mathView(l *leaf, width float32, at int, withCompose bool) *leafView {
	caretIn := at >= 0
	sourceTargeted := sourceRangeTargeted(e, l.lineStart, blockEnd(e.src, l), false)
	tex := e.codeText(l)
	obj := e.mathBlockObject(tex, 0)
	if sourceTargeted && !caretIn {
		return e.markdownBlockSourceView(l, width)
	}
	if !caretIn && !sourceTargeted {
		v := &leafView{block: obj}
		if obj == nil {
			// Nothing to typeset, or TeX the typesetter cannot read: its
			// source shows, in the panel of code.
			return e.linesView(l, width, -1, false, true)
		}
		pad := e.lk.size * 0.5
		v.bx, v.by = max((width-obj.w)/2, 0), pad
		v.h = obj.h + 2*pad
		return v
	}
	v := e.linesView(l, width, at, withCompose, true)
	v.reveal = true
	if obj != nil {
		gap := e.lk.size * 0.6
		v.block = obj
		v.bx, v.by = max((width-obj.w)/2, 0), v.h+gap
		v.h += obj.h + 2*gap
	}
	return v
}

func (e *Editor) markdownBlockSourceView(l *leaf, width float32) *leafView {
	f := e.codeFont()
	b := e.newBuilder(l, -1, false, f.Size, width)
	end := blockEnd(e.src, l)
	b.spanRun(l.lineStart, end, ui.Span{Color: e.lk.text})
	b.finish(end)
	v := &leafView{disp: b.display(), panel: true, tx: e.lk.size * codePadX, ty: e.lk.size * codePadY, reveal: true}
	inner := max(width-2*v.tx, 40)
	v.layout, v.empty = shapeDisplay(&v.disp, f, ui.TextShape{Width: inner})
	v.font = f
	_, h := v.layout.Size()
	v.h = h + 2*v.ty
	return v
}

// htmlView lays out a block of HTML: its source, or the image of a block
// of just an image while the caret is elsewhere.
func (e *Editor) htmlView(l *leaf, width float32, at int, withCompose bool) *leafView {
	html := e.codeText(l)
	obj := e.htmlImageObject(html, width)
	sourceTargeted := sourceRangeTargeted(e, l.lineStart, blockEnd(e.src, l), false)
	if obj != nil {
		obj.sourceFrom, obj.sourceTo, obj.imageSyntax = l.start, l.end, imageSyntaxHTML
		obj.width = obj.w
		e.applyImagePreview(obj, l.start, l.end)
		if at < 0 && !sourceTargeted {
			return &leafView{block: obj, bx: (width - obj.w) / 2, h: obj.h}
		}
		if at < 0 {
			return e.linesView(l, width, at, withCompose, false)
		}
		v := e.linesView(l, width, at, withCompose, false)
		gap := e.lk.size * 0.5
		v.block, v.bx, v.by = obj, (width-obj.w)/2, v.h+gap
		v.h += obj.h + gap
		return v
	}
	if at < 0 && !sourceTargeted {
		if v := e.htmlTextView(l, width); v != nil {
			return v
		}
		// A block that reads as nothing, as a closing tag on a line of its
		// own does, draws nothing while the caret is elsewhere. Its source
		// shows as soon as the caret enters it, through linesView below.
		return &leafView{}
	}
	return e.linesView(l, width, at, withCompose, false)
}

// htmlTextView lays a block of HTML out as the HTML reads, or returns nil
// when it holds nothing to draw, as a closing tag alone does.
func (e *Editor) htmlTextView(l *leaf, width float32) *leafView {
	f := e.textFont(l)
	b := e.newBuilder(l, -1, false, f.Size, width)
	htmlBlockDisplay(b, l)
	if b.text.Len() == 0 {
		return nil
	}
	b.finish(l.end)
	b.ensure(l.start)
	v := &leafView{disp: b.display()}
	v.layout, v.empty = shapeDisplay(&v.disp, f, ui.TextShape{Width: width})
	v.font = f
	_, v.h = v.layout.Size()
	return v
}

// linesView lays out a leaf's lines as they are, in the code font: in a
// panel, or muted.
func (e *Editor) linesView(l *leaf, width float32, at int, withCompose, panel bool) *leafView {
	f := e.codeFont()
	inner := width
	v := &leafView{panel: panel}
	if panel {
		v.tx, v.ty = e.lk.size*codePadX, e.lk.size*codePadY
		inner = max(width-2*v.tx, 40)
	}
	b := e.newBuilder(l, at, withCompose, f.Size, inner)
	sp := ui.Span{Color: e.lk.muted}
	if panel {
		sp.Color = e.lk.text
	}
	for k, ln := range l.codeLines() {
		if k > 0 {
			prev := l.codeLines()[k-1]
			b.spanAtom(prev.off+len(prev.text), ln.off, "\n", sp)
		}
		if l.kind == kMath {
			renderSourceTokens(b, ln.off, ln.text, latexSourceTokens(ln.text), sp)
		} else {
			b.codeRun(ln.off, ln.off+len(ln.text), sp)
		}
	}
	b.finish(l.end)
	b.ensure(l.start)
	v.disp = b.display()
	v.layout, v.empty = shapeDisplay(&v.disp, f, ui.TextShape{Width: inner})
	v.font = f
	_, th := v.layout.Size()
	v.h = th + 2*v.ty
	return v
}

// ruleView lays out a thematic break: a line, or its source while the
// caret is at it.
func (e *Editor) ruleView(l *leaf, width float32, at int, withCompose bool) *leafView {
	if at < 0 && !sourceRangeTargeted(e, l.lineStart, blockEnd(e.src, l), false) {
		return &leafView{h: e.lk.size * 1.2}
	}
	f := ui.Font{Family: e.lk.font, Size: e.lk.size, LineHeight: e.lk.lh}
	b := e.newBuilder(l, at, withCompose, f.Size, width)
	b.spanRun(l.start, l.end, ui.Span{Color: e.lk.marker})
	b.finish(l.end)
	b.ensure(l.start)
	v := &leafView{disp: b.display(), reveal: true}
	v.layout, v.empty = shapeDisplay(&v.disp, f, ui.TextShape{Width: width})
	v.font = f
	_, v.h = v.layout.Size()
	return v
}

// ---------------------------------------------------------------------------
// The document

// indentOf returns how far the containers of a leaf move it in.
func (e *Editor) indentOf(conts []cont) float32 {
	x := float32(0)
	for _, c := range conts {
		switch c.kind {
		case cQuote:
			if isCalloutContainer(c) {
				x += e.lk.size * 1.4
			} else {
				x += e.lk.size * quoteIndent
			}
		case cFootnote:
			x += e.footnoteIndent(c)
		default:
			x += e.lk.size * itemIndent
		}
	}
	return x
}

func (e *Editor) footnoteIndent(c cont) float32 {
	sp := ui.Span{Text: c.label + ": ", Size: e.lk.size, Color: e.lk.accent}
	l := ui.ShapeRichTextWith([]ui.Span{sp}, ui.Font{Family: e.lk.font, Size: e.lk.size}, ui.TextShape{})
	w, _ := l.Size()
	return max(w, e.lk.size*itemIndent)
}

// margins returns the room above and below a leaf.
func (e *Editor) margins(l *leaf) (top, bottom float32) {
	s := e.lk.size
	for _, i := range l.firstOf {
		if i < len(l.conts) && isCalloutContainer(l.conts[i]) {
			return s * .7, s * .7
		}
	}
	switch l.kind {
	case kHeading:
		if l.pending {
			return s * 0.5, s * 0.5
		}
		return markdown.HeadingTopMargin(l.level, s), s * 0.35
	case kCode, kMath, kTable, kHTML:
		return s * 0.6, s * 0.6
	case kRule:
		return s * 0.6, s * 0.6
	case kRaw:
		return s * 0.6, s * 0.6
	case kPhantom, kBlank:
		// A line of its own: the line is the space, and it has no margins
		// of its own to add to the lines around it.
		return 0, 0
	}
	return s * 0.5, s * 0.5
}

// tightAfter reports whether a leaf follows the one before it closely: an
// item of a tight list, or the content of an item after its first leaf.
func tightAfter(prev, l *leaf) bool {
	if prev == nil || prev.isLine() {
		// A blank line is not an item of a list, whatever item it sits in.
		return false
	}
	if it, ok := l.innerItem(); ok && it.tight {
		if _, pin := prev.innerItem(); pin {
			return true
		}
	}
	return false
}

// layoutDoc lays the leaves out one below the other, in the content
// column of the element, from x to x+width. When virtual is set it shapes
// only the leaves near the viewport and the one the caret is in, giving
// the rest an estimated height: the text of a large document is not
// shaped off screen, where nobody reads it.
func (e *Editor) layoutDoc(x, width float32, virtual bool) {
	e.virtualIndex.Reset()
	// Only the real caret starts a session. Speculative navigation views
	// must be pure projections and must not replace the editing interval.
	e.activeInlineMath()
	if virtual && (width != e.estW || e.lk.size != e.estSize || e.lk.lh != e.estLH) {
		// The heights below were estimated for another width or text size,
		// where the text wrapped another way: measure them again.
		for _, l := range e.leaves {
			l.h = 0
		}
	}
	e.estW, e.estSize, e.estLH = width, e.lk.size, e.lk.lh
	lk, assets := e.lk.key(), e.assetGen.Load()
	// The rail's ticks are the headings of this parse, in order: a heading
	// still being typed is no heading yet, and has no tick until it is.
	rebuildRail := e.railRev != e.rev
	if rebuildRail {
		e.railLeaves = e.railLeaves[:0]
	}
	if !e.visibleIndexReady || e.visibleIndexRev != e.rev || e.visibleIndexFolds != e.foldRevision {
		e.visibleLeaves = e.visibleLeaves[:0]
		// This index contains at most one pointer per source leaf. Reserve its
		// final size once so a large document does not repeatedly copy the
		// growing pointer array while building the first layout.
		if cap(e.visibleLeaves) < len(e.leaves) {
			e.visibleLeaves = make([]*leaf, 0, len(e.leaves))
		}
		for _, l := range e.leaves {
			if e.foldHidden(l) {
				l.y = -1e6
				continue
			}
			e.visibleLeaves = append(e.visibleLeaves, l)
		}
		e.visibleIndexRev, e.visibleIndexFolds, e.visibleIndexReady = e.rev, e.foldRevision, true
	}
	y := e.padTop
	var prev *leaf
	var previousBlock *leaf
	previousBlockEnd := float32(0)
	prevBottom := float32(0)
	s := e.lk.size
	for _, l := range e.visibleLeaves {
		if rebuildRail && l.kind == kHeading && !l.pending {
			e.railLeaves = append(e.railLeaves, l)
		}
		// Empty source lines outside containers are common in long prose: a
		// Markdown paragraph contributes one after every block. They need only
		// one line of height and no shaping, so skip the container, margin and
		// estimate checks below for this hot case.
		if l.kind == kBlank && len(l.conts) == 0 && l.start == l.end && !e.composing {
			l.x, l.w = x, max(width, 60)
			l.h, l.view, l.y = s*e.lk.lh, nil, y
			y += l.h
			prev, prevBottom = l, 0
			continue
		}
		ind := float32(0)
		if len(l.conts) > 0 {
			ind = e.indentOf(l.conts)
			prefixOpen := e.containerPrefixOpen(l)
			if e.quotePrefixOpen(l) {
				if prefixOpen {
					ind = 0
				} else {
					ind = max(ind-e.quotePrefixShift(l), 0)
				}
			} else if prefixOpen {
				ind = 0
			}
		}
		l.x = x + ind
		l.w = max(width-ind, 60)
		if l.kind == kBlank {
			// A blank line is one line, with no margins. Its source whitespace
			// is shaped only while the caret is on it, so it stays visually blank
			// at rest but still gives the caret a real position while editing.
			l.h, l.view = s*e.lk.lh, nil
			if e.quotePrefixOpen(l) {
				l.view = e.view(l, l.w, false, parser.AlignNone)
				l.h = max(l.h, l.view.h)
			} else if (l.end > l.start && e.caret >= l.start && e.caret <= l.end) || (e.composing && e.caret == l.start) {
				// Shape source whitespace only while editing that blank line.
				// This preserves a visible caret position within spaces without
				// turning whitespace-only Markdown into a paragraph elsewhere.
				l.view = e.view(l, l.w, false, parser.AlignNone)
				l.h = max(l.h, l.view.h)
			}
			l.y = y
			y += l.h
			prev, prevBottom = l, 0
			continue
		}
		top, bottom := float32(0), float32(0)
		if l.kind == kPara && len(l.conts) == 0 && len(l.firstOf) == 0 {
			top, bottom = s*0.5, s*0.5
		} else {
			top, bottom = e.margins(l)
		}
		tight := prev != nil && tightAfter(prev, l)
		if prev == nil {
			// Blank lines before the first block are lines too — pressing
			// Enter on the first line must show the line it adds.
			start := 0
			if e.opts.HideFrontmatter {
				start = hideFrontMatterEnd(e.src)
				// The closing delimiter's own line ending is not a blank
				// line before the body. Preserve only subsequent empty lines.
				if start > 0 && start < len(e.src) && e.src[start] == '\r' {
					start++
				}
				if start > 0 && start < len(e.src) && e.src[start] == '\n' {
					start++
				}
			}
			top = float32(e.newlinesBetween(start, l.start)) * e.lk.size * e.lk.lh
		} else if tight {
			top = s * 0.2
		} else if prev.kind == kBlank {
			// Editable blank lines count toward the section gap, rather than
			// swallowing it or having another full margin added on top.
			top = 0
			if l.kind == kHeading && !l.pending && previousBlock != nil && (previousBlock.kind != kHeading || previousBlock.level == 1) {
				headingTop, _ := e.margins(l)
				top = max(0, headingTop-(y-previousBlockEnd))
			}
		} else if n := e.blankLines(prev.end, l.start); n > 0 && !e.isFoldedHeading(prev) {
			// Blank lines are lines: a paragraph break is one blank line, and
			// every Enter on an empty line leaves one more. Hidden source
			// inside a folded heading must not reappear as a vertical gap.
			// Visible blank lines take the
			// height of the line they are, so the document's lines and its
			// look agree — pressing Enter there now shows the new line.
			top = max(top, float32(n)*e.lk.size*e.lk.lh)
		}
		if l.kind == kHeading && previousBlock != nil && previousBlock.kind == kHeading && previousBlock.level != 1 {
			// A title followed by a subtitle is one group, not two sections.
			top = min(top, s*.65)
		}
		// The height this leaf is expected to take, until it is shaped:
		// the one it had where it was last laid out, or an estimate.
		est := l.h
		if l.view != nil && l.view.params.width == l.w && l.view.params.look == lk && l.view.params.assets == assets {
			est = l.view.h
		}
		if est <= 0 {
			est = e.estimateHeight(l, l.w)
		}
		at := y + max(top, prevBottom)
		// A leaf near the viewport is shaped, and so is the one the caret
		// is in, wherever that is: the caret has to be drawn on the frame
		// it moves. Everything else keeps its estimate.
		show := !virtual ||
			at <= e.viewBottom+s && at+max(est, s) >= e.viewTop ||
			e.caret >= leafLo(e, l) && e.caret <= l.end
		switch {
		case l.kind == kTable:
			// The cells are laid out below, once the table has its y.
			l.h = est
		case show:
			imageCandidate := l.view.hasImageAsset() || leafMayContainImage(e.src, l)
			outsideViewport := at > e.scroll.Y+e.viewH || at+max(est, s) < e.scroll.Y
			if virtual && imageCandidate && outsideViewport {
				// Keep text layout prefetched, but don't start an image read/decode
				// until the image is on screen. A semaphore limits active decoders,
				// yet every prefetched asset still creates a waiting goroutine and
				// pins its source until that job completes.
				l.view = nil
				l.h = est
				break
			}
			view := e.view(l, l.w, false, parser.AlignNone)
			l.h = view.h
			if view.hasImageAsset() &&
				(at > e.scroll.Y+e.viewH || at+max(view.h, s) < e.scroll.Y) {
				// Text views are cheap to keep in the shaped band, but a view can
				// own a full decoded bitmap. Prefetch its geometry and let the
				// process-wide byte-bounded image LRU own the asset until it enters
				// the actual viewport.
				l.view = nil
			} else {
				l.view = view
			}
		default:
			if l.view.hasImageAsset() {
				// Keep decoded pixels only while this image is inside the virtual
				// layout band. Revisited images can reuse the bounded asset LRU.
				l.view = nil
			}
			l.h = est
		}
		if l.kind == kHTML && l.h == 0 {
			// A block of HTML that reads as nothing, as a closing tag on a
			// line of its own does, leaves no gap where it sits.
			top, bottom = 0, 0
		}
		if tight {
			prevBottom = 0
		}
		gap := max(top, prevBottom)
		if l.kind == kPhantom && prev != nil {
			// The phantom is the line the caret opens where the source has
			// none — below a block that has no line to type on. It takes the
			// line below the block: one line, its own.
			gap = e.lk.size * e.lk.lh
		}
		y += gap
		l.y = y
		if l.kind == kTable && show {
			e.layoutTable(l)
		} else if l.view != nil {
			l.horizontal.x = min(l.horizontal.x, max(blockContentWidth(l)-l.w, 0))
		}
		y += l.h
		previousBlock, previousBlockEnd = l, y
		prevBottom = bottom
		prev = l
	}
	// In a virtual layout the height below the viewport is made of
	// estimates, so the document's height is approximate too. It only
	// gives the scroll bar its range: every leaf that comes into view is
	// shaped, and takes its exact height.
	e.docH = y + e.padBottom
	if virtual {
		e.captureVirtualLayoutIndex()
		// Height corrections above can expose more of the viewport after the
		// first pass. Converge it without another document-wide layout pass.
		e.layoutVirtualBand()
	}
	if rebuildRail {
		e.railRev = e.rev
		// The preview tells a heading's text and the first lines of its
		// section: the paragraph right after it, if one comes before the
		// next heading.
		e.railNext = e.railNext[:0]
		at := 0
		for _, h := range e.railLeaves {
			for at < len(e.leaves) && e.leaves[at] != h {
				at++
			}
			var next *leaf
			for k := at + 1; k < len(e.leaves); k++ {
				l := e.leaves[k]
				if l.isLine() || l.kind == kRule {
					continue
				}
				if l.kind == kPara {
					next = l
				}
				break
			}
			e.railNext = append(e.railNext, next)
		}
		if e.railHover >= len(e.railLeaves) {
			e.railHover = -1
		}
	}
}

// leafMayContainImage is a cheap preflight for leaves in the virtual layout
// band. It avoids constructing a display view (and therefore starting image
// loads) for likely image syntax outside the actual viewport. The scan stays
// within a candidate leaf and never walks the whole source document.
func leafMayContainImage(src string, l *leaf) bool {
	if l == nil || l.start < 0 || l.end > len(src) || l.start >= l.end {
		return false
	}
	switch l.kind {
	case kPara, kHeading, kHTML, kRaw:
	default:
		return false
	}
	text := src[l.start:l.end]
	return strings.Contains(text, "![") || strings.Contains(text, "![[") ||
		strings.Contains(text, "<img") || strings.Contains(text, "<IMG")
}

// estimateHeight returns the height to give a leaf that is not shaped.
// The blocks whose lines are known are nearly exact; text is estimated
// from the bytes it takes at the column's width. Long lines wrap, which
// the count of lines alone would miss, and an underestimate would leave
// a leaf that reaches into the viewport unshaped: its text would draw
// nothing where it starts above the viewport and ends inside it. The
// estimate is therefore on the generous side.
func (e *Editor) estimateHeight(l *leaf, width float32) float32 {
	s := e.lk.size
	switch l.kind {
	case kCode, kMath, kHTML:
		h := e.linesEstimate(l, width, s)
		if l.kind != kHTML && !l.whole {
			h += 2 * s * codePadY
		}
		return h
	case kRaw:
		return e.linesEstimate(l, width, s)
	case kTable:
		return e.tableEstimate(l, width, s)
	case kRule:
		return s * 1.2
	}
	line := s * e.lk.lh
	if l.kind == kHeading {
		line = s * 1.35
	}
	// About as many bytes as a line of mixed text takes at this width.
	perLine := max(width/s*1.8, 8)
	lines := float32(math.Ceil(float64(len(viewText(e.src, l))) / float64(perLine)))
	return max(lines, 1) * line
}

// linesEstimate returns the height of a leaf of lines as they are, in the
// code font, counting the lines their length wraps them to.
func (e *Editor) linesEstimate(l *leaf, width, s float32) float32 {
	if !l.whole {
		width = max(width-2*s*codePadX, 40)
	}
	// A monospaced character takes about 0.6 of the code font's size.
	perLine := max(width/(s*0.875*0.6), 8)
	total := 0
	for _, ln := range l.codeLines() {
		total += len(ln.text)
	}
	visual := max(len(l.codeLines()), int(math.Ceil(float64(total)/float64(perLine))))
	return float32(max(visual, 1)) * s * 0.875 * 1.6
}

// tableEstimate returns the height of a table that is not laid out: its
// rows as tall as the cell that wraps the most, its columns guessed to be
// of the same width.
func (e *Editor) tableEstimate(l *leaf, width, s float32) float32 {
	t := l.tab
	cols := max(t.cols(), 1)
	cellW := max(width/float32(cols)-2*s*cellPadX, 10)
	// Mixed text takes about half a font size a character.
	perLine := max(cellW/(s*0.5), 4)
	line := s * e.lk.lh
	total := float32(0)
	for _, row := range t.rows {
		lines := 1.0
		for _, cell := range row {
			lines = max(lines, math.Ceil(float64(max(cell.end-cell.start, 0))/float64(perLine)))
		}
		total += float32(lines)*line + 2*s*cellPadY
	}
	return max(total, line)
}

// layoutTable lays a table out: its columns as wide as their widest
// cells, narrowed to the table's room when they would not fit, and its
// rows as tall as their tallest cells.
func (e *Editor) layoutTable(l *leaf) {
	t := l.tab
	cols := t.cols()
	padX, padY := e.lk.size*cellPadX, e.lk.size*cellPadY
	g := l.geom
	if g == nil || g.boxW != l.w || len(g.colW) != cols {
		natural := make([]float32, cols)
		for r, row := range t.rows {
			for c, cell := range row {
				v := e.view(cell, 0, r == 0, alignAt(t.align, c))
				natural[c] = max(natural[c], v.natural+2*padX+2)
			}
		}
		total := float32(0)
		minW := e.lk.size * 6
		for c := range natural {
			natural[c] = max(natural[c], minW)
			total += natural[c]
		}
		widths := natural
		if total < l.w {
			// Empty and short tables use the document column, rather than
			// shrinking to a cluster of tiny cells. Keep content proportions.
			widths = make([]float32, cols)
			extra := (l.w - total) / float32(max(cols, 1))
			for c := range natural {
				widths[c] = natural[c] + extra
			}
		} else if total > l.w {
			// Narrow the columns in proportion, none below the narrowest.
			widths = make([]float32, cols)
			room := l.w
			for c := range natural {
				widths[c] = max(natural[c]*room/total, minW)
			}
		}
		g = &tableGeom{colX: make([]float32, cols+1), colW: widths, boxW: l.w}
		for c := range widths {
			g.colX[c+1] = g.colX[c] + widths[c]
		}
		l.geom = g
	}
	g.w = g.colX[cols]
	l.horizontal.x = min(l.horizontal.x, max(g.w-l.w, 0))
	if cap(g.rowY) < len(t.rows)+1 {
		g.rowY = make([]float32, len(t.rows)+1)
	} else {
		g.rowY = g.rowY[:len(t.rows)+1]
	}
	for r, row := range t.rows {
		h := e.lk.size * e.lk.lh
		for c, cell := range row {
			if c >= cols {
				break
			}
			v := e.view(cell, max(g.colW[c]-2*padX, 10), r == 0, alignAt(t.align, c))
			cell.view = v
			h = max(h, v.h)
		}
		h += 2 * padY
		g.rowY[r+1] = g.rowY[r] + h
		for c, cell := range row {
			if c >= cols {
				break
			}
			cell.x = l.x + g.colX[c] + padX - l.horizontal.x
			cell.y = l.y + g.rowY[r] + padY
			cell.w = max(g.colW[c]-2*padX, 10)
			cell.h = cell.view.h
		}
	}
	l.h = g.rowY[len(t.rows)]
}

// tableGeom is where a table's columns and rows are, relative to its box.
// boxW is the table's width when the columns were measured: a table laid
// out again at the same width keeps its columns, and only a table whose
// cells changed is measured anew.
type tableGeom struct {
	colX, colW []float32
	rowY       []float32
	w          float32
	boxW       float32
}

func alignAt(a []parser.Align, c int) parser.Align {
	if c < len(a) {
		return a[c]
	}
	return parser.AlignNone
}

// pruneViews forgets the views no frame has used for a while.
func (e *Editor) pruneViews() {
	if e.frame%120 != 0 {
		return
	}
	for k, v := range e.views {
		if e.frame-v.used > 240 {
			delete(e.views, k)
		}
	}
	if !e.virtualLayout() {
		return
	}
	// Removing an entry from the lookup cache does not release its view if a
	// leaf still points at it. Large virtual documents keep every visited leaf
	// in memory, so detach stale views once they are well outside the viewport.
	// Keep a few screens around for quick back-and-forth scrolling and always
	// retain the caret's view. The next visit can still recover a view from the
	// cache if it remains there, or reshape it otherwise.
	margin := max(e.viewH*2, e.lk.size*24)
	keepTop, keepBottom := e.scroll.Y-margin, e.scroll.Y+e.viewH+margin
	for _, l := range e.visibleLeaves {
		v := l.view
		if v == nil || e.frame-v.used <= 240 || l.y+l.h >= keepTop && l.y <= keepBottom {
			continue
		}
		if e.caret >= leafLo(e, l) && e.caret <= l.end {
			continue
		}
		l.view = nil
	}
}

// blankLines counts the empty lines between two leaves: the source between
// them, less the line break that ends the first one. A leaf right below
// another, as a list's items are, has none.
func (e *Editor) blankLines(from, to int) int {
	if n := e.newlinesBetween(from, to); n > 0 {
		return n - 1
	}
	return 0
}

// newlinesBetween counts the line breaks in the source between two leaves.
func (e *Editor) newlinesBetween(from, to int) int {
	if from >= to || to > len(e.src) {
		return 0
	}
	return strings.Count(e.src[from:to], "\n")
}
