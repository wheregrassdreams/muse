package mdedit

import (
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
)

// A leaf shows as display text: the characters drawn, each piece of which
// stands for a range of the source. Text pieces are the source itself,
// character for character; atoms stand for their range as a whole, as a
// line break for its newline and the prefixes after it, or a formula for
// its TeX; decorations stand for nothing, as a fence's backticks drawn
// beside an info string. Syntax that is hidden is simply not a piece: the
// caret cannot go there, and the display skips it.

type pkind uint8

const (
	pText pkind = iota
	pAtom
	pDeco
)

// piece is a run of display text and the source range it stands for.
type piece struct {
	kind pkind
	// s0 and s1 are the source range, relative to the leaf's start; d0 and
	// d1 the display range, in UTF-16 units.
	s0, s1 int
	d0, d1 int
	text   string
	span   ui.Span
	// obj is the inline object an atom draws: an image or a formula.
	obj *object
	// marker tells syntax shown because the caret is at it.
	marker bool
}

// object is an inline object of a layout, drawn in the box the layout
// gives its object replacement character.
type object struct {
	bmp *ui.Bitmap
	// alt is drawn in a placeholder while bmp is nil.
	alt string
	// w, h and descent are the box's size, in DIPs.
	w, h, descent float32
	// block makes the object a line of its own, centered: a paragraph of
	// just an image.
	block bool
}

// istyle is the inline style a run of text is in.
type istyle struct {
	bold, italic, strike, code, mono, link, tag, under, mark, sup, sub, kbd bool
	marker                                                                  bool
	muted                                                                   bool
	// color and bg are the text's and its background's, as HTML sets
	// them; zero for the look's.
	color, bg ui.Color
}

// builder makes the pieces of a leaf's display.
type builder struct {
	e   *Editor
	src string
	// base is the offset pieces are relative to.
	base int
	// reveal is the caret's offset, which shows the syntax of what it is
	// in; -1 for none.
	reveal int
	pos    int
	pieces []piece
	text   strings.Builder
	u16    int
	// The composition of an input method, inserted at composeAt.
	compose   string
	composeAt int
	composed  bool
	lastSpan  ui.Span
	// hasObjects tells a display with images or formulas.
	hasObjects bool
	// htmlTags are the formatting tags open, as <u>, and the style each
	// had before it.
	htmlTags []htmlTag
	// width is the room of the leaf, which images shrink to.
	width float32
	// alone tells a paragraph of just an image, drawn as a block; and
	// revealedAny that its source shows.
	alone, revealedAny bool
	// fontSize is the size of the leaf's text.
	fontSize float32
	// starts are the offsets of the lines of the leaf's block, where a
	// line break inside an inline ends; cell tells a table's cell.
	starts []int
	cell   bool
}

type htmlTag struct {
	name string
	prev istyle
}

func (b *builder) revealed(s, e int) bool { return b.reveal >= s && b.reveal <= e }

// add appends a piece, inserting the composition where it falls.
func (b *builder) add(p piece) {
	if b.compose != "" && !b.composed && p.kind == pText && b.composeAt > p.s0+b.base && b.composeAt < p.s1+b.base {
		cut := b.composeAt - b.base - p.s0
		left, right := p, p
		left.text, left.s1 = p.text[:cut], p.s0+cut
		right.text, right.s0 = p.text[cut:], p.s0+cut
		b.add(left)
		b.addCompose(p.span)
		b.add(right)
		return
	}
	if b.compose != "" && !b.composed && b.composeAt <= p.s0+b.base && (p.kind != pDeco || b.composeAt < p.s0+b.base) {
		b.addCompose(b.lastSpan)
	}
	p.d0 = b.u16
	b.u16 += ui.UTF16Len(p.text)
	p.d1 = b.u16
	b.text.WriteString(p.text)
	b.pieces = append(b.pieces, p)
	if p.kind == pText {
		b.lastSpan = p.span
	}
}

// addCompose inserts the composition, underlined in the style around it.
func (b *builder) addCompose(sp ui.Span) {
	b.composed = true
	sp.Text = ""
	sp.Underline = true
	sp.ObjectWidth, sp.ObjectHeight, sp.ObjectDescent = 0, 0, 0
	at := b.composeAt - b.base
	p := piece{kind: pDeco, s0: at, s1: at, text: b.compose, span: sp}
	p.d0 = b.u16
	b.u16 += ui.UTF16Len(p.text)
	p.d1 = b.u16
	b.text.WriteString(p.text)
	b.pieces = append(b.pieces, p)
}

