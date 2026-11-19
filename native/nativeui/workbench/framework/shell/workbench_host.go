// Package shell composes the workbench frame with feature renderers.
package shell

import (
	"github.com/egoist/mygo/ui"
)

type SurfaceRenderer func(*ui.Context)

type ActionPorts struct {
	SelectSurface func(Surface)
	TogglePanel   func(open bool, width float32, surface Surface)
	Geometry      func(open bool, width float32, surface Surface)
	OpenSettings  func()
}

// Composition is the full view boundary between workbench and application.
// The host chooses the renderer for the selected shell surface, places the
// shared editor canvas and overlays, and translates shell actions into typed
// application actions.
type Composition struct {
	Frame           func() Frame
	Surfaces        map[Surface]SurfaceRenderer
	RenderCanvas    func(*ui.Context, bool)
	RenderInspector func(*ui.Context)
	RenderOverlays  func(*ui.Context)
	PanelWidth      func(float32)
	ObserveInput    func(ui.InputEvent)
	Actions         ActionPorts
}

type WorkbenchHost struct {
	composition Composition
	layout      *Layout
}

func NewWorkbenchHost(composition Composition) *WorkbenchHost {
	host := &WorkbenchHost{composition: composition}
	host.layout = NewLayout(LayoutPorts{
		Frame: composition.Frame,
		RenderPanel: func(c *ui.Context, surface Surface) {
			if render := composition.Surfaces[surface]; render != nil {
				render(c)
			}
		},
		RenderCanvas:    composition.RenderCanvas,
		RenderInspector: composition.RenderInspector,
		RenderOverlays:  composition.RenderOverlays,
		PanelWidth:      composition.PanelWidth,
		ObserveInput:    composition.ObserveInput,
		Dispatch:        host.dispatch,
	})
	return host
}

func (h *WorkbenchHost) Render(c *ui.Context) {
	if h != nil && h.layout != nil {
		h.layout.Render(c)
	}
}

func (h *WorkbenchHost) dispatch(action Action) {
	if h == nil {
		return
	}
	switch action.Kind {
	case SelectSurface:
		if h.composition.Actions.SelectSurface != nil {
			h.composition.Actions.SelectSurface(action.Surface)
		}
	case TogglePanel:
		if h.composition.Actions.TogglePanel != nil {
			h.composition.Actions.TogglePanel(action.PanelOpen, action.PanelWidth, action.Surface)
		}
	case PanelGeometryChanged:
		if h.composition.Actions.Geometry != nil {
			h.composition.Actions.Geometry(action.PanelOpen, action.PanelWidth, action.Surface)
		}
	case OpenSettings:
		if h.composition.Actions.OpenSettings != nil {
			h.composition.Actions.OpenSettings()
		}
	}
}
