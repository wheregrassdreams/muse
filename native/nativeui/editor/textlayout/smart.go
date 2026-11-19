// Package textlayout adds bounded paragraph optimization to native text layouts.
// It never inserts characters: all line ranges retain the original UTF-16 indices.
package textlayout

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/go-text/typesetting/segmenter"
	knuth "github.com/tdewolff/canvas/text"
)

// Shape keeps native shaping, with optional Knuth-Plass paragraph breaks and
// bounded glue adjustment. Unsupported/oversized paragraphs use native wrapping.
func Shape(spans []ui.Span, font ui.Font, shape ui.TextShape, smart bool) *Layout {
	var base *ui.TextLayout
	native := func() *Layout {
		if base == nil {
			base = ui.ShapeRichTextWith(spans, font, shape)
		}
		return &Layout{TextLayout: base}
	}
	if !smart || shape.Width <= 0 || shape.Align != ui.Start || shape.Direction == ui.RightToLeft {
		return native()
	}
	// Reject oversized paragraphs before copying their text or allocating a
	// rune slice. The Knuth-Plass path is bounded to 4096 runes; paragraphs
	// beyond that limit already use the native layout above.
	runeCount, ok := boundedRuneCount(spans, 4096)
	if !ok {
		return native()
	}
	textString := ""
	if len(spans) == 1 {
		textString = spans[0].Text
	} else {
		var text strings.Builder
		for _, sp := range spans {
			text.WriteString(sp.Text)
		}
		textString = text.String()
	}
	runes := []rune(textString)
	// Bound the quadratic search of potential line breaks.
	if runeCount < 2 {
		return native()
	}
	for _, r := range runes {
		// Hard breaks and bidi paragraphs stay entirely on the native path.
		if r == '\r' || r == '\t' || r == '\u2028' || r == '\u2029' || unicode.In(r, unicode.Arabic, unicode.Hebrew) || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			return native()
		}
	}
	if strings.ContainsRune(textString, '\n') {
		out := native()
		start := 0
		for _, part := range strings.Split(textString, "\n") {
			end := start + utf16Length(part)
			chunk := Shape(sliceSpans(spans, start, end, nil), font, shape, true)
			if len(chunk.segments) == 0 {
				out.segments = append(out.segments, segment{layout: chunk, start: start, end: end, y: out.height})
			} else {
				for _, seg := range chunk.segments {
					seg.start, seg.end, seg.y = seg.start+start, seg.end+start, seg.y+out.height
					out.segments = append(out.segments, seg)
				}
			}
			w, h := chunk.Size()
			out.width, out.height = max(out.width, w), out.height+h
			start = end + 1
		}
		return out
	}
	units := make([]int, len(runes)+1)
	for i, r := range runes {
		units[i+1] = units[i] + utf16.RuneLen(r)
	}
	em := font.Size
	if em <= 0 {
		em = 16
	}
	measure := ui.ShapeRichTextWith(spans, font, ui.TextShape{})
	measureLines := measure.Lines()
	if len(measureLines) != 1 || measureLines[0].RTL {
		return native()
	}
	// Smart segments supply all wrapped geometry. Keep the unwrapped native
	// layout only for source grapheme boundaries; shaping a second wrapped
	// layout here would be discarded by Lines, CaretAt, and the other segment
	// methods below.
	out := &Layout{TextLayout: measure}
	compressed := make(map[int]float32)
	addPunctuationCompression(compressed, runes, units, spans, 0, len(runes), measure, em)
	measureWidth, _ := measure.Size()
	if measureWidth <= shape.Width {
		return trackedLine(measure, compressed, spans)
	}
	tokens := paragraphTokens(runes, units, spans, measure, em)
	if len(tokens) < 2 || len(tokens) > 512 {
		return native()
	}
	// Keep a short tail together so an isolated character or tiny code
	// fragment cannot become the final line. This is a box constraint,
	// not inserted whitespace or a forced source break.
	tail, tailWidth := len(tokens)-1, tokens[len(tokens)-1].width
	for tail > 1 && tailWidth < min(em*6, shape.Width*0.25) {
		tail--
		tailWidth += tokens[tail].width + tokens[tail].space
	}
	if tail < len(tokens)-1 {
		last := tokens[len(tokens)-1]
		last.start, last.width = tokens[tail].start, tailWidth
		tokens = append(tokens[:tail], last)
	}
	items := make(knuth.Items, 0, 6*len(tokens)+2)
	ends := make([]int, 0, cap(items))
	for i, token := range tokens {
		items = append(items, knuth.Box(float64(token.width)))
		ends = append(ends, token.end)
		if i < len(tokens)-1 {
			// A penalized ragged-right option prevents the solver's emergency
			// overfull lines when bounded interword glue cannot fill the width.
			// Opposing glue cancels inside a line; only at this penalty is
			// there extra right-side room. Zero-width CJK glue otherwise isn't
			// a legal breakpoint in Canvas's Knuth-Plass implementation.
			items = append(items, knuth.Glue(0, float64(em*3), 0), knuth.Penalty(0, 40, false), knuth.Glue(0, -float64(em*3), 0))
			ends = append(ends, token.end, token.end, token.end)
			items = append(items, knuth.Glue(float64(token.space), float64(token.stretch), float64(token.shrink)))
			ends = append(ends, token.end)
		}
	}
	items = append(items, knuth.Glue(0, math.Inf(1), 0), knuth.Penalty(0, -knuth.Infinity, true))
	ends = append(ends, len(runes), len(runes))
	// Glue capacities already encode the maximum acceptable adjustment.
	breaks := knuth.KnuthLinebreak(items, float64(shape.Width), 1, 0)
	if len(breaks) < 2 {
		return native()
	}
	start := 0
	for i, br := range breaks {
		if br.Position < 0 || br.Position >= len(ends) {
			return native()
		}
		end := ends[br.Position]
		if end <= start {
			return native()
		}
		trim := end
		for trim > start && unicode.IsSpace(runes[trim-1]) {
			trim--
		}
		lineSpans := sliceSpans(spans, units[start], units[end], nil)
		line := trackedLine(ui.ShapeRichTextWith(lineSpans, font, ui.TextShape{}), relativeGaps(compressed, units[start], units[end]), lineSpans)
		contentWidth := line.Caret(units[trim]-units[start]).X - line.Caret(0).X
		// Each chosen line is reshaped in context, rather than assuming that
		// widths measured in the whole paragraph are additive (kerning/padding).
		if contentWidth > shape.Width+0.5 {
			return native()
		}
		if i < len(breaks)-1 {
			extra := shape.Width - contentWidth
			positive, capacity := lineGaps(tokens, runes, units, start, trim)
			gaps := justifiedGaps(positive, capacity, extra)
			lineSpans = sliceSpans(spans, units[start], units[end], gaps)
			line = trackedLine(ui.ShapeRichTextWith(lineSpans, font, ui.TextShape{}), relativeGaps(compressed, units[start], units[end]), lineSpans)
			adjustedWidth := line.Caret(units[trim]-units[start]).X - line.Caret(0).X
			if adjustedWidth > shape.Width+0.5 {
				// Native shaping is not perfectly additive. If the scaled glue
				// still overfills after reshaping, keep this line ragged.
				line = trackedLine(ui.ShapeRichTextWith(sliceSpans(spans, units[start], units[end], nil), font, ui.TextShape{}), relativeGaps(compressed, units[start], units[end]), lineSpans)
			}
		}
		_, h := line.Size()
		out.segments = append(out.segments, segment{layout: line, start: units[start], end: units[end], y: out.height})
		out.height += h
		w, _ := line.Size()
		out.width = max(out.width, w)
		start = end
	}
	if start != len(runes) {
		return native()
	}
	return out
}

