package textlayout

import (
	"github.com/egoist/mygo/ui"
	"sort"
	"unicode/utf16"
)

// Some native fallback fonts ignore per-span negative tracking. Preserve the
// original shaping runs and reclaim punctuation sidebearings in both paint and
// geometry, rather than trusting a tracking attribute that may be ignored.
type trackingGap struct {
	index              int
	left, right, delta float32
	leading            bool
}

func trackedLine(base *ui.TextLayout, gaps map[int]float32, spans []ui.Span) *Layout {
	leading := map[int]bool{}
	at := 0
	for _, sp := range spans {
		for _, r := range sp.Text {
			if isOpeningPunctuation(r) {
				leading[at] = true
			}
			at += utf16.RuneLen(r)
		}
	}
	l := &Layout{TextLayout: base}
	indices := make([]int, 0, len(gaps))
	for i, g := range gaps {
		if g < 0 {
			indices = append(indices, i)
		}
	}
	sort.Ints(indices)
	for _, i := range indices {
		left, right := base.Caret(i).X, base.Caret(i+1).X
		delta := max(gaps[i], -(right-left)*.3)
		if delta < 0 {
			l.tracking = append(l.tracking, trackingGap{i + 1, left, right, delta, leading[i]})
		}
	}
	return l
}

func relativeGaps(gaps map[int]float32, from, to int) map[int]float32 {
	out := make(map[int]float32)
	for i, g := range gaps {
		if from <= i && i+1 <= to {
			out[i-from] = g
		}
	}
	return out
}

func (l *Layout) trackingShift(index int) float32 {
	var shift float32
	for _, g := range l.tracking {
		if g.index > index {
			break
		}
		shift += g.delta
	}
	return shift
}

func (l *Layout) trackX(x float32) float32 {
	var shift float32
	for _, g := range l.tracking {
		if x < g.left {
			break
		}
		if x < g.right {
			return x + shift + g.delta*(x-g.left)/(g.right-g.left)
		}
		shift += g.delta
	}
	return x + shift
}

func (l *Layout) untrackX(x float32) float32 {
	var shift float32
	for _, g := range l.tracking {
		left, right := g.left+shift, g.right+shift+g.delta
		if x < left {
			break
		}
		if x < right {
			return g.left + (x-left)*(g.right-g.left)/(right-left)
		}
		shift += g.delta
	}
	return x - shift
}

func (l *Layout) drawTracked(p *ui.Painter, x, y float32, color ui.Color) {
	if len(l.tracking) == 0 {
		p.TextLayout(l.TextLayout, x, y, color)
		return
	}
	_, h := l.TextLayout.Size()
	start, shift := float32(-64), float32(0)
	draw := func(end float32) {
		p.Clip(ui.Rect{X: x + start, Y: y - 64, W: max(0, end-start), H: h + 128}, 0, func() { p.TextLayout(l.TextLayout, x+shift, y, color) })
		start = end
	}
	for _, g := range l.tracking {
		if g.leading {
			draw(g.left + shift)
			shift += g.delta
		} else {
			draw(g.right + shift)
			shift += g.delta
			start = g.right + shift
		}
	}
	w, _ := l.TextLayout.Size()
	draw(w + shift + 64)
}