// finish inserts the composition at the end, when nothing came after it.
func (b *builder) finish(end int) {
	if b.compose != "" && !b.composed && b.composeAt >= b.base && b.composeAt <= end {
		b.addCompose(b.lastSpan)
	}
}

// span returns how text in style st looks in the leaf.
func (b *builder) span(st istyle) ui.Span { return b.e.lk.span(st, b.fontSize) }

// spanRun adds source text from s to e as it is, in sp.
func (b *builder) spanRun(s, e int, sp ui.Span) {
	if e <= s {
		return
	}
	b.add(piece{kind: pText, s0: s - b.base, s1: e - b.base, text: b.src[s:e], span: sp})
}

// codeRun adds code from s to e in sp, a tab drawn as four spaces.
func (b *builder) codeRun(s, e int, sp ui.Span) {
	for s < e {
		t := strings.IndexByte(b.src[s:e], '\t')
		if t < 0 {
			b.spanRun(s, e, sp)
			return
		}
		b.spanRun(s, s+t, sp)
		b.spanAtom(s+t, s+t+1, "    ", sp)
		s += t + 1
	}
}

// spanAtom adds display text standing for the source from s to e, in sp.
func (b *builder) spanAtom(s, e int, text string, sp ui.Span) {
	b.add(piece{kind: pAtom, s0: s - b.base, s1: e - b.base, text: text, span: sp})
}

// textRun adds source text from s to e as it is.
func (b *builder) textRun(s, e int, st istyle) { b.lineRun(s, e, b.span(st), false) }

// marker adds syntax shown because the caret is at it.
func (b *builder) marker(s, e int, st istyle) {
	st.marker = true
	b.lineRun(s, e, b.span(st), true)
}

// lineRun adds source text from s to e in sp. A line ending in it, as in
// code or a link's destination written over two lines, shows as a line
// break standing for the prefix of the next line too, which in a
// quotation or a list is not text.
func (b *builder) lineRun(s, e int, sp ui.Span, marker bool) {
	for s < e {
		nl := strings.IndexByte(b.src[s:e], '\n')
		if nl < 0 {
			b.add(piece{kind: pText, s0: s - b.base, s1: e - b.base, text: b.src[s:e], span: sp, marker: marker})
			return
		}
		nl += s
		t := nl
		if t > s && b.src[t-1] == '\r' {
			t--
		}
		if t > s {
			b.add(piece{kind: pText, s0: s - b.base, s1: t - b.base, text: b.src[s:t], span: sp, marker: marker})
		}
		next := b.lineContent(nl, e)
		b.add(piece{kind: pAtom, s0: t - b.base, s1: next - b.base, text: "\n", span: sp})
		s = next
	}
}

// lineContent returns where the text of the line after the line ending at
// nl starts, no further than limit: after the prefixes of the containers
// the block is in and the spaces before the text.
func (b *builder) lineContent(nl, limit int) int {
	at := nl + 1
	for _, s := range b.starts {
		if s > nl {
			at = s
			break
		}
	}
	for at < limit && (b.src[at] == ' ' || b.src[at] == '\t') {
		at++
	}
	return min(at, limit)
}

// objectRune is the character a layout gives its box to: the object
// replacement character, one per object (see ui.Span.ObjectWidth).
const objectRune = "\uFFFC"

// box gives sp the box of an object, and reports whether the layout will
// draw it. An object with no box draws nothing, and must not leave the
// replacement character in the text: with no box the shaper draws that
// character as a glyph, and a font that has none draws a box of its own —
// the empty square that stood where a picture with no size was.
func (b *builder) box(sp *ui.Span, obj *object) bool {
	if obj == nil || obj.w <= 0 || obj.h <= 0 {
		return false
	}
	b.hasObjects = true
	sp.ObjectWidth, sp.ObjectHeight, sp.ObjectDescent = obj.w, obj.h, obj.descent
	return true
}

