package textlayout

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"
)

func TestSmartParagraphRetainsSourceAndNativeGeometry(t *testing.T) {
	for _, text := range []string{
		strings.Repeat("A paragraph with balanced word spacing and natural native font shaping. ", 5),
		strings.Repeat("中文排版应当保留原始内容，智能选择断行位置，并使中英文 spacing 保持自然。", 5),
		strings.Repeat("组合 e\u0301 与 emoji 👩‍👩‍👧‍👦 should never split their grapheme clusters. ", 5),
	} {
		spans := []ui.Span{{Text: text}}
		shape := ui.TextShape{Width: 320}
		font := ui.Font{Size: 16, LineHeight: 1.75}
		l := Shape(spans, font, shape, true)
		if len(l.segments) < 2 {
			t.Fatalf("smart path not used: %q", text[:25])
		}
		end := 0
		adjusted := false
		for _, seg := range l.segments {
			if seg.start != end || seg.end <= seg.start {
				t.Fatalf("source gap: %+v after %d", seg, end)
			}
			end = seg.end
			natural := ui.ShapeRichTextWith(sliceSpans(spans, seg.start, seg.end, nil), font, ui.TextShape{})
			w, _ := seg.layout.Size()
			nw, _ := natural.Size()
			if w > nw+0.1 {
				adjusted = true
			}
			lines := seg.layout.Lines()
			if len(lines) != 1 {
				t.Fatal("optimized line wrapped again")
			}
			// All glyph positions still come from native shaped line geometry.
			x := seg.layout.Caret(0).X
			y := l.CaretAt(ui.TextCaretPosition{Index: seg.start, Affinity: ui.TextDownstream})
			if y.X != x || y.Y < seg.y {
				t.Fatalf("caret disagrees with line: %+v, %+v", y, seg)
			}
		}
		if end != utf16Length(text) {
			t.Fatal("source characters lost")
		}
		if !adjusted {
			t.Fatal("paragraph did not adjust any glue")
		}
	}
}

func TestTypographyCacheBoundAndInvalidation(t *testing.T) {
	var cache Cache
	font, shape := ui.Font{Size: 16}, ui.TextShape{Width: 320}
	spans := []ui.Span{{Text: strings.Repeat("A native paragraph with adjustable spacing. ", 6)}}
	first := cache.Shape(spans, font, shape)
	if next := cache.Shape(spans, font, shape); first != next {
		t.Fatal("unchanged paragraph recalculated")
	}
	shape.Width = 300
	if next := cache.Shape(spans, font, shape); first == next {
		t.Fatal("width did not invalidate")
	}
	spans[0].Underline = true
	if next := cache.Shape(spans, font, ui.TextShape{Width: 320}); first == next {
		t.Fatal("style did not invalidate")
	}
	for i := 0; i < 30; i++ {
		spans[0].Text = strings.Repeat("paragraph ", 40) + strings.Repeat("x", i)
		cache.Shape(spans, font, shape)
	}
	if len(cache.entries) > 16 || cache.units > 16384 {
		t.Fatal("cache limit exceeded")
	}
	cache.Clear()
	if len(cache.entries) != 0 || cache.units != 0 {
		t.Fatal("cache clear failed")
	}
}

func BenchmarkCachedSmartParagraph(b *testing.B) {
	var cache Cache
	spans := []ui.Span{{Text: strings.Repeat("中文正文 mixed with links and ordinary prose should remain responsive. ", 12)}}
	cache.Shape(spans, ui.Font{Size: 16}, ui.TextShape{Width: 640})
	b.ResetTimer()
	for b.Loop() {
		cache.Shape(spans, ui.Font{Size: 16}, ui.TextShape{Width: 640})
	}
}

func TestSmartTypographyFallbackAndOptOut(t *testing.T) {
	for _, text := range []string{"one short line", "مرحبا بالعالم " + strings.Repeat("word ", 100), strings.Repeat("long ", 1200)} {
		l := Shape([]ui.Span{{Text: text}}, ui.Font{Size: 16}, ui.TextShape{Width: 200}, true)
		if len(l.segments) != 0 {
			t.Fatal("unsafe paragraph did not use native fallback")
		}
	}
	text := strings.Repeat("Native text remains untouched when the setting is off. ", 10)
	l := Shape([]ui.Span{{Text: text}}, ui.Font{Size: 16}, ui.TextShape{Width: 300}, false)
	if len(l.segments) != 0 || len(l.Lines()) < 2 {
		t.Fatal("off mode did not retain native wrapping")
	}
}

