package shell

import (
	"testing"

)

func TestDispatchRoutesShellActionsToFeatureComposition(t *testing.T) {
	var selected Surface
	var toggled bool
	var geometryWidth float32
	host := NewWorkbenchHost(Composition{Actions: ActionPorts{
		SelectSurface: func(surface Surface) { selected = surface },
		TogglePanel:   func(open bool, _ float32, _ Surface) { toggled = open },
		Geometry:      func(_ bool, width float32, _ Surface) { geometryWidth = width },
	}})
	host.dispatch(Action{Kind: SelectSurface, Surface: SurfaceOutline})
	host.dispatch(Action{Kind: TogglePanel, PanelOpen: true})
	host.dispatch(Action{Kind: PanelGeometryChanged, PanelWidth: 312})
	if selected != SurfaceOutline || !toggled || geometryWidth != 312 {
		t.Fatalf("routed actions = surface %v, toggled %v, width %v", selected, toggled, geometryWidth)
	}
}
