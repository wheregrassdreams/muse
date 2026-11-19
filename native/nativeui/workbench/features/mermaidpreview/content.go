package mermaidpreview

import (
	"github.com/egoist/mygo/ui"
	"github.com/wheregrassdreams/muse-editor/markdown/diagrams"
	markdownui "muse/nativeui/workbench/framework/markdown"
	"muse/nativeui/workbench/framework/components/viewport"
	hosting "muse/nativeui/workbench/framework/hosting"
)

type Content struct {
	source   string
	model    diagrams.Result
	diagram  markdownui.Diagram
	viewport viewport.Transform
	controls hosting.Controls
	onClose  func()
	closed   bool
}

// NewContent snapshots the neutral diagram model. The editor remains the
// owner of source and selection; this surface only changes its viewport.
func NewContent(source string, model diagrams.Result, onClose func()) *Content {
	model.Labels = append([]diagrams.Label(nil), model.Labels...)
	return &Content{source: source, model: model, diagram: markdownui.DiagramFromResult(model), viewport: viewport.NewTransform(), onClose: onClose}
}

func (s *Content) Bind(controls hosting.Controls) { s.controls = controls }

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
				s.viewport.PanBy(-ev.DX, -ev.DY)
			} else if ev.DY != 0 {
				s.viewport.ZoomBy(viewport.ScrollZoom(ev.DY))
			}
			return true
		case ui.InputZoom:
			s.viewport.ZoomBy(ev.Scale)
			return true
		case ui.InputPointerDown:
			if ev.Button == 0 {
				s.viewport.BeginDrag(ev.X, ev.Y)
				return true
			}
		case ui.InputPointerMove:
			if ev.Button == 0 {
				if _, _, ok := s.viewport.DragTo(ev.X, ev.Y); ok {
					return true
				}
			}
		case ui.InputPointerUp:
			if s.viewport.Dragging {
				s.viewport.EndDrag()
				return true
			}
		}
		return false
	})
	root.Children(func() {
		width, height := s.model.Width*s.viewport.Zoom, s.model.Height*s.viewport.Zoom
		ui.Box(c).Key("mermaid-preview-canvas").Absolute().Top(48).Left(0).Right(0).Bottom(0).
			Draw(func(p *ui.Painter, r ui.Rect) {
				canvas := ui.Rect{X: r.X + (r.W-width)/2 + s.viewport.X, Y: r.Y + (r.H-height)/2 + s.viewport.Y, W: width, H: height}
				s.diagram.Draw(p, canvas, t)
			})
		ui.Row(c).Key("mermaid-preview-toolbar").Absolute().Top(10).Left(12).Right(12).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "Mermaid preview").TextColor(t.Text).Grow(1)
			ui.Text(c, "−").TextColor(t.Text).OnClick(func() { s.viewport.ZoomBy(1 / 1.2) })
			ui.Text(c, "Zoom").TextColor(t.TextMuted)
			ui.Text(c, "+").TextColor(t.Text).OnClick(func() { s.viewport.ZoomBy(1.2) })
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

var _ hosting.Content = (*Content)(nil)
var _ hosting.Bindable = (*Content)(nil)
var _ hosting.FocusLifecycle = (*Content)(nil)