// atom adds display text standing for the source from s to e as a whole.
func (b *builder) atom(s, e int, text string, st istyle, obj *object) {
	sp := b.span(st)
	if b.box(&sp, obj) {
		text = objectRune
	} else {
		obj = nil
	}
	b.add(piece{kind: pAtom, s0: s - b.base, s1: e - b.base, text: text, span: sp, obj: obj})
}

// deco adds display text standing for no source, at s.
func (b *builder) deco(s int, text string, sp ui.Span, obj *object) {
	if b.box(&sp, obj) {
		text = objectRune
	} else {
		obj = nil
	}
	b.add(piece{kind: pDeco, s0: s - b.base, s1: s - b.base, text: text, span: sp, obj: obj})
}

// gap shows the spaces between the last piece and to, which no inline
// holds: the parser trims them, the editor shows them, so that a space
// typed at the end of a line shows with the caret after it.
func (b *builder) gap(to int, st istyle) {
	if b.pos >= to {
		return
	}
	s := b.pos
	b.pos = to
	g := b.src[s:to]
	if strings.Trim(g, " \t") != "" {
		return
	}
	b.textRun(s, to, st)
}

// inlines adds the display of a block's inlines, from the source offset
// from to to.
func (b *builder) inlines(ins []parser.Inline, from, to int, st istyle) {
	b.pos = from
	b.walk(ins, 0, len(ins), st)
	b.gap(to, st)
	b.finish(to)
}

func (b *builder) walk(ins []parser.Inline, i, end int, st istyle) {
	marks := highlights(b.src, ins, i, end)
	cmts := b.e.comments
	// outer is whether the text was marked before the highlight it is in.
	outer := false
	for i < end {
		in := &ins[i]
		b.gap(in.Start, st)
		next := max(in.Next, i+1)
		// The comment the caret is in is parsed whole (see parseAll): its
		// source draws as syntax, dimmed, the way a revealed construct does —
		// so `**bold**` inside it stays the four bytes it is.
		if c, ok := commentAt(cmts, in.Start); ok && b.revealed(c.from, c.to) {
			b.marker(in.Start, in.End, st)
			b.pos = max(b.pos, in.End)
			i = next
			continue
		}
		switch in.Kind {
		case parser.Text:
			st = b.text_(in.Start, in.End, st, marks, cmts, wikiLinks(b.src, in.Start, in.End), &outer)
		case parser.SoftBreak, parser.LineBreak:
			// A comment's line breaks are the comment's: hidden with it, so a
			// `%%` block collapses instead of leaving blank lines behind. The
			// break right after a block comment goes too — but not the one
			// after an inline comment mid-line, which separates two lines.
			if c, ok := commentAt(cmts, in.Start); ok && !b.revealed(c.from, c.to) {
				b.pos = in.End
				break
			}
			if in.Start > 0 {
				if c, ok := commentAt(cmts, in.Start-1); ok && c.to == in.Start &&
					!b.revealed(c.from, c.to) && (c.from == 0 || b.src[c.from-1] == '\n') {
					b.pos = in.End
					break
				}
			}
			// The break stands for the spaces before the next line's text
			// too, which the parser skips.
			end := in.End
			for end < len(b.src) && (b.src[end] == ' ' || b.src[end] == '\t') {
				end++
			}
			b.atom(in.Start, end, "\n", st, nil)
			b.pos = end
		case parser.Code:
			b.codeSpan(in, st)
		case parser.Emphasis, parser.Strong, parser.Strikethrough:
			b.wrapped(ins, i, st)
		case parser.Link:
			b.link(ins, i, st)
		case parser.Image:
			b.image(ins, i, st)
		case parser.HTML:
			st = b.htmlInline(in, st)
		case parser.Math:
			b.math(in, st)
		case parser.FootnoteRef:
			b.footnoteRef(in, st)
		default:
			b.textRun(in.Start, in.End, st)
		}
		b.pos = max(b.pos, in.End)
		i = next
	}
}

// children returns where the children of inline i start and end in the
// source: the first one's start and the last one's end, or from and from
// when it has none.
func children(ins []parser.Inline, i int, from int) (int, int) {
	in := &ins[i]
	if i+1 >= in.Next || i+1 >= len(ins) {
		return from, from
	}
	s := ins[i+1].Start
	e := s
	for j := i + 1; j < in.Next && j < len(ins); j = max(ins[j].Next, j+1) {
		e = max(e, ins[j].End)
	}
	return s, e
}

