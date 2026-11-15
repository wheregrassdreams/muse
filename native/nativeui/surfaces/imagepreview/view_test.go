package imagepreview

import "testing"

func TestTypedActionsUpdatePreviewState(t *testing.T) {
	state := state{open: true, zoom: 1, baseZoom: 0.5}
	state.Apply(Action{Kind: ZoomBy, Scale: 1.25})
	state.Apply(Action{Kind: PanBy, DX: 12, DY: -8})
	state.Apply(Action{Kind: BeginDrag, PointerX: 20, PointerY: 30})
	vm := state.ViewModel()
	if vm.Zoom != 1.25 || vm.X != 12 || vm.Y != -8 || !vm.Dragging || vm.DragX != 20 || vm.DragY != 30 {
		t.Fatalf("action state = %+v", vm)
	}
	state.Apply(Action{Kind: SetBaseZoom, Scale: 0.75})
	state.Apply(Action{Kind: ResetView})
	if vm = state.ViewModel(); vm.Zoom != 1 || vm.X != 0 || vm.Y != 0 || vm.BaseZoom != 0.75 {
		t.Fatalf("reset state = %+v", vm)
	}
	state.Apply(Action{Kind: EndDrag})
	if state.ViewModel().Dragging {
		t.Fatal("EndDrag left dragging enabled")
	}
}

func TestCloseClearsImageReferences(t *testing.T) {
	state := state{source: "image.png", open: true, loading: true, zoom: 2}
	state.Close()
	vm := state.ViewModel()
	if vm.Open || vm.Loading || vm.Source != "image.png" || state.asset != nil || state.bmp != nil {
		t.Fatalf("closed preview state = %+v", vm)
	}
}
