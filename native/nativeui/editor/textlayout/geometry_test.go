package textlayout

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"
)

var geometryFont = ui.Font{Family: "sans-serif", Size: 16}

// Assemble independently of Shape so geometry failures do not depend on the
// planner choosing any particular breakpoints.
func geometryAssemble(parts ...string) *Layout {
	l := &Layout{TextLayout: ui.ShapeText(strings.Join(parts, ""), geometryFont, 0)}
	at := 0
	for _, part := range parts {
		native := ui.ShapeText(part, geometryFont, 0)
		end := at + len(utf16.Encode([]rune(part)))
		l.segments = append(l.segments, segment{layout: &Layout{TextLayout: native}, start: at, end: end, y: l.height})
		w, h := native.Size()
		l.width = max(l.width, w)
		l.height += h
		at = end
	}
	return l
}

func geometryEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestGeometryPlainForwardsNative(t *testing.T) {
	for _, text := range []string{"", "a😀e\u0301 中\nאבג def", "one two three four five"} {
		base := ui.ShapeText(text, geometryFont, 65)
		l := &Layout{TextLayout: base}
		w, h := l.Size()
		bw, bh := base.Size()
		geometryEqual(t, []float32{w, h}, []float32{bw, bh})
		geometryEqual(t, l.Lines(), base.Lines())
		geometryEqual(t, l.Objects(), base.Objects())
		end := len(utf16.Encode([]rune(text)))
		for index := -1; index <= end+1; index++ {
			geometryEqual(t, l.Caret(index), base.Caret(index))
			geometryEqual(t, l.PreviousBoundary(index), base.PreviousBoundary(index))
			geometryEqual(t, l.NextBoundary(index), base.NextBoundary(index))
			for _, affinity := range []ui.TextAffinity{ui.TextDownstream, ui.TextUpstream} {
				p := ui.TextCaretPosition{Index: index, Affinity: affinity}
				geometryEqual(t, l.CaretAt(p), base.CaretAt(p))
				for _, d := range []int{-1, 0, 1} {
					geometryEqual(t, l.MoveCaret(p, d), base.MoveCaret(p, d))
				}
				geometryEqual(t, l.SelectionRanges(ui.TextCaretPosition{}, p), base.SelectionRanges(ui.TextCaretPosition{}, p))
			}
		}
		for _, point := range []ui.Point{{X: -10, Y: -10}, {X: 30, Y: 5}, {X: 100, Y: h + 10}} {
			geometryEqual(t, l.PositionAt(point), base.PositionAt(point))
			geometryEqual(t, l.IndexAt(point), base.IndexAt(point))
		}
		ranges := []ui.TextInputRange{{Start: end, End: 0}, {Start: 1, End: 2}}
		geometryEqual(t, l.SelectionRects(ranges...), base.SelectionRects(ranges...))
	}
}

func TestGeometryOffsetsPreserveNativeMetadataAndObjects(t *testing.T) {
	l := geometryAssemble("😀 prefix ", "אבג\nsecond\ufffc")
	s := &l.segments[1]
	s.layout = &Layout{TextLayout: ui.ShapeRichTextWith([]ui.Span{
		{Text: "אבג\nsecond", Size: 21, BaselineShift: 2},
		{Text: "\ufffc", ObjectWidth: 23, ObjectHeight: 31, ObjectDescent: 4},
	}, geometryFont, ui.TextShape{Width: 170, Align: ui.Center})}
	w, h := s.layout.Size()
	l.width = max(l.width, w)
	l.height = s.y + h
	var wantLines []ui.TextLine
	var wantObjects []ui.TextObject
	for _, s := range l.segments {
		for _, line := range s.layout.Lines() {
			line.Start += s.start
			line.End += s.start
			line.Rect.Y += s.y
			line.Baseline += s.y
			wantLines = append(wantLines, line)
		}
		for _, object := range s.layout.Objects() {
			object.Index += s.start
			object.Rect.Y += s.y
			wantObjects = append(wantObjects, object)
		}
	}
	geometryEqual(t, l.Lines(), wantLines)
	geometryEqual(t, l.Objects(), wantObjects)
	if len(wantObjects) != 1 || !wantLines[1].RTL {
		t.Fatalf("fixture lost object or RTL metadata: %v, %v", wantObjects, wantLines)
	}
	gotW, gotH := l.Size()
	geometryEqual(t, []float32{gotW, gotH}, []float32{l.width, l.height})
}

