// Package workbench owns the composition of the native shell and feature
// surfaces. Feature state stays in its feature package; this host owns which
// surface is mounted and routes shell intents to the application boundary.
package workbench

import (
	"github.com/egoist/mygo/ui"
	"muse/nativeui/shell"
)

type SurfaceRenderer func(*ui.Context)

type ActionPorts struct {
	SelectSurface func(shell.Surface)
	TogglePanel   func(open bool, width float32, surface shell.Surface)
	Geometry      func(open bool, width float32, surface shell.Surface)
	OpenSettings  func()
}

// Composition is the full view boundary between workbench and application.
// The host chooses the renderer for the selected shell surface, places the
// shared editor canvas and overlays, and translates shell actions into typed
// application actions.
type Composition struct {
	Frame           func() shell.Frame
	Surfaces        map[shell.Surface]SurfaceRenderer
	RenderCanvas    func(*ui.Context, bool)
	RenderInspector func(*ui.Context)
	RenderOverlays  func(*ui.Context)
	PanelWidth      func(float32)
	ObserveInput    func(ui.InputEvent)
	Actions         ActionPorts
}

type Host struct {
	composition Composition
	layout      *shell.Layout
}

func NewHost(composition Composition) *Host {
	host := &Host{composition: composition}
	host.layout = shell.NewLayout(shell.LayoutPorts{
		Frame: composition.Frame,
		RenderPanel: func(c *ui.Context, surface shell.Surface) {
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

func (h *Host) Render(c *ui.Context) {
	if h != nil && h.layout != nil {
		h.layout.Render(c)
	}
}

func (h *Host) dispatch(action shell.Action) {
	if h == nil {
		return
	}
	switch action.Kind {
	case shell.SelectSurface:
		if h.composition.Actions.SelectSurface != nil {
			h.composition.Actions.SelectSurface(action.Surface)
		}
	case shell.TogglePanel:
		if h.composition.Actions.TogglePanel != nil {
			h.composition.Actions.TogglePanel(action.PanelOpen, action.PanelWidth, action.Surface)
		}
	case shell.PanelGeometryChanged:
		if h.composition.Actions.Geometry != nil {
			h.composition.Actions.Geometry(action.PanelOpen, action.PanelWidth, action.Surface)
		}
	case shell.OpenSettings:
		if h.composition.Actions.OpenSettings != nil {
			h.composition.Actions.OpenSettings()
		}
	}
}
