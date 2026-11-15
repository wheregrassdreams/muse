package mdedit

import (
	"sort"

	"muse/mdedit/internal/parser"
)

func normalizeSourceRevealRanges(in []Range) []Range {
	less := func(a, b Range) bool {
		return a.Start < b.Start || a.Start == b.Start && a.End < b.End
	}
	for i, r := range in {
		if r.End < r.Start || i > 0 && less(r, in[i-1]) {
			return normalizeRanges(in)
		}
	}
	if len(in) == 0 {
		return nil
	}
	out := make([]Range, 0, len(in))
	for _, r := range in {
		if r.End < r.Start {
			r.Start, r.End = r.End, r.Start
		}
		if r.Start == r.End {
			continue
		}
		if n := len(out); n > 0 && r.Start < out[n-1].End {
			if r.End > out[n-1].End {
				out[n-1].End = r.End
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

// sourceRangeTargeted reports whether a selection or active search match
// overlaps source syntax. Caret reveal remains caller-specific because some
// source objects open only when the caret is inside their body.
func sourceRangeTargeted(e *Editor, s, end int, tableCell bool) bool {
	if e == nil || end <= s {
		return false
	}
	if selectionReveals(e, s, end, tableCell) {
		return true
	}
	if len(e.sourceRevealRanges) == 0 {
		return false
	}
	// The index is sorted and non-overlapping, so this remains logarithmic
	// even for large result sets while layout visits each visible leaf.
	i := sort.Search(len(e.sourceRevealRanges), func(i int) bool {
		return e.sourceRevealRanges[i].End > s
	})
	return i < len(e.sourceRevealRanges) && e.sourceRevealRanges[i].Start < end
}

func sourceRangeRevealed(e *Editor, s, end int, tableCell bool, caret int) bool {
	return caret >= s && caret <= end || sourceRangeTargeted(e, s, end, tableCell)
}

// sourceRevealFingerprint identifies the active matches that touch one leaf.
// Views for unaffected leaves stay reusable during incremental search edits.
func (e *Editor) sourceRevealFingerprint(s, end int) uint64 {
	if e == nil || end <= s || len(e.sourceRevealRanges) == 0 {
		return 0
	}
	i := sort.Search(len(e.sourceRevealRanges), func(i int) bool {
		return e.sourceRevealRanges[i].End > s
	})
	const offset = uint64(1469598103934665603)
	const prime = uint64(1099511628211)
	h := offset
	found := false
	for ; i < len(e.sourceRevealRanges) && e.sourceRevealRanges[i].Start < end; i++ {
		r := e.sourceRevealRanges[i]
		if r.End <= s {
			continue
		}
		found = true
		for _, n := range [...]uint64{uint64(max(r.Start, s) - s), uint64(min(r.End, end) - s)} {
			for shift := uint(0); shift < 64; shift += 8 {
				h ^= uint64(byte(n >> shift))
				h *= prime
			}
		}
	}
	if !found {
		return 0
	}
	return h
}

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
	// Most document leaves are not inside a quote. Avoid checking callout and
	// quoted source prefixes for them during every virtual layout pass.
	if l == nil || quoteDepth(l) == 0 {
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
		if caret >= r.Start && caret < r.End || sourceRangeTargeted(e, r.Start, r.End, false) {
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

// editablePrefixRange returns source syntax that becomes navigable when a
// caret moves left from the first rendered character. List/container markers
// already use this rule; headings opt in through HeadingMarkerReveal.
func (e *Editor) editablePrefixRange(l *leaf) (int, int, bool) {
	if l == nil {
		return 0, 0, false
	}
	if l.kind == kHeading && e.opts.HeadingMarkerReveal {
		return l.lineStart, l.start, l.lineStart < l.start
	}
	return containerPrefixRange(l)
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
		sourceRangeTargeted(e, s, end, false)
}

func (e *Editor) itemPrefixOpen(l *leaf) bool {
	if _, _, ok := itemPrefixRange(l); !ok {
		return false
	}
	return e.containerPrefixOpen(l)
}

func (b *builder) containerPrefixRevealed(s, end, lineEnd int) bool {
	return b.reveal >= s && b.reveal < end || sourceRangeTargeted(b.e, s, end, false)
}