func TestGeometryWrapAffinityAndHitTesting(t *testing.T) {
	l := geometryAssemble("one 😀 ", "e\u0301中 two", " last")
	for i := 1; i < len(l.segments); i++ {
		before, after := l.segments[i-1], l.segments[i]
		up := ui.TextCaretPosition{Index: after.start, Affinity: ui.TextUpstream}
		down := ui.TextCaretPosition{Index: after.start, Affinity: ui.TextDownstream}
		wantUp := before.layout.CaretAt(ui.TextCaretPosition{Index: before.end - before.start, Affinity: ui.TextUpstream})
		wantUp.Y += before.y
		wantDown := after.layout.CaretAt(ui.TextCaretPosition{})
		wantDown.Y += after.y
		geometryEqual(t, l.CaretAt(up), wantUp)
		geometryEqual(t, l.CaretAt(down), wantDown)
		geometryEqual(t, l.Caret(after.start), wantDown)
		geometryEqual(t, l.PositionAt(ui.Point{X: wantUp.X + 1, Y: wantUp.Y + wantUp.H/2}), up)
		geometryEqual(t, l.PositionAt(ui.Point{X: wantDown.X - 1, Y: wantDown.Y + wantDown.H/2}), down)
		geometryEqual(t, l.MoveCaret(up, 1), down)
		geometryEqual(t, l.MoveCaret(down, -1), up)
	}
	for _, s := range l.segments {
		for index := s.start + 1; index < s.end; index++ {
			for _, affinity := range []ui.TextAffinity{ui.TextDownstream, ui.TextUpstream} {
				p := ui.TextCaretPosition{Index: index, Affinity: affinity}
				if l.snappedIndex(index) == s.start && affinity == ui.TextUpstream {
					geometryEqual(t, l.CaretAt(p), l.CaretAt(ui.TextCaretPosition{Index: s.start, Affinity: affinity}))
					continue
				}
				local := p
				local.Index = l.snappedIndex(index) - s.start
				want := s.layout.CaretAt(local)
				want.Y += s.y
				geometryEqual(t, l.CaretAt(p), want)
			}
		}
	}
	geometryEqual(t, l.Caret(-10), l.Caret(0))
	geometryEqual(t, l.Caret(1000), l.Caret(l.segments[2].end))
	geometryEqual(t, l.IndexAt(ui.Point{X: 0, Y: 10000}), l.segments[2].start)
}

func TestGeometrySelectionUsesSourceUTF16(t *testing.T) {
	l := geometryAssemble("😀a ", "e\u0301中 ", "last")
	ranges := []ui.TextInputRange{{Start: 11, End: 1}, {Start: 5, End: 6}, {Start: 4, End: 4}}
	var want []ui.Rect
	for _, r := range ranges {
		if r.Start > r.End {
			r.Start, r.End = r.End, r.Start
		}
		for _, s := range l.segments {
			a, b := max(s.start, r.Start), min(s.end, r.End)
			if a >= b {
				continue
			}
			for _, rect := range s.layout.SelectionRects(ui.TextInputRange{Start: a - s.start, End: b - s.start}) {
				rect.Y += s.y
				want = append(want, rect)
			}
		}
	}
	geometryEqual(t, l.SelectionRects(ranges...), want)
	a := ui.TextCaretPosition{Index: 0}
	b := ui.TextCaretPosition{Index: 12, Affinity: ui.TextUpstream}
	full := []ui.TextInputRange{{Start: 0, End: 12}}
	geometryEqual(t, l.SelectionRanges(a, b), full)
	geometryEqual(t, l.SelectionRanges(b, a), full)
	geometryEqual(t, l.SelectionRanges(a, a), []ui.TextInputRange(nil))
	for i := 0; i <= 12; i++ {
		geometryEqual(t, l.PreviousBoundary(i), l.TextLayout.PreviousBoundary(i))
		geometryEqual(t, l.NextBoundary(i), l.TextLayout.NextBoundary(i))
	}
}

func TestGeometryNativeBidiAndMultilineWithinSegments(t *testing.T) {
	l := geometryAssemble("prefix ", "abc אבג def\nnext", "suffix")
	s := l.segments[1]
	for _, a := range []int{0, 2, 4, 5, 7, 10, 12} {
		for _, b := range []int{2, 5, 7, 9, 14} {
			anchor := ui.TextCaretPosition{Index: a, Affinity: ui.TextDownstream}
			caret := ui.TextCaretPosition{Index: b, Affinity: ui.TextUpstream}
			want := s.layout.SelectionRanges(anchor, caret)
			for i := range want {
				want[i].Start += s.start
				want[i].End += s.start
			}
			anchor.Index += s.start
			caret.Index += s.start
			geometryEqual(t, l.SelectionRanges(anchor, caret), want)
			for _, d := range []int{-1, 1} {
				local := ui.TextCaretPosition{Index: a}
				next := s.layout.MoveCaret(local, d)
				if next != local && next.Index > 0 && next.Index < s.end-s.start {
					next.Index += s.start
					geometryEqual(t, l.MoveCaret(anchor, d), next)
				}
			}
		}
	}
	// Full selection includes the hard newline inside the native segment.
	end := l.segments[len(l.segments)-1].end
	geometryEqual(t, l.SelectionRanges(ui.TextCaretPosition{}, ui.TextCaretPosition{Index: end, Affinity: ui.TextUpstream}), []ui.TextInputRange{{Start: 0, End: end}})
	// A logical endpoint inside a visual line must not truncate an
	// intermediate segment when the gesture spans beyond that segment.
	l = geometryAssemble("prefix ", "abc אבג", "suffix")
	end = l.segments[2].end
	geometryEqual(t, l.SelectionRanges(ui.TextCaretPosition{}, ui.TextCaretPosition{Index: end, Affinity: ui.TextUpstream}), []ui.TextInputRange{{Start: 0, End: end}})
}
