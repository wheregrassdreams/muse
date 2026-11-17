package mermaidpreview

import (
	"math"

	"github.com/egoist/mygo/ui"
	"muse/markdown/diagrams"
	markdownui "muse/nativeui/workbench/features/markdown"
	"muse/nativeui/workbench/hosting"
)

type Content struct {
	source   string
	model    diagrams.Result
	diagram  markdownui.Diagram
	zoom     float32
	x, y     float32
	dragging bool
	dragX    float32
	dragY    float32
	controls window.Controls
	onClose  func()
	closed   bool
}

// NewContent snapshots the neutral diagram model. The editor remains the
// owner of source and selection; this surface only changes its viewport.
func NewContent(source string, model diagrams.Result, onClose func()) *Content {
	model.Labels = append([]diagrams.Label(nil), model.Labels...)
	return &Content{source: source, model: model, diagram: markdownui.DiagramFromResult(model), zoom: 1, onClose: onClose}
}

func (s *Content) Bind(controls window.Controls) { s.controls = controls }

func (s *Content) Render(c *ui.Context) {
	if s == nil || s.closed {
		return
	}
	t := c.Theme()
	root := ui.Box(c).Key("mermaid-preview").Absolute().Top(0).Left(0).Right(0).Bottom(0).
		Background(ui.RGBA(0, 0, 0, 0.88)).Focusable().Focus().HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputKeyDown:
			if ev.Key == ui.KeyEscape && s.controls != nil {
				s.controls.CloseSelf()
			}
			return true
		case ui.InputText, ui.InputCompose, ui.InputCommand, ui.InputKeyUp:
			return true
		case ui.InputScroll:
			if ev.Precise {
				s.x -= ev.DX
				s.y -= ev.DY
			} else if ev.DY != 0 {
				s.zoom = clamp(s.zoom * zoomFactor(ev.DY))
			}
			return true
		case ui.InputZoom:
			if ev.Scale > 0 {
				s.zoom = clamp(s.zoom * ev.Scale)
			}
			return true
		case ui.InputPointerDown:
			if ev.Button == 0 {
				s.dragging, s.dragX, s.dragY = true, ev.X, ev.Y
				return true
			}
		case ui.InputPointerMove:
			if s.dragging && ev.Button == 0 {
				s.x += ev.X - s.dragX
				s.y += ev.Y - s.dragY
				s.dragX, s.dragY = ev.X, ev.Y
				return true
			}
		case ui.InputPointerUp:
			if s.dragging {
				s.dragging = false
				return true
			}
		}
		return false
	})
	root.Children(func() {
		width, height := s.model.Width*s.zoom, s.model.Height*s.zoom
		ui.Box(c).Key("mermaid-preview-canvas").Absolute().Top(48).Left(0).Right(0).Bottom(0).
			Draw(func(p *ui.Painter, r ui.Rect) {
				viewport := ui.Rect{X: r.X + (r.W-width)/2 + s.x, Y: r.Y + (r.H-height)/2 + s.y, W: width, H: height}
				s.diagram.Draw(p, viewport, t)
			})
		ui.Row(c).Key("mermaid-preview-toolbar").Absolute().Top(10).Left(12).Right(12).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "Mermaid preview").TextColor(t.Text).Grow(1)
			ui.Text(c, "−").TextColor(t.Text).OnClick(func() { s.zoom = clamp(s.zoom / 1.2) })
			ui.Text(c, "Zoom").TextColor(t.TextMuted)
			ui.Text(c, "+").TextColor(t.Text).OnClick(func() { s.zoom = clamp(s.zoom * 1.2) })
			ui.Button(c, "×").Label("Close preview").OnClick(func() {
				if s.controls != nil {
					s.controls.CloseSelf()
				}
			})
		})
	})
}

func (s *Content) Close() {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	s.model = diagrams.Result{}
	s.diagram = markdownui.Diagram{}
	if s.onClose != nil {
		s.onClose()
	}
}

func (s *Content) Focused() {}
func (s *Content) Blurred() {}

func clamp(v float32) float32 { return min(float32(8), max(float32(0.1), v)) }

func zoomFactor(delta float32) float32 { return float32(math.Pow(1.12, float64(-delta/60))) }

var _ window.Content = (*Content)(nil)
var _ window.Bindable = (*Content)(nil)
var _ window.FocusLifecycle = (*Content)(nil)
