package imagepreview

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"muse/nativeui/ui/viewport"
	"muse/platform/filesystem/imagefiles"
)

func TestTypedActionsUpdatePreviewState(t *testing.T) {
	state := state{open: true, transform: viewport.NewTransform(), baseZoom: 0.5}
	state.Apply(Action{Kind: SetBaseZoom, Scale: 0.75})
	state.Apply(Action{Kind: ResetView})
	if vm := state.ViewModel(); vm.Zoom != 1 || vm.X != 0 || vm.Y != 0 || vm.BaseZoom != 0.75 {
		t.Fatalf("reset state = %+v", vm)
	}
}

func TestCloseClearsImageReferences(t *testing.T) {
	state := state{source: "image.png", open: true, loading: true, transform: viewport.NewTransform()}
	state.Close()
	vm := state.ViewModel()
	if vm.Open || vm.Loading || vm.Source != "image.png" || state.asset != nil || state.bmp != nil {
		t.Fatalf("closed preview state = %+v", vm)
	}
}

func TestContentCloseIsIdempotent(t *testing.T) {
	closed := 0
	content := NewContent("image.png", "image.png", nil, nil, nil, nil, func() { closed++ })
	content.state = state{source: "image.png", open: true, transform: viewport.NewTransform()}
	content.Close()
	content.Close()
	if closed != 1 || !content.closed || content.state.open || content.state.bmp != nil {
		t.Fatalf("content close count=%d closed=%v state=%+v", closed, content.closed, content.state.ViewModel())
	}
}

func TestOpenMissingLocalImageStopsLoading(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.png")
	var state state
	state.Open(path, "missing.png", nil, imagefiles.New(), nil, nil)
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
	state.Open(path, "invalid.png", nil, imagefiles.New(), func(complete func()) { mainQueue <- complete }, nil)

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
