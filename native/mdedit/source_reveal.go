package mdedit

import "github.com/egoist/mygo/ui/markdown/parser"

// selectionReveals reports whether the editor's nonempty source selection
// overlaps a syntax range. Cell text selection can reveal inline syntax;
// ranges represented as a table structure keep their grid rendering.
func selectionReveals(e *Editor, s, end int, tableCell bool) bool {
	if e == nil {
		return false
	}
	if e.block != nil {
		for _, r := range e.SelectionRanges() {
			if r.Start < end && r.End > s {
				return true
			}
		}
		return false
	}
	a, z := e.textSelectionRange()
	if a == z {
		return false
	}
	if tableCell {
		for _, t := range e.leaves {
			if t.kind != kTable || e.tableStructureSelection(t) == nil {
				continue
			}
			if s >= t.lineStart && end <= blockEnd(e.src, t) {
				return false
			}
		}
	}
	return a < end && z > s
}

func selectionTouchesSourceRange(e *Editor, s, end int) bool {
	if e == nil || end <= s {
		return false
	}
	for _, r := range e.SelectionRanges() {
		if r.Start < end && r.End > s {
			return true
		}
	}
	return false
}

// quoteDepth is the number of source-backed quotation markers carried by a
// leaf. A callout is a quotation with an additional semantic projection.
func quoteDepth(l *leaf) int {
	if l == nil {
		return 0
	}
	n := 0
	for _, c := range l.conts {
		if c.kind == cQuote {
			n++
		}
	}
	return n
}

// quoteSourcePrefixes returns the Markdown prefix range on each physical
// source line represented by l. The first line can also contain list or
// indentation syntax; later lines use the parser's quoted continuation
// prefix. These ranges describe real source, independently of whether the
// current view exposes it.
func quoteSourcePrefixes(e *Editor, l *leaf) []Range {
	depth := quoteDepth(l)
	if e == nil || l == nil || depth == 0 {
		return nil
	}
	firstEnd := min(lineEndAt(e.src, l.lineStart), max(l.start, l.lineStart))
	var out []Range
	if prefixHasQuotes(e.src, l.lineStart, firstEnd, depth) {
		out = append(out, Range{Start: l.lineStart, End: firstEnd})
	}
	if l.id >= 0 {
		for _, in := range e.p.doc.Inlines(l.id) {
			if in.Kind != parser.SoftBreak && in.Kind != parser.LineBreak {
				continue
			}
			s, z, ok := quotePrefixForBreak(e.src, in.Start, in.End, depth)
			if ok {
				out = append(out, Range{Start: s, End: z})
			}
		}
	}
	return out
}

func prefixHasQuotes(src string, start, end, depth int) bool {
	if depth == 0 || end <= start {
		return false
	}
	n := 0
	for i := start; i < end; i++ {
		if src[i] == '>' {
			n++
		}
	}
	return n >= depth
}

func leadingQuotePrefixEnd(src string, start, end, depth int) (int, bool) {
	p := start
	for n := 0; n < depth; n++ {
		for p < end && (src[p] == ' ' || src[p] == '\t') {
			p++
		}
		if p >= end || src[p] != '>' {
			return 0, false
		}
		p++
		for p < end && (src[p] == ' ' || src[p] == '\t') {
			p++
		}
	}
	return p, true
}

func (e *Editor) quotePrefixOpen(l *leaf) bool {
	if l == nil {
		return false
	}
	return e.quotePrefixOpenAt(l, e.revealCaret()-l.lineStart)
}

func (e *Editor) quotePrefixOpenAt(l *leaf, rel int) bool {
	if l == nil {
		return false
	}
	caret := -1
	if rel >= 0 {
		caret = l.lineStart + rel
	}
	if e.calloutHeaderSourceOpenAt(l, caret) {
		return true
	}
	for _, r := range quoteSourcePrefixes(e, l) {
		if caret >= r.Start && caret < r.End || selectionTouchesSourceRange(e, r.Start, r.End) {
			return true
		}
	}
	return false
}

func (e *Editor) quotePrefixShift(l *leaf) float32 {
	if e.containerPrefixOpen(l) {
		return e.indentOf(l.conts)
	}
	return e.quoteIndentOf(l.conts)
}

func (e *Editor) quoteIndentOf(conts []cont) float32 {
	var shift float32
	for _, c := range conts {
		if c.kind == cQuote {
			if isCalloutContainer(c) {
				shift += e.lk.size * 1.4
			} else {
				shift += e.lk.size * quoteIndent
			}
		}
	}
	return shift
}

func quotePrefixForBreak(src string, start, end, depth int) (int, int, bool) {
	if depth == 0 || start >= end {
		return 0, 0, false
	}
	p := start
	for p < end && src[p] != '\n' {
		p++
	}
	if p < end {
		p++
	}
	if p >= end {
		return 0, 0, false
	}
	q, ok := leadingQuotePrefixEnd(src, p, end, depth)
	if !ok {
		return 0, 0, false
	}
	return p, q, true
}

// containerPrefixRange returns a container's source prefix on the first line
// it opens: indentation, list/quote/footnote markers, and their separator.
func containerPrefixRange(l *leaf) (int, int, bool) {
	if l == nil || len(l.firstOf) == 0 || l.isLine() || l.kind == kTable || l.table != nil {
		return 0, 0, false
	}
	return l.lineStart, l.start, l.lineStart < l.start
}

func itemPrefixRange(l *leaf) (int, int, bool) {
	if l == nil || !l.opensItem() {
		return 0, 0, false
	}
	return containerPrefixRange(l)
}

func (e *Editor) containerPrefixOpen(l *leaf) bool {
	s, end, ok := containerPrefixRange(l)
	if !ok {
		return false
	}
	caret := e.revealCaret()
	return caret >= s && caret < end && e.revealFor(l, caret) >= 0 ||
		selectionTouchesSourceRange(e, s, end)
}

func (e *Editor) itemPrefixOpen(l *leaf) bool {
	if _, _, ok := itemPrefixRange(l); !ok {
		return false
	}
	return e.containerPrefixOpen(l)
}

func (b *builder) containerPrefixRevealed(s, end, lineEnd int) bool {
	return b.reveal >= s && b.reveal < end || selectionTouchesSourceRange(b.e, s, end)
}