// text_ adds a Text inline's source, with its escapes and character
// references as the characters they stand for, unless the caret is at
// them, and its ==s of highlight among marks hidden, unless the caret is
// in what they mark. Its %%comments%% draw nothing, and its [[wiki links]]
// draw their label, with the delimiters hidden unless the caret is at
// them. It returns the style after it, marked after an == that opens;
// outer keeps whether it was marked before.
func (b *builder) text_(s, e int, st istyle, marks []hlMark, cmts []cmt, wikis []wikiLink, outer *bool) istyle {
	src := b.src
	run := s
	m := 0
	for m < len(marks) && marks[m].at < s {
		m++
	}
	w := 0
	for w < len(wikis) && wikis[w].to <= s {
		w++
	}
	for p := s; p < e; {
		if c, ok := commentAt(cmts, p); ok {
			// Only this inline's slice of the comment: a block comment spans
			// several Text inlines, and each draws its own part.
			from, to := max(c.from, s), min(c.to, e)
			b.textRun(run, p, st)
			if b.revealed(c.from, c.to) {
				b.marker(from, to, st)
			}
			p = to
			run = p
			continue
		}
		if w < len(wikis) && wikis[w].from <= p {
			wk := wikis[w]
			if wk.from < p {
				w++
				continue
			}
			b.textRun(run, p, st)
			b.wiki(wk, st)
			p = wk.to
			run = p
			w++
			continue
		}
		if m < len(marks) && marks[m].at == p {
			b.textRun(run, p, st)
			if mk := marks[m]; b.revealed(mk.from, mk.to) {
				b.marker(p, p+2, st)
			}
			if marks[m].open {
				*outer = st.mark
				st.mark = true
			} else {
				st.mark = *outer
			}
			p += 2
			run = p
			m++
			continue
		}
		c := src[p]
		if c == '#' {
			if te := tagAt(src, p, s, e); te > 0 {
				b.textRun(run, p, st)
				ts := st
				ts.tag = true
				b.textRun(p, te, ts)
				p = te
				run = p
				continue
			}
		}
		if c == '\\' && p+1 < e && isASCIIPunct(src[p+1]) {
			b.textRun(run, p, st)
			if b.revealed(p, p+2) {
				b.marker(p, p+1, st)
				b.textRun(p+1, p+2, st)
			} else {
				b.atom(p, p+2, src[p+1:p+2], st, nil)
			}
			p += 2
			run = p
			continue
		}
		if c == '&' {
			if n, txt := parser.Entity(src[p:e]); n > 0 && txt != "" {
				b.textRun(run, p, st)
				if b.revealed(p, p+n) {
					b.marker(p, p+n, st)
				} else {
					b.atom(p, p+n, txt, st, nil)
				}
				p += n
				run = p
				continue
			}
		}
		p++
	}
	b.textRun(run, e, st)
	return st
}

// wiki draws a wiki link's label in the link color. Its brackets (and the
// `|` before a label) show only while the caret is in the link, like the
// markers of every other construct, so the source can be edited in place.
func (b *builder) wiki(w wikiLink, st istyle) {
	inner := st
	inner.link = true
	// An embed of a picture is the picture, as Obsidian draws it. A note
	// embed transcludes later; for now it reads as its label.
	if w.embed && !b.revealed(w.from, w.to) && isImageName(w.target) {
		if obj := b.e.imageObject(w.target, "", b.width, b.alone); obj != nil {
			b.atom(w.from, w.to, "", st, obj)
			return
		}
	}
	if b.revealed(w.from, w.to) {
		b.marker(w.from, w.labelFrom, st)
		b.textRun(w.labelFrom, w.labelTo, inner)
		b.marker(w.labelTo, w.to, st)
		return
	}
	b.textRun(w.labelFrom, w.labelTo, inner)
}

// wrapped adds emphasis, strong emphasis or strikethrough: its markers
// while the caret is at it, and its text in its style.
func (b *builder) wrapped(ins []parser.Inline, i int, st istyle) {
	in := &ins[i]
	cs, ce := children(ins, i, in.Start)
	if cs == in.Start {
		// Nothing inside: show it as written.
		b.textRun(in.Start, in.End, st)
		return
	}
	rev := b.revealed(in.Start, in.End)
	inner := st
	switch in.Kind {
	case parser.Emphasis:
		inner.italic = true
	case parser.Strong:
		inner.bold = true
	case parser.Strikethrough:
		inner.strike = true
	}
	if rev {
		b.marker(in.Start, cs, st)
	}
	b.pos = cs
	b.walk(ins, i+1, in.Next, inner)
	b.gap(ce, inner)
	if rev {
		b.marker(ce, in.End, st)
	}
	b.pos = in.End
}