// boundedRuneCount stops as soon as the smart-layout limit is exceeded, so
// huge source blocks cost only a short scan before falling back to native
// wrapping. DecodeRune matches the replacement-rune behavior of []rune for
// malformed UTF-8.
func boundedRuneCount(spans []ui.Span, limit int) (int, bool) {
	n := 0
	for _, span := range spans {
		text := span.Text
		for len(text) > 0 {
			_, size := utf8.DecodeRuneInString(text)
			text = text[size:]
			n++
			if n > limit {
				return n, false
			}
		}
	}
	return n, true
}

type paragraphToken struct {
	start, end, trim              int // rune offsets
	width, space, stretch, shrink float32
}

func paragraphTokens(runes []rune, units []int, spans []ui.Span, measure *ui.TextLayout, em float32) []paragraphToken {
	var seg segmenter.Segmenter
	seg.Init(runes)
	iter := seg.LineIterator()
	start := 0
	var out []paragraphToken
	for iter.Next() {
		part := iter.Line()
		end := part.Offset + len(part.Text)
		if end < len(runes) && protectedBoundary(spans, units[end]) {
			continue
		}
		trim := end
		for trim > start && unicode.IsSpace(runes[trim-1]) {
			trim--
		}
		x0, x1, x2 := measure.Caret(units[start]).X, measure.Caret(units[trim]).X, measure.Caret(units[end]).X
		if x1 < x0 || x2 < x1 {
			return nil
		}
		token := paragraphToken{start: start, end: end, trim: trim, width: x1 - x0, space: x2 - x1}
		if trim < end {
			token.stretch, token.shrink = token.space*0.5, 0 // no glyph compression
		} else if end > start && unicode.In(runes[end-1], unicode.Han, unicode.Hiragana, unicode.Katakana) {
			token.stretch = em * 0.12
			if punctuationAdjacent(runes, end-1) {
				// Keep punctuation from becoming a conspicuous expansion point.
				token.stretch = em * 0.025
			}
		}
		out = append(out, token)
		start = end
	}
	return out
}

