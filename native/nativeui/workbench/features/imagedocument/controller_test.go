package imagedocument

import (
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"muse/markdown/images"
	hosting "muse/nativeui/workbench/framework/hosting"
	"muse/platform/filesystem/imagefiles"
)

type fakeHost struct {
	manager    *hosting.Manager
	content    hosting.Content
	placements []hosting.Placement
	openErr    error
}

func newFakeHost() *fakeHost { return &fakeHost{manager: hosting.NewManager(nil)} }

func (h *fakeHost) Open(content hosting.Content, placement hosting.Placement) (hosting.Handle, error) {
	h.placements = append(h.placements, placement)
	if h.openErr != nil {
		return hosting.Handle{}, h.openErr
	}
	h.content = content
	return h.manager.Open(content, placement)
}
func (h *fakeHost) Close(handle hosting.Handle) error   { return h.manager.Close(handle) }
func (h *fakeHost) Contains(handle hosting.Handle) bool { return h.manager.Contains(handle) }

func TestControllerOpensLocalImageThroughTypedPorts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 24, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 24; x++ {
			img.Set(x, y, color.RGBA{R: 30, G: 140, B: 210, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, img, nil); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	host := newFakeHost()
	placement := hosting.Placement{Host: "main", Pane: "editor-1", Layer: hosting.PaneContent}
	selected, recorded := "", ""
	controller := NewController(Ports{
		Host:            host,
		Placement:       func() hosting.Placement { return placement },
		Files:           func() images.LocalFiles { return imagefiles.New() },
		OnSelectedPath:  func(path string) { selected = path },
		OnRecordHistory: func(path string) { recorded = path },
	})
	if err := controller.Open(path, true); err != nil {
		t.Fatal(err)
	}
	content, ok := host.content.(*Content)
	if !ok || content.IsClosed() || !content.state.HasImage() {
		t.Fatalf("hosted content = %T; closed=%v image=%v", host.content, !ok || content.IsClosed(), ok && content.state.HasImage())
	}
	if got := host.placements[len(host.placements)-1]; got != placement {
		t.Fatalf("placement = %+v, want %+v", got, placement)
	}
	if controller.Path() != path || selected != path || recorded != path {
		t.Fatalf("open path=%q selected=%q recorded=%q", controller.Path(), selected, recorded)
	}
}

func TestControllerOpenReplacesImageAndRecordsSelection(t *testing.T) {
	host := newFakeHost()
	placement := hosting.Placement{Host: "main", Pane: "right", Layer: hosting.PaneContent}
	selected, recorded, changed := "", "", 0
	decodeErrors := []string{}
	controller := NewController(Ports{
		Host:            host,
		Placement:       func() hosting.Placement { return placement },
		OnSelectedPath:  func(path string) { selected = path },
		OnRecordHistory: func(path string) { recorded = path },
		OnDecodeError:   func(path string) { decodeErrors = append(decodeErrors, path) },
		OnChanged:       func() { changed++ },
	})

	if err := controller.Open("first.png", true); err != nil {
		t.Fatal(err)
	}
	first := controller.Handle()
	if first.IsZero() || controller.Path() != "first.png" || selected != "first.png" || recorded != "first.png" {
		t.Fatalf("first open state: handle=%v path=%q selected=%q recorded=%q", first, controller.Path(), selected, recorded)
	}
	if got := host.placements[0]; got != placement {
		t.Fatalf("placement = %+v, want %+v", got, placement)
	}

	if err := controller.Open("second.png", false); err != nil {
		t.Fatal(err)
	}
	if host.Contains(first) || controller.Path() != "second.png" || selected != "second.png" {
		t.Fatalf("replacement state: oldOpen=%v path=%q selected=%q", host.Contains(first), controller.Path(), selected)
	}
	if recorded != "first.png" {
		t.Fatalf("record callback ran for record=false: %q", recorded)
	}
	if len(decodeErrors) != 2 || decodeErrors[0] != "first.png" || decodeErrors[1] != "second.png" {
		t.Fatalf("decode errors = %v", decodeErrors)
	}
	if changed < 2 {
		t.Fatalf("changed callback count = %d, want at least 2", changed)
	}

	second := controller.Handle()
	if err := host.Close(second); err != nil {
		t.Fatal(err)
	}
	if controller.IsOpen() || controller.Path() != "" || !controller.Handle().IsZero() {
		t.Fatalf("external close left controller open: path=%q", controller.Path())
	}
}

func TestControllerCloseIsIdempotent(t *testing.T) {
	host := newFakeHost()
	controller := NewController(Ports{Host: host, Placement: func() hosting.Placement {
		return hosting.Placement{Host: "main", Pane: "left", Layer: hosting.PaneContent}
	}})
	if err := controller.Open("image.png", false); err != nil {
		t.Fatal(err)
	}
	controller.Close()
	controller.Close()
	if controller.IsOpen() || controller.Path() != "" {
		t.Fatalf("close did not clear controller: path=%q", controller.Path())
	}
}

func TestControllerReportsHostErrors(t *testing.T) {
	host := newFakeHost()
	host.openErr = errors.New("host failed")
	var noticed error
	controller := NewController(Ports{
		Host: host,
		Placement: func() hosting.Placement {
			return hosting.Placement{Host: "main", Pane: "left", Layer: hosting.PaneContent}
		},
		OnNotice: func(err error) { noticed = err },
	})
	if err := controller.Open("image.png", true); !errors.Is(err, host.openErr) {
		t.Fatalf("Open error = %v, want %v", err, host.openErr)
	}
	if !errors.Is(noticed, host.openErr) || controller.IsOpen() {
		t.Fatalf("notice=%v open=%v", noticed, controller.IsOpen())
	}
}

func TestControllerIgnoresEmptyPath(t *testing.T) {
	host := newFakeHost()
	controller := NewController(Ports{Host: host, Placement: func() hosting.Placement {
		return hosting.Placement{Host: "main", Pane: "left", Layer: hosting.PaneContent}
	}})
	if err := controller.Open("image.png", false); err != nil {
		t.Fatal(err)
	}
	current := controller.Handle()
	if err := controller.Open("", true); err != nil {
		t.Fatal(err)
	}
	if controller.Handle() != current {
		t.Fatal("empty path replaced current image")
	}
}