// link adds a link: its text, and its brackets and destination while the
// caret is in it.
func (b *builder) link(ins []parser.Inline, i int, st istyle) {
	in := &ins[i]
	cs, ce := children(ins, i, in.Start)
	inner := st
	inner.link = true
	if i+1 >= in.Next || i+1 >= len(ins) {
		// A link without text shows its source, as there is nothing else
		// to click or to see.
		b.marker(in.Start, in.End, st)
		return
	}
	rev := b.revealed(in.Start, in.End)
	if rev {
		b.marker(in.Start, cs, st)
	}
	b.pos = cs
	if in.Auto {
		// An address, between angle brackets or not, shows as written:
		// it has no escapes.
		b.textRun(cs, ce, inner)
	} else {
		b.walk(ins, i+1, in.Next, inner)
		b.gap(ce, inner)
	}
	if rev {
		b.marker(ce, in.End, st)
	}
	b.pos = in.End
}

// codeSpan adds code within text: its backticks while the caret is at it.
func (b *builder) codeSpan(in *parser.Inline, st istyle) {
	n := 0
	for in.Start+n < in.End && b.src[in.Start+n] == '`' {
		n++
	}
	cs, ce := in.Start+n, in.End-n
	if ce < cs {
		ce = cs
	}
	inner := st
	inner.code = true
	if b.revealed(in.Start, in.End) {
		b.marker(in.Start, cs, st)
		b.textRun(cs, ce, inner)
		b.marker(ce, in.End, st)
		return
	}
	if ce == cs {
		b.marker(in.Start, in.End, st)
		return
	}
	for b.cell {
		// In a table's cell, \| is the pipe it stands for, in code too.
		k := strings.Index(b.src[cs:ce], `\|`)
		if k < 0 {
			break
		}
		b.textRun(cs, cs+k, inner)
		b.atom(cs+k, cs+k+2, "|", inner, nil)
		cs += k + 2
	}
	b.textRun(cs, ce, inner)
}

// math adds a formula within text: typeset, or its TeX while the caret is
// at it.
func (b *builder) math(in *parser.Inline, st istyle) {
	n := 1
	if strings.HasPrefix(b.src[in.Start:in.End], "$$") || strings.HasPrefix(b.src[in.Start:in.End], `\(`) || strings.HasPrefix(b.src[in.Start:in.End], `\[`) {
		n = 2
	}
	cs, ce := min(in.Start+n, in.End), max(in.End-n, in.Start+n)
	if b.revealed(in.Start, in.End) {
		inner := st
		inner.code = true
		b.marker(in.Start, cs, st)
		b.textRun(cs, min(ce, in.End), inner)
		b.marker(min(ce, in.End), in.End, st)
		return
	}
	if obj := b.e.mathObject(in.Text, b.fontSize, in.Display); obj != nil {
		b.atom(in.Start, in.End, "", st, obj)
		return
	}
	inner := st
	inner.italic = true
	b.atom(in.Start, in.End, texFallback(in.Text), inner, nil)
}

// image adds an image: drawn, and its source too while the caret is at it.
func (b *builder) image(ins []parser.Inline, i int, st istyle) {
	in := &ins[i]
	obj := b.e.imageObject(in.Dest, b.e.plainText(ins[i+1:max(in.Next, i+1)]), b.width, b.alone)
	if !b.revealed(in.Start, in.End) {
		b.atom(in.Start, in.End, "", st, obj)
		return
	}
	b.revealedAny = true
	cs, ce := children(ins, i, in.Start+2)
	alt := st
	alt.muted = true
	b.marker(in.Start, cs, st)
	b.pos = cs
	b.walk(ins, i+1, in.Next, alt)
	b.gap(ce, alt)
	b.marker(ce, in.End, st)
	b.pos = in.End
	if obj != nil && obj.block {
		b.deco(in.End, "\n", b.span(st), nil)
	}
	b.deco(in.End, "", b.span(st), obj)
}