func TestExplicitNewlineGeometryAndSelection(t *testing.T) {
	const text = "first line\nsecond line"
	l := Shape([]ui.Span{{Text: text}}, ui.Font{Size: 16}, ui.TextShape{Width: 300}, true)
	if len(l.Lines()) != 2 {
		t.Fatal("source newline not preserved")
	}
	before := utf16Length("first line")
	first := l.Caret(before)
	second := l.Caret(before + 1)
	if first.Y >= second.Y {
		t.Fatal("caret before newline moved to next line")
	}
	ranges := l.SelectionRanges(ui.TextCaretPosition{}, ui.TextCaretPosition{Index: utf16Length(text)})
	if len(ranges) != 1 || ranges[0].Start != 0 || ranges[0].End != utf16Length(text) {
		t.Fatalf("selection lost newline: %+v", ranges)
	}
}

func TestSmartTypographyObjectsAndStyles(t *testing.T) {
	spans := []ui.Span{{Text: strings.Repeat("Text and ", 15)}, {Text: "\ufffc", ObjectWidth: 44, ObjectHeight: 23}, {Text: strings.Repeat(" linked words ", 15), Color: ui.Hex("#526b82"), Underline: true}}
	l := Shape(spans, ui.Font{Size: 16}, ui.TextShape{Width: 300}, true)
	if len(l.segments) < 2 || len(l.Objects()) != 1 {
		t.Fatal("inline object lost in smart layout")
	}
	want := utf16Length(spans[0].Text)
	if got := l.Objects()[0].Index; got != want {
		t.Fatalf("object source offset=%d want %d", got, want)
	}
	// Slicing preserves decorations and object metrics while changing glue.
	sliced := sliceSpans(spans, want, want+1, nil)
	if len(sliced) != 1 || sliced[0].ObjectWidth != 44 || sliced[0].ObjectHeight != 23 {
		t.Fatal("object metrics changed")
	}
}

func TestPunctuationAwareCJKSpacingAndSourceGeometry(t *testing.T) {
	text := strings.Repeat("（圆括号），【方括号】、 「引号」 mixed English, words. ", 8)
	spans := []ui.Span{{Text: text}}
	font := ui.Font{Family: "sans-serif", Size: 16}
	shape := ui.TextShape{Width: 250}
	l := Shape(spans, font, shape, true)
	if len(l.segments) < 2 {
		t.Fatal("expected optimized lines")
	}
	end := 0
	for _, seg := range l.segments {
		if seg.start != end || seg.end <= seg.start {
			t.Fatalf("source mapping is discontinuous: %+v after %d", seg, end)
		}
		end = seg.end
		if lines := seg.layout.Lines(); len(lines) != 1 {
			t.Fatalf("optimized line wrapped again: %d", len(lines))
		}
		trim := seg.end
		for trim > seg.start && unicode.IsSpace([]rune(text)[trim-1]) {
			trim--
		}
		contentWidth := seg.layout.Caret(trim-seg.start).X - seg.layout.Caret(0).X
		if contentWidth > shape.Width+0.5 {
			t.Fatalf("optimized line overflowed: width %.2f > %.2f", contentWidth, shape.Width)
		}
		for i := seg.start; i < seg.end; i++ {
			a := l.CaretAt(ui.TextCaretPosition{Index: i, Affinity: ui.TextDownstream})
			b := l.CaretAt(ui.TextCaretPosition{Index: i + 1, Affinity: ui.TextUpstream})
			if a.Y != b.Y {
				continue // a grapheme boundary or a line edge may use another visual edge.
			}
			if a.X > b.X+0.01 {
				t.Fatalf("punctuation spacing reversed source geometry at %d: %.2f > %.2f", i, a.X, b.X)
			}
		}
	}
	if end != utf16Length(text) {
		t.Fatalf("source end=%d want %d", end, utf16Length(text))
	}
	selection := l.SelectionRects(ui.TextInputRange{Start: 0, End: end})
	if len(selection) < 2 {
		t.Fatalf("display selection did not follow optimized lines: %v", selection)
	}

	runes := []rune("汉（字）汉，字、汉 English, words")
	units := make([]int, len(runes)+1)
	for i, r := range runes {
		units[i+1] = units[i] + utf16.RuneLen(r)
	}
	measure := ui.ShapeText(string(runes), font, 0)
	tokens := paragraphTokens(runes, units, []ui.Span{{Text: string(runes)}}, measure, font.Size)
	for _, token := range tokens {
		if token.end > 0 && punctuationAdjacent(runes, token.end-1) && token.trim == token.end && token.stretch > font.Size*0.025+0.01 {
			t.Fatalf("punctuation-adjacent stretch too large: %+v", token)
		}
	}
	for _, pair := range []string{"）、", "、【", "】、", "、「"} {
		if punctuationPairCompression([]rune(pair)[0], []rune(pair)[1]) <= 0 {
			t.Fatalf("missing fullwidth punctuation pair %q", pair)
		}
	}
	for _, pair := range []string{"汉（", "（字", "字）", "汉，", "，字", "d,", ", ", "、 "} {
		if punctuationPairCompression([]rune(pair)[0], []rune(pair)[1]) != 0 {
			t.Fatalf("ordinary text or authored space compressed: %q", pair)
		}
	}
}

