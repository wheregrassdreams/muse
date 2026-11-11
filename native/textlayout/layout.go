package textlayout

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

// Layout retains the original native layout for logical grapheme boundaries,
// and optionally composes native layouts of contiguous slices of that text.
// Segment indices are global UTF-16 offsets; no synthetic text is inserted.
type Layout struct {
	*ui.TextLayout
	segments      []segment
	width, height float32
	tracking      []trackingGap
}

type segment struct {
	layout     *Layout
	start, end int
	y          float32
}

func (l *Layout) Size() (float32, float32) {
	if len(l.segments) == 0 {
		w, h := l.TextLayout.Size()
		return w + l.trackingShift(1<<30), h
	}
	return l.width, l.height
}

func (l *Layout) Lines() []ui.TextLine {
	if len(l.segments) == 0 {
		if len(l.tracking) == 0 {
			return l.TextLayout.Lines()
		}
		lines := append([]ui.TextLine(nil), l.TextLayout.Lines()...)
		for i := range lines {
			lines[i].Rect.W += l.trackingShift(lines[i].End)
		}
		return lines
	}
	var out []ui.TextLine
	for _, s := range l.segments {
		for _, line := range s.layout.Lines() {
			line.Start += s.start
			line.End += s.start
			line.Rect.Y += s.y
			line.Baseline += s.y
			out = append(out, line)
		}
	}
	return out
}

func (l *Layout) Objects() []ui.TextObject {
	if len(l.segments) == 0 {
		if len(l.tracking) == 0 {
			return l.TextLayout.Objects()
		}
		objects := append([]ui.TextObject(nil), l.TextLayout.Objects()...)
		for i := range objects {
			objects[i].Rect.X += l.trackingShift(objects[i].Index)
		}
		return objects
	}
	var out []ui.TextObject
	for _, s := range l.segments {
		for _, object := range s.layout.Objects() {
			object.Index += s.start
			object.Rect.Y += s.y
			out = append(out, object)
		}
	}
	return out
}

// snappedIndex uses the original text's graphemes, including surrogate pairs
// and combining sequences, rather than treating a segment as new source text.
func (l *Layout) snappedIndex(index int) int {
	index = max(0, min(index, l.segments[len(l.segments)-1].end))
	if index == 0 {
		return 0
	}
	previous := l.PreviousBoundary(index)
	next := l.NextBoundary(previous)
	if next <= index {
		return next
	}
	return previous
}

func (l *Layout) segmentAt(p ui.TextCaretPosition) int {
	for i, s := range l.segments {
		if p.Index < s.end || p.Index == s.end && (p.Affinity == ui.TextUpstream || i+1 < len(l.segments) && l.segments[i+1].start > s.end) {
			return i
		}
	}
	return len(l.segments) - 1
}

func (l *Layout) Caret(index int) ui.Rect {
	if len(l.segments) == 0 {
		return l.CaretAt(ui.TextCaretPosition{Index: index, Affinity: ui.TextDownstream})
	}
	return l.CaretAt(ui.TextCaretPosition{Index: index, Affinity: ui.TextDownstream})
}

func (l *Layout) CaretAt(p ui.TextCaretPosition) ui.Rect {
	if len(l.segments) == 0 {
		r := l.TextLayout.CaretAt(p)
		r.X += l.trackingShift(p.Index)
		return r
	}
	p.Index = l.snappedIndex(p.Index)
	s := l.segments[l.segmentAt(p)]
	p.Index -= s.start
	r := s.layout.CaretAt(p)
	r.Y += s.y
	return r
}

func (l *Layout) PositionAt(point ui.Point) ui.TextCaretPosition {
	if len(l.segments) == 0 {
		point.X = l.untrackX(point.X)
		return l.TextLayout.PositionAt(point)
	}
	i := len(l.segments) - 1
	for j, s := range l.segments {
		_, h := s.layout.Size()
		if point.Y < s.y+h {
			i = j
			break
		}
	}
	s := l.segments[i]
	point.Y -= s.y
	p := s.layout.PositionAt(point)
	p.Index += s.start
	// A native layout's final edge can have downstream affinity (for
	// example, trimmed spaces). Retain the line hit at a composite wrap.
	if p.Index == s.end && i+1 < len(l.segments) {
		p.Affinity = ui.TextUpstream
	} else if p.Index == s.start && i > 0 {
		p.Affinity = ui.TextDownstream
	}
	return p
}

func (l *Layout) IndexAt(point ui.Point) int { return l.PositionAt(point).Index }

func (l *Layout) SelectionRects(ranges ...ui.TextInputRange) []ui.Rect {
	if len(l.segments) == 0 {
		rects := l.TextLayout.SelectionRects(ranges...)
		for i := range rects {
			left, right := l.trackX(rects[i].X), l.trackX(rects[i].X+rects[i].W)
			rects[i].X, rects[i].W = left, right-left
		}
		return rects
	}
	var out []ui.Rect
	for _, r := range ranges {
		if r.Start > r.End {
			r.Start, r.End = r.End, r.Start
		}
		for _, s := range l.segments {
			start, end := max(r.Start, s.start), min(r.End, s.end)
			if start >= end {
				continue
			}
			for _, rect := range s.layout.SelectionRects(ui.TextInputRange{Start: start - s.start, End: end - s.start}) {
				rect.Y += s.y
				out = append(out, rect)
			}
		}
	}
	return out
}