// footnoteRef adds a reference to a footnote: its label raised, or its
// source while the caret is at it.
func (b *builder) footnoteRef(in *parser.Inline, st istyle) {
	if b.revealed(in.Start, in.End) {
		b.marker(in.Start, in.End, st)
		return
	}
	sp := b.span(st)
	sp.Color = b.e.lk.accent
	sp.Size = b.fontSize * 0.75
	b.add(piece{kind: pAtom, s0: in.Start - b.base, s1: in.End - b.base, text: "[" + in.Text + "]", span: sp})
}

// htmlInline adds a tag within text. The tags of formatting, as <u> and
// <b>, are hidden, unless the caret is at them, and style the text up to
// their closing tags; others show as written.
func (b *builder) htmlInline(in *parser.Inline, st istyle) istyle {
	if src, alt, sz, ok := imgTag(in.Text); ok {
		b.htmlImage(in, src, alt, sz, st)
		return st
	}
	name, closing, ok := htmlTagName(in.Text)
	if !ok {
		b.marker(in.Start, in.End, st)
		return st
	}
	rev := b.revealed(in.Start, in.End)
	if name == "br" {
		if rev {
			b.marker(in.Start, in.End, st)
		} else {
			b.atom(in.Start, in.End, "\n", st, nil)
		}
		return st
	}
	if rev {
		b.marker(in.Start, in.End, st)
	} else {
		b.pos = in.End
	}
	if closing {
		for k := len(b.htmlTags) - 1; k >= 0; k-- {
			if b.htmlTags[k].name == name {
				prev := b.htmlTags[k].prev
				b.htmlTags = b.htmlTags[:k]
				return prev
			}
		}
		return st
	}
	b.htmlTags = append(b.htmlTags, htmlTag{name: name, prev: st})
	return htmlTagStyle(name, in.Text, st)
}

// htmlImage adds an <img> within text: drawn at the size it asks for, and
// its tag too while the caret is at it.
func (b *builder) htmlImage(in *parser.Inline, src, alt string, sz imgSize, st istyle) {
	obj := b.e.sizedImage(src, alt, sz, b.width, false)
	if !b.revealed(in.Start, in.End) {
		b.atom(in.Start, in.End, "", st, obj)
		return
	}
	b.revealedAny = true
	b.marker(in.Start, in.End, st)
	b.deco(in.End, "", b.span(st), obj)
}

// htmlTagName reads the name of a tag of formatting, and whether it
// closes, from the text of an HTML inline.
func htmlTagName(tag string) (name string, closing, ok bool) {
	if !strings.HasPrefix(tag, "<") || !strings.HasSuffix(tag, ">") {
		return "", false, false
	}
	t := strings.TrimSpace(tag[1 : len(tag)-1])
	t = strings.TrimSuffix(t, "/")
	if strings.HasPrefix(t, "/") {
		closing = true
		t = t[1:]
	}
	t = strings.ToLower(strings.TrimSpace(t))
	if i := strings.IndexAny(t, " \t\n"); i >= 0 {
		t = t[:i]
	}
	switch t {
	case "b", "strong", "i", "em", "u", "ins", "s", "del", "strike", "mark", "sup", "sub", "kbd", "code", "br", "span", "font":
		return t, closing, true
	}
	return "", false, false
}

func isASCIIPunct(c byte) bool {
	return c >= '!' && c <= '/' || c >= ':' && c <= '@' || c >= '[' && c <= '`' || c >= '{' && c <= '~'
}

// texFallback shows TeX that could not be typeset as readable text.
func texFallback(tex string) string {
	return strings.TrimSpace(tex)
}

// display is a leaf's display: its text, pieces and spans.
type display struct {
	text       string
	pieces     []piece
	hasObjects bool
}

func (b *builder) display() display {
	return display{text: b.text.String(), pieces: b.pieces, hasObjects: b.hasObjects}
}

// ensure gives an empty display a piece at the content's start, the one
// place the caret can be.
func (b *builder) ensure(at int) {
	if len(b.pieces) == 0 {
		b.add(piece{kind: pText, s0: at - b.base, s1: at - b.base})
	}
}

// ---------------------------------------------------------------------------
// Mapping between the display and the source