// Inline code/object spans are kept whole. Line opportunities otherwise come
// from Unicode UAX #14, including CJK punctuation and grapheme constraints.
func protectedBoundary(spans []ui.Span, unit int) bool {
	start := 0
	for _, sp := range spans {
		end := start + utf16Length(sp.Text)
		shortStyle := end-start <= 16 && (sp.Weight >= 600 || sp.Italic || sp.Strikethrough || sp.Underline)
		if start < unit && unit < end && (sp.Font != "" || sp.ObjectWidth != 0 || shortStyle) {
			return true
		}
		start = end
	}
	return false
}

func lineGaps(tokens []paragraphToken, runes []rune, units []int, start, trim int) (map[int]float32, float32) {
	gaps := make(map[int]float32)
	var capacity float32
	for _, token := range tokens {
		if token.start < start || token.end >= trim || token.stretch <= 0 {
			continue
		}
		if token.trim < token.end {
			count := token.end - token.trim
			for i := token.trim; i < token.end; i++ {
				stretch := token.stretch
				if spaceNearCJKPunctuation(runes, i) {
					stretch = 0 // preserve authored spaces at their natural width
				}
				gaps[units[i]] = stretch / float32(count)
				capacity += stretch / float32(count)
			}
		} else {
			stretch := token.stretch
			if punctuationAdjacent(runes, token.end-1) || punctuationAdjacent(runes, token.end) {
				stretch *= 0.25
			}
			gaps[units[token.end-1]] += stretch
			capacity += stretch
		}
	}
	return gaps, capacity
}

func justifiedGaps(positive map[int]float32, capacity, extra float32) map[int]float32 {
	if capacity <= 0 || extra <= 0 || extra > capacity {
		return make(map[int]float32)
	}
	gaps := make(map[int]float32, len(positive))
	for index, weight := range positive {
		if weight > 0 {
			gaps[index] = weight * extra / capacity
		}
	}
	return gaps
}

