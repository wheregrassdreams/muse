package mdedit

import "muse/mdedit/internal/parser"

// parserSource is what the parser reads, and everything the editor derives
// from the source for it: the document, the source it was built from, the
// masked source the document holds, the comments that mask hides, and where
// the front matter ends.
//
// It is one value on purpose. Every field here is a function of the source,
// and there are two ways to keep it: set reads the source whole, edit
// advances across one change. A field added here has exactly those two
// places to be filled in, and Editor.sync is the one caller that chooses
// between them — which is what keeps a fast path from drifting away from the
// full one. Nothing else writes these fields.
type parserSource struct {
	doc *parser.Document
	src string
	// mask is a mutable parser-text copy, made on the first incremental edit
	// only when masking changed the source. Before then doc.Source is enough.
	mask         []byte
	maskIsSource bool
	comments     []cmt
	imageInlines []parser.Inline
	fm           int
}

// clear drops what the parser read: a new source, or one the editor does
// not parse as Markdown at all. The document stays — an empty source has no
// parse, and the leaves are still built from a document — and set fills the
// rest again.
func (p *parserSource) clear() {
	*p = parserSource{doc: parser.New(exts)}
}

// set reads src whole: the comments and the front matter are masked out of
// it, and the document is built from what the parser then reads.
func (p *parserSource) set(src string, fm int) {
	if p.doc == nil {
		p.doc = parser.New(exts)
	}
	p.imageInlines = nil
	p.comments = nil
	masked := ""
	if fm == 0 {
		p.imageInlines, _ = largeDataImageOnlySource(src)
		if len(p.imageInlines) > 0 {
			masked = src
		}
	}
	if len(p.imageInlines) == 0 {
		p.comments = commentRegions(src)
		maskedSource := blankComments(maskFrontMatter(src, fm), p.comments)
		masked = parserText(maskedSource)
	}
	p.src, p.fm = src, fm
	if masked == src {
		p.mask, p.maskIsSource = nil, true
	} else {
		p.mask, p.maskIsSource = nil, false
	}
	if len(p.imageInlines) > 0 {
		p.doc.SetSingleLine(masked)
	} else {
		p.doc.Set(masked)
	}
}

func (p *parserSource) maskedSource() string {
	if p.doc != nil {
		return p.doc.Source()
	}
	if p.maskIsSource {
		return p.src
	}
	return string(p.mask)
}

// edit advances across one change of the source. src is the source after the
// change, pe the change. It reports whether it could follow the change;
// false means the source has to be read whole — through set — because the
// change reached something the fast path cannot reason about: the front
// matter the mask hides, a comment's markers, or the mask's own edit.
func (p *parserSource) edit(src string, pe *pendingEdit) bool {
	// Front matter is the first bytes of the source: an edit at its start
	// can add it or take it away.
	if pe.from <= 0 {
		return false
	}
	if p.fm > 0 && (pe.from <= p.fm || pe.to <= p.fm) {
		return false
	}
	// Which bytes a comment covers depends on the whole document: its
	// markers, the `\` that escapes one, the fences of ` and ~ that hide
	// them. An edit that keeps away from all of those only moves them.
	if touchesComment(p.comments, pe.from, pe.to) ||
		!commentsStay(p.src, src, pe.from, pe.to, len(pe.text)) {
		return false
	}
	if p.maskIsSource {
		if pe.from < 0 || pe.from > pe.to || pe.to > len(p.src) {
			return false
		}
		end := pe.to + len(src) - len(p.src)
		if end < pe.from || end > len(src) {
			return false
		}
		newFrom, newTo := lineStartAt(src, pe.from), lineEndAt(src, end)
		if line := src[newFrom:newTo]; parserText(line) != line {
			return false
		}
		p.doc.Edit(pe.from, pe.to, pe.text)
		p.src = src
		p.imageInlines = nil
		return true
	}
	delta := len(pe.text) - (pe.to - pe.from)
	for i := range p.comments {
		if c := &p.comments[i]; c.from >= pe.to {
			c.from += delta
			c.to += delta
		}
	}
	// The lines the edit rewrote are masked again, at the comments' new
	// place: the comments moved before the mask is edited.
	mask := p.mask
	if mask == nil {
		mask = []byte(p.doc.Source())
	}
	next, from, to, repl, ok := editMask(mask, src, pe.from, pe.to, p.fm, p.comments)
	if !ok {
		for i := range p.comments {
			if c := &p.comments[i]; c.from >= pe.to {
				c.from -= delta
				c.to -= delta
			}
		}
		return false
	}
	p.mask = next
	p.doc.Edit(from, to, repl)
	p.src = src
	p.imageInlines = nil
	return true
}
