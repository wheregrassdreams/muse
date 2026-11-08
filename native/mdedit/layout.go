package mdedit

import (
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown/parser"

	"muse/mdoc"
)

// look is how the document looks, resolved from the window's theme and the
// editor's options as a frame builds.
type look struct {
	dark bool
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
	dark       bool
	size, lh   float32
	font, mono string
	text       ui.Color
	accent     ui.Color
}

func (lk *look) key() lookKey {
	return lookKey{dark: lk.dark, size: lk.size, lh: lk.lh, font: lk.font, mono: lk.mono, text: lk.text, accent: lk.accent}
}

// setLook resolves the look from the theme and the options.
func (e *Editor) setLook(t *ui.Theme) {
	lk := &e.lk
	lk.dark = t.Dark
	lk.size = e.opts.FontSize
	if lk.size <= 0 {
		lk.size = 16
	}
	lk.lh = e.opts.LineHeight
	if lk.lh <= 0 {
		lk.lh = 1.75
	}
	lk.font = e.opts.Font
	if lk.font == "" {
		lk.font = t.Font
	}
	lk.mono = e.opts.MonoFont
	if lk.mono == "" {
		lk.mono = "monospace"
	}
	lk.text = t.Text
	lk.muted = t.TextMuted
	lk.accent = t.Accent
	lk.marker = t.TextMuted.Alpha(0.7)
	lk.border = t.Border
	lk.selection = t.Selection
	lk.caret = t.Accent
	if t.Dark {
		lk.codeBg = ui.RGBA(255, 255, 255, 0.08)
		lk.panel = ui.RGBA(255, 255, 255, 0.045)
		lk.codeText = ui.Hex("#e4a3a3")
		lk.quoteBar = ui.RGBA(255, 255, 255, 0.16)
		lk.markBg = ui.RGBA(250, 204, 21, 0.32)
	} else {
		lk.codeBg = ui.RGBA(0, 0, 0, 0.055)
		lk.panel = ui.Hex("#f6f7f9")
		lk.codeText = ui.Hex("#c7254e")
		lk.quoteBar = ui.RGBA(0, 0, 0, 0.12)
		lk.markBg = ui.RGBA(250, 204, 21, 0.45)
	}
	lk.code = editor.DefaultTheme(t)
}