func addPunctuationCompression(gaps map[int]float32, runes []rune, units []int, spans []ui.Span, start, trim int, measure *ui.TextLayout, em float32) {
	for i := start; i < trim; i++ {
		if !plainSpanAt(spans, units[i]) || (i+1 < trim && !plainSpanAt(spans, units[i+1])) {
			continue
		}
		fraction := float32(0)
		if i+1 < trim {
			fraction = punctuationPairCompression(runes[i], runes[i+1])
		}
		if isOpeningPunctuation(runes[i]) && i+1 < trim && isCJK(runes[i+1]) {
			fraction = max(fraction, .2)
		}
		if isClosingPunctuation(runes[i]) && i > 0 && isCJK(runes[i-1]) {
			fraction = max(fraction, .2)
		}
		if fraction == 0 {
			continue
		}
		advance := measure.Caret(units[i+1]).X - measure.Caret(units[i]).X
		// Reclaim fullwidth bracket outer sidebearings and redundant space
		// between punctuation. Keep at least 70% of native advance; never squeeze
		// letters, code, authored spaces or ASCII punctuation.
		amount := min(em*fraction, max(0, advance*0.3))
		if amount > 0 {
			gaps[units[i]] -= amount
		}
	}
}

func plainSpanAt(spans []ui.Span, unit int) bool {
	at := 0
	for _, sp := range spans {
		end := at + utf16Length(sp.Text)
		if at <= unit && unit < end {
			return sp.Font == "" && sp.ObjectWidth == 0 && sp.LetterSpacing == 0 && sp.Weight == 0 && !sp.Italic && !sp.Underline && !sp.WavyUnderline && !sp.DottedUnderline && !sp.Strikethrough && sp.BaselineShift == 0
		}
		at = end
	}
	return false
}

func spaceNearCJKPunctuation(runes []rune, i int) bool {
	if i < 0 || i >= len(runes) || !unicode.IsSpace(runes[i]) {
		return false
	}
	left, right := i-1, i+1
	for left >= 0 && unicode.IsSpace(runes[left]) {
		left--
	}
	for right < len(runes) && unicode.IsSpace(runes[right]) {
		right++
	}
	return left >= 0 && isCJKPunctuation(runes[left]) || right < len(runes) && isCJKPunctuation(runes[right])
}

func punctuationAdjacent(runes []rune, i int) bool {
	if i < 0 || i >= len(runes) {
		return false
	}
	if isCJKPunctuation(runes[i]) {
		return i > 0 && isCJK(runes[i-1]) || i+1 < len(runes) && isCJK(runes[i+1])
	}
	return isCJK(runes[i]) && (i > 0 && isCJKPunctuation(runes[i-1]) || i+1 < len(runes) && isCJKPunctuation(runes[i+1]))
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana)
}

func isCJKPunctuation(r rune) bool {
	return strings.ContainsRune("，。；：、！？（）［］【】｛｝〈〉《》「」『』〔〕〝〟", r)
}

// Opening marks are right-aligned and closing/separating marks left-aligned
// in CJK fonts. Adjacent marks otherwise accumulate full sidebearings.
func isOpeningPunctuation(r rune) bool {
	return strings.ContainsRune("（［【｛〈《「『〔〝", r)
}
func isClosingPunctuation(r rune) bool {
	return strings.ContainsRune("）］】｝〉》」』〕〟", r)
}

func punctuationPairCompression(left, right rune) float32 {
	closing := isClosingPunctuation(left)
	separator := strings.ContainsRune("，。；：、！？", left)
	if (closing || separator) && isOpeningPunctuation(right) {
		return .25
	}
	if closing && strings.ContainsRune("，。；：、！？）］】｝〉》」』〕〟", right) {
		return .1
	}
	return 0
}

func utf16Length(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// sliceSpans preserves every native style and object metric. Glue changes only
// the advance of selected whitespace/Han characters, never source characters.
func sliceSpans(spans []ui.Span, from, to int, gaps map[int]float32) []ui.Span {
	var out []ui.Span
	at := 0
	for _, sp := range spans {
		var run strings.Builder
		flush := func() {
			if run.Len() > 0 {
				copy := sp
				copy.Text = run.String()
				out = append(out, copy)
				run.Reset()
			}
		}
		for _, r := range sp.Text {
			n := utf16.RuneLen(r)
			if at >= from && at+n <= to {
				if gap, ok := gaps[at]; ok {
					flush()
					copy := sp
					copy.Text = string(r)
					copy.LetterSpacing += gap
					out = append(out, copy)
				} else {
					run.WriteRune(r)
				}
			}
			at += n
		}
		flush()
		if at >= to {
			break
		}
	}
	return out
}
