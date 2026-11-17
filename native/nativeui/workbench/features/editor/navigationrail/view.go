// Package navigationrail renders a detached heading-navigation frame and
// owns its transient pointer and animation state. Source lookup and heading
// jumps remain in the editor host and are supplied as typed callbacks.
package navigationrail

import (
	"image/color"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

const (
	Width      = float32(28)
	Inset      = float32(12)
	TickWidth  = float32(16)
	TickHeight = float32(2)
	ItemHeight = float32(14)
	CardWidth  = float32(256)
)

type Item struct {
	Offset int
	Level  int
}

// ViewportSize is the available editor area used to decide whether the rail
// should be projected into a host frame.
type ViewportSize struct{ Width, Height float32 }

type Frame struct {
	Visible     bool
	Revision    uint64
	Active      int
	Items       []Item
	MarkerColor color.RGBA
	ActiveColor color.RGBA
	HoverColor  color.RGBA
}

type Preview struct {
	Title   string
	Excerpt string
	Level   int
}

// state belongs to one rendered editor surface. Its slices are reused across
// frames; it stores no source model or editor pointer.
type state struct {
	Hover    int
	Active   int
	Revision uint64
	Rect     ui.Rect
	Ys       []float32
	Pitch    float32
	Scales   []float32
	CardY    float32
	CardOn   bool
	Anim     bool
}

func newState() state { return state{Hover: -1, Active: -1} }

func (s *state) Reset() {
	if s == nil {
		return
	}
	*s = newState()
}

// Draw paints the rail and reports heading jumps through the supplied host
// action. Preview data is requested only for the currently hovered heading.
func (s *state) Draw(ctx *ui.Context, bounds ui.Rect, frame Frame, preview func(int) Preview, jump func(uint64, int)) {
	if s == nil || ctx == nil {
		return
	}
	if !frame.Visible || len(frame.Items) < 2 {
		s.Hover, s.Active = -1, -1
		s.Rect = ui.Rect{}
		s.Ys, s.Scales = nil, nil
		s.CardOn = false
		return
	}
	if s.Revision != frame.Revision {
		s.Hover = -1
		s.CardOn = false
		s.Revision = frame.Revision
	}
	s.Anim = false
	s.Active = frame.Active
	avail := bounds.H - 2*Inset
	s.Ys = Positions(s.Ys, len(frame.Items), avail, &s.Pitch)
	if len(s.Scales) != len(frame.Items) {
		s.Scales = make([]float32, len(frame.Items))
		for i := range s.Scales {
			s.Scales[i] = 1
		}
	}
	hi := s.Hover
	if hi < 0 {
		hi = s.Active
	}
	for i := range s.Scales {
		distance := 1 << 20
		if hi >= 0 {
			distance = max(i-hi, hi-i)
		}
		Step(&s.Scales[i], Scale(distance), &s.Anim)
	}
	strip := ui.Rect{X: bounds.X + bounds.W - Width - Inset, Y: bounds.Y + Inset, W: Width, H: avail}
	s.Rect = strip
	ui.Overlay(ctx, func() {
		rail := ui.Box(ctx).Absolute().Left(strip.X).Top(strip.Y).Width(strip.W).Height(strip.H).
			Label(locales.EditorHeadingNavigation()).Cursor(ui.CursorPointer).Draw(func(p *ui.Painter, r ui.Rect) {
			DrawTicks(p, r, s.Ys, s.Pitch, s.Scales, s.Active, s.Hover, frame)
		})
		_, pointerY, over := rail.PointerPosition()
		hover := -1
		if over {
			hover = IndexAt(s.Ys, s.Pitch, pointerY)
		}
		if hover != s.Hover {
			s.Hover = hover
			if hover < 0 {
				s.CardOn = false
			}
			ctx.Invalidate()
		}
		if s.Hover >= 0 && s.Hover < len(frame.Items) && rail.Clicked() && jump != nil {
			jump(frame.Revision, frame.Items[s.Hover].Offset)
		}
		if s.Hover >= 0 && s.Hover < len(s.Ys) {
			center := strip.Y + s.Ys[s.Hover]
			if !s.CardOn {
				s.CardOn = true
				s.CardY = strip.Y + strip.H/2
			}
			Step(&s.CardY, center, &s.Anim)
			DrawPreview(ctx, bounds, strip, s.CardY, s.Hover, preview)
		}
	})
	if s.Anim {
		ctx.After(16 * time.Millisecond)
	} else if s.Hover >= 0 {
		ctx.After(80 * time.Millisecond)
	}
}

// Snapshot exposes a detached view of the current rail presentation for host
// diagnostics and interaction tests. Mutable animation state remains private.
type Snapshot struct {
	Hover, Active int
	Rect          ui.Rect
	Ys            []float32
	Pitch         float32
}

// Handle keeps pointer, animation and geometry state private to this package.
type Handle struct{ state *state }

func NewHandle() *Handle {
	state := newState()
	return &Handle{state: &state}
}

func (h *Handle) Reset() {
	if h != nil && h.state != nil {
		h.state.Reset()
	}
}

func (h *Handle) Draw(ctx *ui.Context, bounds ui.Rect, frame Frame, preview func(int) Preview, jump func(uint64, int)) {
	if h != nil && h.state != nil {
		h.state.Draw(ctx, bounds, frame, preview, jump)
	}
}

func (h *Handle) Snapshot() Snapshot {
	if h == nil || h.state == nil {
		return Snapshot{Hover: -1, Active: -1}
	}
	return Snapshot{
		Hover: h.state.Hover, Active: h.state.Active, Rect: h.state.Rect,
		Ys: append([]float32(nil), h.state.Ys...), Pitch: h.state.Pitch,
	}
}

func Scale(distance int) float32 {
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

func Positions(dst []float32, count int, height float32, pitch *float32) []float32 {
	dst = dst[:0]
	if count == 0 || height <= 0 {
		*pitch = 0
		return dst
	}
	*pitch = min(ItemHeight, height/float32(count))
	top := (height - *pitch*float32(count)) / 2
	for i := 0; i < count; i++ {
		dst = append(dst, top+(float32(i)+0.5)*(*pitch))
	}
	return dst
}

func IndexAt(ys []float32, pitch, y float32) int {
	if len(ys) == 0 || pitch <= 0 {
		return -1
	}
	top := ys[0] - pitch/2
	if y < top-2 || y > top+pitch*float32(len(ys))+2 {
		return -1
	}
	return min(max(int((y-top)/pitch), 0), len(ys)-1)
}

func Step(value *float32, target float32, anim *bool) {
	delta := target - *value
	if delta > -0.01 && delta < 0.01 {
		*value = target
		return
	}
	*value += delta * 0.22
	*anim = true
}

func DrawTicks(p *ui.Painter, r ui.Rect, ys []float32, pitch float32, scales []float32, active, hover int, frame Frame) {
	n := min(len(ys), len(scales))
	dense := pitch < TickHeight
	for i := 0; i < n; {
		width, col := tickStyle(i, scales, active, hover, frame)
		end := i + 1
		if dense {
			end = runEnd(ys[:n], scales[:n], i, active, hover)
		}
		top := r.Y + ys[i] - TickHeight/2
		bottom := r.Y + ys[end-1] + TickHeight/2
		p.Fill(ui.Rect{X: r.X + r.W - width, Y: top, W: width, H: bottom - top}, col, TickHeight/2)
		i = end
	}
}

func DrawPreview(ctx *ui.Context, bounds, strip ui.Rect, center float32, index int, preview func(int) Preview) {
	if preview == nil {
		return
	}
	item := preview(index)
	if item.Title == "" {
		return
	}
	theme := ctx.Theme()
	right := strip.X - 8
	left := max(right-min(CardWidth, bounds.W-16), bounds.X+4)
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

func tickStyle(i int, scales []float32, active, hover int, frame Frame) (float32, ui.Color) {
	col := toUI(frame.MarkerColor)
	switch {
	case i == active:
		col = toUI(frame.ActiveColor)
	case i == hover:
		col = toUI(frame.HoverColor)
	}
	return TickWidth * scales[i], col
}

func toUI(value color.RGBA) ui.Color { return ui.Color{R: value.R, G: value.G, B: value.B, A: value.A} }

func runEnd(ys, scales []float32, start, active, hover int) int {
	if start < 0 || start >= len(ys) || start >= len(scales) {
		return start
	}
	w := TickWidth * scales[start]
	style := styleKind(start, active, hover)
	bottom := ys[start] + TickHeight/2
	end := start + 1
	for end < len(ys) && end < len(scales) {
		if TickWidth*scales[end] != w || styleKind(end, active, hover) != style || ys[end]-TickHeight/2 > bottom {
			break
		}
		bottom = max(bottom, ys[end]+TickHeight/2)
		end++
	}
	return end
}

func styleKind(i, active, hover int) uint8 {
	switch {
	case i == active:
		return 1
	case i == hover:
		return 2
	default:
		return 0
	}
}
