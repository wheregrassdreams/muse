package imagepreview

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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

func TestContentCloseIsIdempotent(t *testing.T) {
	closed := 0
	content := NewContent("image.png", "image.png", nil, nil, nil, nil, func() { closed++ })
	content.state = state{source: "image.png", open: true, zoom: 1}
	content.Close()
	content.Close()
	if closed != 1 || !content.closed || content.state.open || content.state.bmp != nil {
		t.Fatalf("content close count=%d closed=%v state=%+v", closed, content.closed, content.state.ViewModel())
	}
}

func TestOpenMissingLocalImageStopsLoading(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.png")
	var state state
	state.Open(path, "missing.png", nil, nil, nil)
	if state.asset == nil || !state.asset.Failed() {
		t.Fatal("missing local image was not reported as failed by the asset")
	}
	if state.ViewModel().Loading {
		t.Fatal("missing local image remained in loading state")
	}
}

func TestOpenDecodeFailureStopsLoading(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.png")
	if err := os.WriteFile(path, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}

	mainQueue := make(chan func(), 1)
	var state state
	state.Open(path, "invalid.png", nil, func(complete func()) { mainQueue <- complete }, nil)

	select {
	case complete := <-mainQueue:
		complete()
	case <-time.After(5 * time.Second):
		t.Fatal("decode failure did not complete image loading")
	}
	if !state.asset.Loaded() || !state.asset.Failed() {
		t.Fatalf("invalid image asset loaded=%v failed=%v, want completed failure", state.asset.Loaded(), state.asset.Failed())
	}
	if state.ViewModel().Loading {
		t.Fatal("decode failure left preview in loading state")
	}
}
