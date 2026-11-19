package imagedocument

import (
	"testing"

	"muse/nativeui/ui/viewport"
)

func TestCloseClearsImageDocument(t *testing.T) {
	state := state{path: "diagram.png", open: true, transform: viewport.NewTransform()}
	state.Close()
	if state.IsOpen() || state.Path() != "" {
		t.Fatalf("close left image document state: %+v", state)
	}
}

func TestContentCloseIsIdempotent(t *testing.T) {
	closed := 0
	content := &Content{state: state{path: "image.png", open: true}, options: SurfaceOptions{OnClose: func() { closed++ }}}
	content.Close()
	content.Close()
	if closed != 1 || !content.closed || content.state.IsOpen() || content.state.Path() != "" {
		t.Fatalf("content close count=%d closed=%v state=%+v", closed, content.closed, content.state)
	}
}
