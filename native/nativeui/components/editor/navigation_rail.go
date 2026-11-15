package editor

import (
	"image/color"
	"time"

	"github.com/egoist/mygo/ui"

	core "muse/editor"
	"muse/locales"
)

const (
	headingRailWidth      = float32(28)
	headingRailInset      = float32(12)
	headingRailTickWidth  = float32(16)
	headingRailTickHeight = float32(2)
	headingRailItemHeight = float32(14)
	headingRailCardWidth  = float32(256)
)

type headingRailState struct {
	hover    int
	active   int
	revision uint64
	rect     ui.Rect
	ys       []float32
	pitch    float32
	scales   []float32
	cardY    float32
	cardOn   bool
	anim     bool
}

func (c *ProductionContent) drawHeadingRail(ctx *ui.Context, bounds ui.Rect, frame core.HeadingRailFrame, preview func(int) core.HeadingRailPreview, jump func(int)) {
	if !frame.Visible || len(frame.Items) < 2 {
		c.rail.hover = -1
		c.rail.active = -1
		c.rail.rect = ui.Rect{}
		c.rail.ys = nil
		c.rail.scales = nil
		c.rail.cardOn = false
		return
	}

	s := &c.rail
	if s.revision != frame.Revision {
		s.hover = -1
		s.cardOn = false
		s.revision = frame.Revision
	}
	s.anim = false
	s.active = frame.Active
	avail := bounds.H - 2*headingRailInset
	s.ys = headingRailPositions(s.ys, len(frame.Items), avail, &s.pitch)
	if len(s.scales) != len(frame.Items) {
		s.scales = make([]float32, len(frame.Items))
		for i := range s.scales {
			s.scales[i] = 1
		}
	}
	hi := s.hover
	if hi < 0 {
		hi = s.active
	}
	for i := range s.scales {
		dist := 1 << 20
		if hi >= 0 {
			dist = max(i-hi, hi-i)
		}
		headingRailStep(&s.scales[i], headingRailScale(dist), &s.anim)
	}
	strip := ui.Rect{X: bounds.X + bounds.W - headingRailWidth - headingRailInset, Y: bounds.Y + headingRailInset, W: headingRailWidth, H: avail}
	s.rect = strip
	ui.Overlay(ctx, func() {
		rail := ui.Box(ctx).Absolute().Left(strip.X).Top(strip.Y).Width(strip.W).Height(strip.H).
			Label(locales.EditorHeadingNavigation()).Cursor(ui.CursorPointer).Draw(func(p *ui.Painter, r ui.Rect) {
			drawHeadingRailTicks(p, r, s.ys, s.pitch, s.scales, s.active, s.hover, frame)
		})
		_, py, over := rail.PointerPosition()
		hover := -1
		if over {
			hover = headingRailIndexAt(s.ys, s.pitch, py)
		}
		if hover != s.hover {
			s.hover = hover
			if hover < 0 {
				s.cardOn = false
			}
			ctx.Invalidate()
		}
		if hover >= 0 && rail.Clicked() && jump != nil {
			jump(frame.Items[hover].Offset)
		}
		if s.hover >= 0 && s.hover < len(s.ys) {
			center := strip.Y + s.ys[s.hover]
			if !s.cardOn {
				s.cardOn = true
				s.cardY = strip.Y + strip.H/2
			}
			headingRailStep(&s.cardY, center, &s.anim)
			drawHeadingRailPreview(ctx, bounds, strip, s.cardY, s.hover, preview)
		}
	})
	if s.anim {
		ctx.After(16 * time.Millisecond)
	} else if s.hover >= 0 {
		ctx.After(80 * time.Millisecond)
	}
}

func headingRailScale(distance int) float32 {
	switch distance {
	case 0:
		return 1
	case 1:
		return 0.68
	case 2:
		return 0.44
	default:
		return 0.25
	}
}

func headingRailPositions(dst []float32, count int, height float32, pitch *float32) []float32 {
	dst = dst[:0]
	if count == 0 || height <= 0 {
		*pitch = 0
		return dst
	}
	*pitch = min(headingRailItemHeight, height/float32(count))
	top := (height - *pitch*float32(count)) / 2
	for i := 0; i < count; i++ {
		dst = append(dst, top+(float32(i)+0.5)*(*pitch))
	}
	return dst
}