// span returns how text in style st looks, in a leaf of text of size.
func (lk *look) span(st istyle, size float32) ui.Span {
	s := ui.Span{Color: lk.text}
	if st.muted {
		s.Color = lk.muted
	}
	if st.bold {
		s.Weight = 700
	}
	if st.italic {
		s.Italic = true
	}
	if st.strike {
		s.Strikethrough = true
	}
	if st.under {
		s.Underline = true
	}
	if st.link {
		s.Color = lk.accent
	}
	if st.tag {
		// A tag reads as a tag, not as a link: the accent on a soft pill.
		s.Color = lk.accent
		s.Background = lk.accent.Alpha(0.12)
		s.Radius = max(3, size/5)
	}
	if st.mark {
		s.Background = lk.markBg
	}
	if st.code {
		s.Font = lk.mono
		s.Size = size * 0.875
		s.Background = lk.codeBg
		s.Radius = max(3, size/5)
		if !st.link {
			s.Color = lk.codeText
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
		s.Color = lk.marker
		s.Background = ui.Color{}
		s.Underline, s.Strikethrough = false, false
		s.Radius = 0
	}
	return s
}

// headingSize returns the size of the text of a heading of level.
func (lk *look) headingSize(level int) float32 { return mdoc.HeadingSize(level, lk.size) }

func headingWeight(level int) int {
	if level <= 2 {
		return 700
	}
	return 650
}

// Spacing, in ems of the body text.
const (
	quoteIndent = 1.2
	itemIndent  = 1.75
	codePadX    = 1.0
	codePadY    = 0.75
	cellPadX    = 0.7
	cellPadY    = 0.45
)

// leafView is a leaf as the editor shows it: its display, laid out, and
// the boxes around it, relative to the leaf's box.
type leafView struct {
	disp   display
	layout *ui.TextLayout
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
	diagram *mdoc.Diagram
	diagX   float32
	used    uint64
}

// viewKey is what a leaf's view is made of: views of the same key are the
// same, and the editor keeps them across frames.
type viewKey struct {
	kind      leafKind
	level     int
	flags     uint8
	width     float32
	reveal    int
	compose   string
	composeAt int
	text      string
	look      lookKey
	assets    uint64
	align     parser.Align
}

// viewParams is what a view depends on besides the leaf's text. A leaf
// keeps its view while these stay equal and its content does not change
// (see sameLeaf), which is what makes laying out an edited document cost
// nothing for the leaves the edit did not touch.
type viewParams struct {
	kind      leafKind
	level     int
	flags     uint8
	width     float32
	reveal    int
	compose   string
	composeAt int
	look      lookKey
	assets    uint64
	align     parser.Align
}

func (k *viewKey) params() viewParams {
	return viewParams{
		kind: k.kind, level: k.level, flags: k.flags, width: k.width,
		reveal: k.reveal, compose: k.compose, composeAt: k.composeAt,
		look: k.look, assets: k.assets, align: k.align,
	}
}

const (
	fPending uint8 = 1 << iota
	fSetext
	fHeader
	fFront
	fQuote
	fNatural
)

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
	return e.viewAt(l, width, header, align, e.revealAt(l), true)
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
	k := viewKey{
		kind:   l.kind,
		level:  l.level,
		flags:  leafFlags(l, header),
		width:  width,
		reveal: reveal,
		look:   e.lk.key(),
		assets: e.assetGen.Load(),
		align:  align,
	}
	at := -1
	if reveal >= 0 {
		at = reveal + l.lineStart
	}
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
	if settle && l.view != nil && l.vp == p {
		l.view.used = e.frame
		return l.view
	}
	k.text = viewText(e.src, l)
	if v, ok := e.views[k]; ok {
		v.used = e.frame
		if settle {
			l.view, l.vp = v, p
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
		v = e.linesView(l, width, at, withCompose, l.frontMatter)
	case kRule:
		v = e.ruleView(l, width, at, withCompose)
	default:
		v = e.textView(l, width, at, withCompose, header, align)
	}
	v.used = e.frame
	if settle {
		l.view, l.vp = v, p
	}
	e.views[k] = v
	return v
}

// newBuilder returns a builder of the display of leaf l, with the caret
// at at (-1 for elsewhere), and the composition there when withCompose.
func (e *Editor) newBuilder(l *leaf, at int, withCompose bool, size, width float32) *builder {
	b := &builder{e: e, src: e.src, base: l.lineStart, reveal: at, fontSize: size, width: width}
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
		f.LineHeight = 1.35
	}
	return f
}

// textView lays out a paragraph, a heading, a table cell or the phantom.
func (e *Editor) textView(l *leaf, width float32, at int, withCompose, header bool, align parser.Align) *leafView {
	f := e.textFont(l)
	b := e.newBuilder(l, at, withCompose, f.Size, width)
	st := istyle{}
	if header {
		st.bold = true
	}
	for _, c := range l.conts {
		if c.kind == cQuote {
			st.muted = true
		}
	}
	if task, ok := l.innerItem(); ok && task.task == parser.Checked && l.opensItem() {
		st.muted, st.strike = true, true
	}
	switch {
	case l.pending || l.isLine() || l.id < 0:
		b.textRun(l.start, l.end, st)
		b.finish(l.end)
	default:
		ins := e.p.doc.Inlines(l.id)
		b.alone = aloneImage(ins, e.src)
		b.starts, b.cell = e.lineStarts(l.id), l.table != nil
		// A heading's `#`s are chrome, hidden like every other block marker —
		// but the caret on the line shows them, as Obsidian's live preview
		// does, so the level can be edited in place.
		if l.kind == kHeading && !l.pending && at >= l.lineStart && at <= l.end {
			b.marker(l.lineStart, l.start, st)
		}
		b.inlines(ins, l.start, l.end, st)
	}
	b.ensure(l.start)
	v := &leafView{disp: b.display()}
	shape := ui.TextShape{Width: width}
	switch align {
	case parser.AlignCenter:
		shape.Align = ui.Center
	case parser.AlignRight:
		shape.Align = ui.End
	}
	if b.alone && !b.revealedAny {
		shape.Align = ui.Center
	}
	v.layout, v.empty = shapeDisplay(&v.disp, f, shape)
	v.font = f
	_, v.h = v.layout.Size()
	if width == 0 {
		v.natural, _ = v.layout.Size()
	}
	return v
}

