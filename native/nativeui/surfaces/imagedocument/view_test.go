package imagedocument

import "testing"

func TestActionsUpdateImageDocumentViewState(t *testing.T) {
	state := state{path: "diagram.png", open: true, zoom: 1}
	state.Apply(Action{Kind: ZoomBy, Scale: 1.5})
	state.Apply(Action{Kind: PanBy, DX: 20, DY: -10})
	state.Apply(Action{Kind: BeginDrag, PointerX: 5, PointerY: 8})
	vm := state.ViewModel(2)
	if vm.Zoom != 1.5 || vm.X != 20 || vm.Y != -10 || !vm.Dragging || vm.BacklinkCount != 2 {
		t.Fatalf("image document state = %+v", vm)
	}
	state.Apply(Action{Kind: ResetView})
	vm = state.ViewModel(0)
	if vm.Zoom != 1 || vm.X != 0 || vm.Y != 0 {
		t.Fatalf("reset image document state = %+v", vm)
	}
	state.Apply(Action{Kind: EndDrag})
	if state.ViewModel(0).Dragging {
		t.Fatal("EndDrag left pointer gesture active")
	}
}

func TestCloseClearsImageDocument(t *testing.T) {
	state := state{path: "diagram.png", open: true, zoom: 1}
	state.Close()
	if state.IsOpen() || state.Path() != "" {
		t.Fatalf("close left image document state: %+v", state)
	}
}
