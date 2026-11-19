package workbench

import (
	"testing"

	"muse/nativeui/shell"
)

func TestDispatchRoutesShellActionsToFeatureComposition(t *testing.T) {
	var selected shell.Surface
	var toggled bool
	var geometryWidth float32
	host := NewHost(Composition{Actions: ActionPorts{
		SelectSurface: func(surface shell.Surface) { selected = surface },
		TogglePanel:   func(open bool, _ float32, _ shell.Surface) { toggled = open },
		Geometry:      func(_ bool, width float32, _ shell.Surface) { geometryWidth = width },
	}})
	host.dispatch(shell.Action{Kind: shell.SelectSurface, Surface: shell.SurfaceOutline})
	host.dispatch(shell.Action{Kind: shell.TogglePanel, PanelOpen: true})
	host.dispatch(shell.Action{Kind: shell.PanelGeometryChanged, PanelWidth: 312})
	if selected != shell.SurfaceOutline || !toggled || geometryWidth != 312 {
		t.Fatalf("routed actions = surface %v, toggled %v, width %v", selected, toggled, geometryWidth)
	}
}