// shapeDisplay lays out a display, or a space for an empty one.
func shapeDisplay(d *display, f ui.Font, shape ui.TextShape) (*ui.TextLayout, bool) {
	if d.text == "" {
		return ui.ShapeRichTextWith([]ui.Span{{Text: " "}}, f, shape), true
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
	return ui.ShapeRichTextWith(spans, f, shape), false
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
	return l.kind == kCode && l.fenced && strings.EqualFold(l.lang, "mermaid")
}

// codeView lays out a block of code: its lines, highlighted, in a panel,
// with its fence's info string above them while the caret is there.
func (e *Editor) codeView(l *leaf, width float32, at int, withCompose bool) *leafView {
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
	v := &leafView{panel: !l.whole, tx: padX, ty: padY, label: l.lang}
	if l.whole {
		v.label = ""
	}
	plain := ui.Span{Color: e.lk.text}
	infoLine := l.fenced && caretIn && b.reveal >= l.infoStart && b.reveal <= l.infoEnd
	if infoLine {
		mark := ui.Span{Color: e.lk.marker}
		fence := strings.TrimLeft(e.src[l.lineStart:l.fenceEnd], " \t>")
		b.deco(l.infoStart, fence, mark, nil)
		b.spanRun(l.infoStart, l.infoEnd, ui.Span{Color: e.lk.muted})
		if len(l.lines) > 0 {
			b.spanAtom(l.infoEnd, l.lines[0].off, "\n", plain)
		}
		v.reveal = true
	}
	lang := editor.LanguageNamed(l.lang)
	var state uint64
	var toks []editor.Token
	for k, ln := range l.lines {
		if k > 0 {
			prev := l.lines[k-1]
			b.spanAtom(prev.off+len(prev.text), ln.off, "\n", plain)
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
	b.finish(l.end)
	b.ensure(l.start)
	v.disp = b.display()
	v.layout, v.empty = shapeDisplay(&v.disp, f, ui.TextShape{Width: inner})
	v.font = f
	_, th := v.layout.Size()
	v.h = th + 2*padY
	if !caretIn && isMermaid(l) {
		if d, ok := mdoc.ParseDiagram(e.codeText(l)); ok {
			v.diagram = &d
			v.h = d.Height(width) + 2*padY
		}
	}
	return v
}

// codeText returns the text of a code or math leaf's lines.
func (e *Editor) codeText(l *leaf) string {
	var sb strings.Builder
	for k, ln := range l.lines {
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
	tex := e.codeText(l)
	obj := e.mathBlockObject(tex, width)
	if !caretIn {
		v := &leafView{block: obj}
		if obj == nil {
			// Nothing to typeset, or TeX the typesetter cannot read: its
			// source shows, in the panel of code.
			return e.linesView(l, width, -1, false, true)
		}
		pad := e.lk.size * 0.5
		v.bx, v.by = (width-obj.w)/2, pad
		v.h = obj.h + 2*pad
		return v
	}
	v := e.linesView(l, width, at, withCompose, true)
	v.reveal = true
	if obj != nil {
		gap := e.lk.size * 0.6
		v.block = obj
		v.bx, v.by = (width-obj.w)/2, v.h+gap
		v.h += obj.h + 2*gap
	}
	return v
}

// htmlView lays out a block of HTML: its source, or the image of a block
// of just an image while the caret is elsewhere.
func (e *Editor) htmlView(l *leaf, width float32, at int, withCompose bool) *leafView {
	html := e.codeText(l)
	if obj := e.htmlImageObject(html, width); obj != nil {
		if at < 0 {
			return &leafView{block: obj, bx: (width - obj.w) / 2, h: obj.h}
		}
		v := e.linesView(l, width, at, withCompose, false)
		gap := e.lk.size * 0.5
		v.block, v.bx, v.by = obj, (width-obj.w)/2, v.h+gap
		v.h += obj.h + gap
		return v
	}
	if at < 0 {
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
	for k, ln := range l.lines {
		if k > 0 {
			prev := l.lines[k-1]
			b.spanAtom(prev.off+len(prev.text), ln.off, "\n", sp)
		}
		b.codeRun(ln.off, ln.off+len(ln.text), sp)
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
	if at < 0 {
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
			x += e.lk.size * quoteIndent
		default:
			x += e.lk.size * itemIndent
		}
	}
	return x
}

// margins returns the room above and below a leaf.
func (e *Editor) margins(l *leaf) (top, bottom float32) {
	s := e.lk.size
	switch l.kind {
	case kHeading:
		if l.pending {
			return s * 0.7, s * 0.7
		}
		tops := [...]float32{1.4, 1.2, 1.0, 0.9, 0.8, 0.8}
		return s * tops[min(max(l.level, 1), 6)-1], s * 0.45
	case kCode, kMath, kTable, kHTML:
		return s * 0.8, s * 0.8
	case kRule:
		return s * 0.6, s * 0.6
	case kRaw:
		return s * 0.6, s * 0.6
	case kPhantom, kBlank:
		// A line of its own: the line is the space, and it has no margins
		// of its own to add to the lines around it.
		return 0, 0
	}
	return s * 0.7, s * 0.7
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
	y := e.padTop
	var prev *leaf
	prevBottom := float32(0)
	s := e.lk.size
	for _, l := range e.leaves {
		if rebuildRail && l.kind == kHeading && !l.pending {
			e.railLeaves = append(e.railLeaves, l)
		}
		ind := e.indentOf(l.conts)
		l.x = x + ind
		l.w = max(width-ind, 60)
		if l.kind == kBlank {
			// A blank line is one line, whatever its own paragraph would
			// shape to: no margins, nothing to shape, no text to draw — but
			// for the text the input method composes on it, which is drawn
			// in the line's own view while it is being composed.
			l.h, l.view = s*e.lk.lh, nil
			if e.composing && e.caret == l.start {
				l.view = e.view(l, l.w, false, parser.AlignNone)
			}
			l.y = y
			y += l.h
			prev, prevBottom = l, 0
			continue
		}
		top, bottom := e.margins(l)
		tight := prev != nil && tightAfter(prev, l)
		if prev == nil {
			// Blank lines before the first block are lines too — pressing
			// Enter on the first line must show the line it adds.
			top = float32(e.newlinesBetween(0, l.start)) * e.lk.size * e.lk.lh
		} else if tight {
			top = s * 0.2
		} else if prev.kind == kBlank {
			// The blank line above is the gap: a block right below it starts
			// on the next line, not on its own margin of its own.
			top = 0
		} else if n := e.blankLines(prev.end, l.start); n > 0 {
			// Blank lines are lines: a paragraph break is one blank line, and
			// every Enter on an empty line leaves one more. They take the
			// height of the line they are, so the document's lines and its
			// look agree — pressing Enter there now shows the new line.
			top = max(top, float32(n)*e.lk.size*e.lk.lh)
		}
		// The height this leaf is expected to take, until it is shaped:
		// the one it had where it was last laid out, or an estimate.
		est := l.h
		if l.view != nil && l.vp.width == l.w && l.vp.look == lk && l.vp.assets == assets {
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
			e.caret >= leafLo(l) && e.caret <= l.end
		switch {
		case l.kind == kTable:
			// The cells are laid out below, once the table has its y.
			l.h = est
		case show:
			l.view = e.view(l, l.w, false, parser.AlignNone)
			l.h = l.view.h
		default:
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
		}
		y += l.h
		prevBottom = bottom
		prev = l
	}
	// In a virtual layout the height below the viewport is made of
	// estimates, so the document's height is approximate too. It only
	// gives the scroll bar its range: every leaf that comes into view is
	// shaped, and takes its exact height.
	e.docH = y + e.padBottom
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
	for _, ln := range l.lines {
		total += len(ln.text)
	}
	visual := max(len(l.lines), int(math.Ceil(float64(total)/float64(perLine))))
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
		minW := e.lk.size * 3
		for c := range natural {
			natural[c] = max(natural[c], minW)
			total += natural[c]
		}
		widths := natural
		if total > l.w {
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
			cell.x = l.x + g.colX[c] + padX
			cell.y = l.y + g.rowY[r] + padY
			cell.w = max(g.colW[c]-2*padX, 10)
			cell.h = cell.view.h
		}
	}
	g.w = g.colX[cols]
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