func (l *Layout) MoveCaret(p ui.TextCaretPosition, direction int) ui.TextCaretPosition {
	if len(l.segments) == 0 {
		return l.TextLayout.MoveCaret(p, direction)
	}
	if direction == 0 {
		return p
	}
	p.Index = l.snappedIndex(p.Index)
	i := l.segmentAt(p)
	s := l.segments[i]
	local := p
	local.Index -= s.start
	next := s.layout.MoveCaret(local, direction)
	if next != local {
		next.Index += s.start
		if next.Index == s.end && i+1 < len(l.segments) {
			next.Affinity = ui.TextUpstream
		} else if next.Index == s.start && i > 0 {
			next.Affinity = ui.TextDownstream
		}
		return next
	}
	step := 1
	if direction < 0 {
		step = -1
	}
	lines := s.layout.Lines()
	caret := s.layout.CaretAt(local)
	for _, line := range lines {
		if line.Rect.Y == caret.Y {
			if line.RTL {
				step = -step
			}
			break
		}
	}
	i += step
	if i < 0 || i >= len(l.segments) {
		return p
	}
	s = l.segments[i]
	lines = s.layout.Lines()
	line := lines[0]
	if step < 0 {
		line = lines[len(lines)-1]
	}
	x := line.Rect.X - 1
	if direction < 0 {
		x = line.Rect.X + line.Rect.W + 1
	}
	return l.PositionAt(ui.Point{X: x, Y: s.y + line.Rect.Y + line.Rect.H/2})
}

func (l *Layout) SelectionRanges(anchor, caret ui.TextCaretPosition) []ui.TextInputRange {
	if len(l.segments) == 0 {
		return l.TextLayout.SelectionRanges(anchor, caret)
	}
	lines := l.Lines()
	if len(lines) == 0 {
		return nil
	}
	ar, br := l.CaretAt(anchor), l.CaretAt(caret)
	ai, bi := 0, 0
	for i, line := range lines {
		if line.Rect.Y == ar.Y {
			ai = i
		}
		if line.Rect.Y == br.Y {
			bi = i
		}
	}
	ax, bx := ar.X, br.X
	if ai > bi {
		ai, bi, ax, bx = bi, ai, bx, ax
	}
	var out []ui.TextInputRange
	low, high := min(anchor.Index, caret.Index), max(anchor.Index, caret.Index)
	for i := 0; i+1 < len(l.segments); i++ {
		start, end := l.segments[i].end, l.segments[i+1].start
		if start < end && low < end && high > start {
			out = append(out, ui.TextInputRange{Start: max(low, start), End: min(high, end)})
		}
	}
	for i := ai; i <= bi; i++ {
		line := lines[i]
		lo, hi := line.Rect.X, line.Rect.X+line.Rect.W
		if ai == bi {
			lo, hi = min(ax, bx), max(ax, bx)
		} else {
			if i == ai {
				if line.RTL {
					hi = ax
				} else {
					lo = ax
				}
			}
			if i == bi {
				if line.RTL {
					lo = bx
				} else {
					hi = bx
				}
			}
		}
		// Sweep original graphemes against native visual edges, just as the
		// native gesture does. Logical segment endpoints need not be visual
		// line extremes in mixed bidi text, so selecting each segment from
		// logical start to end would lose parts of intermediate lines.
		for start := line.Start; start < line.End; {
			end := l.NextBoundary(start)
			if end <= start || end > line.End {
				break
			}
			x0 := l.CaretAt(ui.TextCaretPosition{Index: start, Affinity: ui.TextDownstream}).X
			x1 := l.CaretAt(ui.TextCaretPosition{Index: end, Affinity: ui.TextUpstream}).X
			left, right := min(x0, x1), max(x0, x1)
			if right > left && left >= lo-0.01 && right <= hi+0.01 {
				out = append(out, ui.TextInputRange{Start: start, End: end})
			}
			start = end
		}
		if i < bi && lines[i+1].Start > line.End {
			out = append(out, ui.TextInputRange{Start: line.End, End: lines[i+1].Start})
		}
	}
	slices.SortFunc(out, func(a, b ui.TextInputRange) int { return a.Start - b.Start })
	merged := out[:0]
	for _, r := range out {
		if n := len(merged); n > 0 && merged[n-1].End >= r.Start {
			merged[n-1].End = max(merged[n-1].End, r.End)
		} else {
			merged = append(merged, r)
		}
	}
	return merged
}

// Draw paints the same native layouts used for geometry queries.
func (l *Layout) Draw(p *ui.Painter, x, y float32, color ui.Color) {
	if len(l.segments) == 0 {
		l.drawTracked(p, x, y, color)
		return
	}
	for _, s := range l.segments {
		s.layout.Draw(p, x, y+s.y, color)
	}
}