// srcToDisp returns the display offset of the source offset rel (relative
// to the leaf), and whether the caret can be there: in a text piece, or at
// the edge of an atom.
func (d *display) srcToDisp(rel int) (int, bool) {
	for i := range d.pieces {
		p := &d.pieces[i]
		if rel < p.s0 || rel > p.s1 {
			continue
		}
		switch p.kind {
		case pText:
			return p.d0 + ui.UTF16Len(p.text[:rel-p.s0]), true
		case pAtom:
			if rel == p.s0 {
				return p.d0, true
			}
			if rel == p.s1 {
				return p.d1, true
			}
		case pDeco:
			// The caret is beside a decoration, at the piece after it or
			// before it.
		}
	}
	if len(d.pieces) == 0 && rel == 0 {
		return 0, true
	}
	return 0, false
}

// dispToSrc returns the source offset (relative to the leaf) of display
// offset u: of the smallest candidate when several pieces meet there with
// hidden syntax between them and up is set, else of the largest.
func (d *display) dispToSrc(u int, up bool) int {
	best, found := 0, false
	take := func(s int) {
		if !found || (up && s < best) || (!up && s > best) {
			best, found = s, true
		}
	}
	for i := range d.pieces {
		p := &d.pieces[i]
		if u < p.d0 || u > p.d1 {
			continue
		}
		switch p.kind {
		case pText:
			take(p.s0 + utf16ByteOffset(p.text, u-p.d0))
		case pAtom:
			if u == p.d0 {
				take(p.s0)
			} else if u == p.d1 {
				take(p.s1)
			} else if u-p.d0 <= p.d1-u {
				take(p.s0)
			} else {
				take(p.s1)
			}
		case pDeco:
			if u == p.d0 || u == p.d1 || p.d0 == p.d1 {
				take(p.s0)
			} else {
				take(p.s0)
			}
		}
	}
	if !found && len(d.pieces) > 0 {
		last := d.pieces[len(d.pieces)-1]
		if u > last.d1 {
			return last.s1
		}
		return d.pieces[0].s0
	}
	return best
}

// stops returns the source offsets (relative) the caret can take in the
// display, in order.
func (d *display) stops() []int {
	var out []int
	add := func(s int) {
		if len(out) == 0 || out[len(out)-1] != s {
			out = append(out, s)
		}
	}
	for i := range d.pieces {
		p := &d.pieces[i]
		switch p.kind {
		case pText:
			for k := 0; k <= len(p.text); {
				add(p.s0 + k)
				if k == len(p.text) {
					break
				}
				_, n := utf8.DecodeRuneInString(p.text[k:])
				k += n
			}
		case pAtom:
			add(p.s0)
			add(p.s1)
		case pDeco:
			add(p.s0)
		}
	}
	if len(out) == 0 {
		out = append(out, 0)
	}
	return out
}

// utf16ByteOffset returns the byte offset in s of UTF-16 unit u, keeping
// surrogate pairs whole.
func utf16ByteOffset(s string, u int) int {
	if u <= 0 {
		return 0
	}
	n := 0
	for i, r := range s {
		if n >= u {
			return i
		}
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return len(s)
}

// plainText returns the text of inlines, as parser.PlainText does, with
// the characters mdoc.CJKFriendly swapped taken back: a Text inline that
// is the parsed source as it is reads the editor's source instead.
func (e *Editor) plainText(ins []parser.Inline) string {
	parsed := e.doc.Source()
	marks := allHighlights(e.src, ins, 0, len(ins))
	var b strings.Builder
	for k := range ins {
		switch in := &ins[k]; in.Kind {
		case parser.Text:
			if in.End <= len(parsed) && in.End <= len(e.src) && in.Text == parsed[in.Start:in.End] {
				// The text as written, but for its ==s of highlight.
				at := in.Start
				for _, m := range marks {
					if m.at >= in.Start && m.at < in.End {
						b.WriteString(e.src[at:m.at])
						at = m.at + 2
					}
				}
				b.WriteString(e.src[at:in.End])
			} else {
				b.WriteString(in.Text)
			}
		case parser.Code, parser.HTML, parser.Math:
			b.WriteString(in.Text)
		case parser.SoftBreak, parser.LineBreak:
			b.WriteByte(' ')
		case parser.FootnoteRef:
			b.WriteString("[^" + in.Text + "]")
		}
	}
	return b.String()
}
