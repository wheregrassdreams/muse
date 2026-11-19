package imagepreview

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"muse/locales"
	"muse/markdown/images"
	markdownui "muse/nativeui/markdown"
	workspaceui "muse/nativeui/workspace"
)

type missingImageFiles struct{}

func (missingImageFiles) Stat(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist }
func (missingImageFiles) ReadFile(string) ([]byte, error)  { return nil, fs.ErrNotExist }
func (missingImageFiles) EvalSymlinks(path string) (string, error) {
	return path, nil
}

type previewCache struct{}

func (previewCache) Read(string) ([]byte, error) { return nil, fs.ErrNotExist }
func (previewCache) Write(string, []byte) error  { return nil }
func (previewCache) Delete(string) error         { return nil }

type hostedContentStub struct{}

func (hostedContentStub) Render(*ui.Context) {}
func (hostedContentStub) Close()             {}

func controllerTestOptions(manager *workspaceui.Manager) ControllerOptions {
	return ControllerOptions{
		Windows: manager,
		Placement: workspaceui.Placement{
			Host: "main", Layer: workspaceui.ModalOverlay,
		},
		ImageCache: previewCache{},
		LocalFiles: missingImageFiles{},
		Label: func(source string) string {
			return "test: " + filepath.Base(source)
		},
		OnMain: func(callback func()) { callback() },
	}
}

func TestControllerHostsReplacesAndClosesPreview(t *testing.T) {
	manager := workspaceui.NewManager(nil)
	invalidations := 0
	options := controllerTestOptions(manager)
	options.Invalidate = func() { invalidations++ }
	controller := NewController(options)

	if err := controller.Open("/vault/first.png"); err != nil {
		t.Fatal(err)
	}
	first := controller.Handle()
	if first.IsZero() || !controller.IsOpen() || !manager.HasModal() {
		t.Fatalf("first preview state handle=%v open=%v modal=%v", first, controller.IsOpen(), manager.HasModal())
	}
	var firstContent *Content
	if err := manager.WithContent(first, func(content workspaceui.Content) error {
		var ok bool
		firstContent, ok = content.(*Content)
		if !ok {
			t.Fatalf("hosted content type = %T, want *Content", content)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := controller.Open("/vault/second.png"); err != nil {
		t.Fatal(err)
	}
	if !firstContent.IsClosed() || !controller.IsOpen() || controller.Handle() == first {
		t.Fatalf("replacement lifecycle closed=%v open=%v handle=%v", firstContent.IsClosed(), controller.IsOpen(), controller.Handle())
	}
	controller.Close()
	controller.Close()
	if controller.IsOpen() || manager.HasModal() || !controller.Handle().IsZero() {
		t.Fatal("Close did not clear the hosted preview")
	}
	if invalidations < 3 {
		t.Fatalf("invalidate count = %d, want open/replacement/close notifications", invalidations)
	}
}

func TestControllerRenderUsesInjectedLabelAndCopyCallback(t *testing.T) {
	manager := workspaceui.NewManager(nil)
	var copied string
	options := controllerTestOptions(manager)
	options.CopyImage = func(source string) { copied = source }
	controller := NewController(options)
	const source = "/vault/Attachments/diagram.png"
	if err := controller.Open(source); err != nil {
		t.Fatal(err)
	}
	tester := ui.NewTester(func(c *ui.Context) {
		manager.Render(c, workspaceui.HostID("main"), workspaceui.ModalOverlay)
	}, 900, 600)
	tester.Frame()
	if want := markdownui.MissingImageMessage("test: diagram.png"); !tester.HasText(want) {
		t.Fatalf("missing-image label = %q; expected %q", tester.Texts(), want)
	}
	tester.RightClickAt(450, 300)
	tester.Frame()
	if err := tester.ChooseMenuItem(locales.CommonCopyAction()); err != nil {
		t.Fatalf("choose copy image action: %v", err)
	}
	tester.Frame()
	if copied != source {
		t.Fatalf("copy callback source = %q, want %q", copied, source)
	}
	tester.Key(0, ui.KeyEscape)
	tester.Frame()
	if controller.IsOpen() {
		t.Fatal("Escape left the hosted preview open")
	}
}

func TestControllerReportsHostOpenFailure(t *testing.T) {
	manager := workspaceui.NewManager(nil)
	if _, err := manager.Open(hostedContentStub{}, workspaceui.Placement{Host: "main", Layer: workspaceui.ModalOverlay}); err != nil {
		t.Fatal(err)
	}
	var reported error
	options := controllerTestOptions(manager)
	options.OnError = func(err error) { reported = err }
	controller := NewController(options)
	if err := controller.Open("image.png"); !errors.Is(err, workspaceui.ErrModalAlreadyOpen) {
		t.Fatalf("Open error = %v, want modal conflict", err)
	}
	if !errors.Is(reported, workspaceui.ErrModalAlreadyOpen) || controller.IsOpen() {
		t.Fatalf("reported error=%v controller open=%v", reported, controller.IsOpen())
	}
}

func TestControllerIgnoresEmptySource(t *testing.T) {
	controller := NewController(controllerTestOptions(workspaceui.NewManager(nil)))
	if err := controller.Open(""); err != nil || controller.IsOpen() {
		t.Fatalf("Open empty source error=%v open=%v; want no-op", err, controller.IsOpen())
	}
}

var _ images.LocalFiles = missingImageFiles{}
var _ images.CacheStore = previewCache{}