func TestRaggedLineDropsPositiveGlueAndPunctuationSpacesStayNatural(t *testing.T) {
	positive := map[int]float32{1: 3, 4: 2}
	for _, extra := range []float32{0, 5.1, 8} {
		if got := justifiedGaps(positive, 5, extra); len(got) != 0 {
			t.Fatalf("ragged line retained positive glue for extra=%v: %v", extra, got)
		}
	}
	justified := justifiedGaps(positive, 5, 2.5)
	if justified[1]+justified[4] < 2.49 || justified[1]+justified[4] > 2.51 {
		t.Fatalf("valid justification did not use requested capacity: %v", justified)
	}

	runes := []rune("汉， 字 汉")
	units := make([]int, len(runes)+1)
	for i, r := range runes {
		units[i+1] = units[i] + utf16.RuneLen(r)
	}
	tokens := []paragraphToken{{start: 0, end: 3, trim: 2, stretch: 4}}
	gaps, capacity := lineGaps(tokens, runes, units, 0, len(runes))
	if gaps[units[2]] != 0 || capacity != 0 {
		t.Fatalf("author space beside punctuation became stretch glue: gaps=%v capacity=%v", gaps, capacity)
	}

	font := ui.Font{Family: "sans-serif", Size: 16}
	measure := ui.ShapeText("（汉）", font, 0)
	code := []ui.Span{{Text: "（汉）", Font: "monospace"}}
	codeUnits := []int{0, 1, 2, 3}
	compressed := make(map[int]float32)
	addPunctuationCompression(compressed, []rune("（汉）"), codeUnits, code, 0, 3, measure, font.Size)
	if len(compressed) != 0 {
		t.Fatalf("code span received punctuation compression: %v", compressed)
	}
}

func BenchmarkSmartParagraph(b *testing.B) {
	spans := []ui.Span{{Text: strings.Repeat("中文正文 mixed with links and ordinary prose should remain responsive. ", 12)}}
	for b.Loop() {
		Shape(spans, ui.Font{Size: 16}, ui.TextShape{Width: 640}, true)
	}
}

func TestFullwidthPunctuationPairsCompressOnShortAndFinalLines(t *testing.T) {
	font := ui.Font{Family: "sans-serif", Size: 20}
	for _, text := range []string{"括号（圆括号）、【方括号】、「引号」，结束", "first line\n括号（圆括号）、【方括号】、「引号」，结束"} {
		spans := []ui.Span{{Text: text}}
		native := Shape(spans, font, ui.TextShape{Width: 1000}, false)
		smart := Shape(spans, font, ui.TextShape{Width: 1000}, true)
		nw, _ := native.Size()
		sw, _ := smart.Size()
		if nw-sw < font.Size*.5 {
			t.Fatalf("pair spacing still excessive: native=%v smart=%v", nw, sw)
		}
		for i := 0; i < utf16Length(text); i++ {
			a, b := smart.Caret(i), smart.Caret(i+1)
			if a.Y == b.Y && b.X < a.X {
				t.Fatalf("caret reversed at %d", i)
			}
		}
	}
	for _, spans := range [][]ui.Span{
		{{Text: "代码）、【原样", Font: "monospace"}},
		{{Text: "English, words. and (brackets)"}},
	} {
		a := Shape(spans, font, ui.TextShape{Width: 1000}, false)
		b := Shape(spans, font, ui.TextShape{Width: 1000}, true)
		aw, _ := a.Size()
		bw, _ := b.Size()
		if aw != bw {
			t.Fatalf("protected text changed advance: %v != %v", aw, bw)
		}
	}
}

func TestPunctuationTrackingKeepsAuthoredSpaceAndCaretGeometry(t *testing.T) {
	const text = "括号（圆括号）、 【方括号】、「引号」，后文"
	font := ui.Font{Size: 20}
	spans := []ui.Span{{Text: text}}
	a := Shape(spans, font, ui.TextShape{Width: 1000}, false)
	b := Shape(spans, font, ui.TextShape{Width: 1000}, true)
	for i, r := range []rune(text) {
		if r == ' ' {
			aw := a.Caret(i+1).X - a.Caret(i).X
			bw := b.Caret(i+1).X - b.Caret(i).X
			if abs(aw-bw) > .01 {
				t.Fatalf("authored space changed: %v vs %v", aw, bw)
			}
		}
		caret := b.Caret(i)
		if got := b.PositionAt(ui.Point{X: caret.X, Y: caret.Y + caret.H/2}).Index; got != i {
			t.Fatalf("compressed caret roundtrip %d != %d", got, i)
		}
	}
	rects := b.SelectionRects(ui.TextInputRange{Start: 0, End: utf16Length(text)})
	if len(rects) != 1 {
		t.Fatalf("selection split unexpectedly: %v", rects)
	}
	end := b.Caret(utf16Length(text))
	if abs(rects[0].W-end.X) > .1 {
		t.Fatalf("selection disagrees with caret: %v vs %v", rects, end)
	}
}
func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