func headingRailIndexAt(ys []float32, pitch, y float32) int {
	if len(ys) == 0 || pitch <= 0 {
		return -1
	}
	top := ys[0] - pitch/2
	if y < top-2 || y > top+pitch*float32(len(ys))+2 {
		return -1
	}
	return min(max(int((y-top)/pitch), 0), len(ys)-1)
}

func headingRailStep(value *float32, target float32, anim *bool) {
	delta := target - *value
	if delta > -0.01 && delta < 0.01 {
		*value = target
		return
	}
	*value += delta * 0.22
	*anim = true
}

func drawHeadingRailTicks(p *ui.Painter, r ui.Rect, ys []float32, pitch float32, scales []float32, active, hover int, frame core.HeadingRailFrame) {
	n := min(len(ys), len(scales))
	dense := pitch < headingRailTickHeight
	for i := 0; i < n; {
		w, col := headingRailTickStyle(i, scales, active, hover, frame)
		end := i + 1
		if dense {
			end = headingRailRunEnd(ys[:n], scales[:n], i, active, hover)
		}
		top := r.Y + ys[i] - headingRailTickHeight/2
		bottom := r.Y + ys[end-1] + headingRailTickHeight/2
		p.Fill(ui.Rect{X: r.X + r.W - w, Y: top, W: w, H: bottom - top}, col, headingRailTickHeight/2)
		i = end
	}
}

func headingRailTickStyle(i int, scales []float32, active, hover int, frame core.HeadingRailFrame) (float32, ui.Color) {
	col := headingRailColor(frame.MarkerColor)
	switch {
	case i == active:
		col = headingRailColor(frame.ActiveColor)
	case i == hover:
		col = headingRailColor(frame.HoverColor)
	}
	return headingRailTickWidth * scales[i], col
}

func headingRailColor(value color.RGBA) ui.Color {
	return ui.Color{R: value.R, G: value.G, B: value.B, A: value.A}
}

func headingRailRunEnd(ys, scales []float32, start, active, hover int) int {
	if start < 0 || start >= len(ys) || start >= len(scales) {
		return start
	}
	w := headingRailTickWidth * scales[start]
	style := headingRailStyleKind(start, active, hover)
	bottom := ys[start] + headingRailTickHeight/2
	end := start + 1
	for end < len(ys) && end < len(scales) {
		if headingRailTickWidth*scales[end] != w || headingRailStyleKind(end, active, hover) != style || ys[end]-headingRailTickHeight/2 > bottom {
			break
		}
		bottom = max(bottom, ys[end]+headingRailTickHeight/2)
		end++
	}
	return end
}

func headingRailStyleKind(i, active, hover int) uint8 {
	switch {
	case i == active:
		return 1
	case i == hover:
		return 2
	default:
		return 0
	}
}

func drawHeadingRailPreview(ctx *ui.Context, bounds, strip ui.Rect, center float32, index int, preview func(int) core.HeadingRailPreview) {
	if preview == nil {
		return
	}
	item := preview(index)
	if item.Title == "" {
		return
	}
	theme := ctx.Theme()
	right := strip.X - 8
	left := max(right-min(headingRailCardWidth, bounds.W-16), bounds.X+4)
	const cardHeight = float32(66)
	top := min(max(center-cardHeight/2, bounds.Y+6), max(bounds.Y+bounds.H-6-cardHeight, bounds.Y+6))
	shade := ui.RGBA(0, 0, 0, 0.12)
	if theme.Dark {
		shade = ui.RGBA(0, 0, 0, 0.4)
	}
	card := ui.Box(ctx).Absolute().Left(left).Top(top).Width(right-left).Radius(10).
		Padding(8, 12).Background(theme.Surface).Border(1, theme.Border).Shadow(0, 6, 18, 0, shade)
	card.Children(func() {
		ui.Column(ctx).GapY(2).Children(func() {
			ui.Text(ctx, locales.EditorHeadingLevelLabel(item.Level, item.Title)).FontSize(14).FontWeight(600).
				TextColor(theme.Text).SingleLine().Ellipsis("…")
			if item.Excerpt != "" {
				ui.Text(ctx, item.Excerpt).FontSize(12).TextColor(theme.TextMuted).MaxLines(2)
			}
		})
	})
}
