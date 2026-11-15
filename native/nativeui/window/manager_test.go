package window

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

type testContent struct {
	rendered int
	closed   int
	focused  int
	blurred  int
	controls Controls
}

func (c *testContent) Render(*ui.Context)     { c.rendered++ }
func (c *testContent) Close()                 { c.closed++ }
func (c *testContent) Bind(controls Controls) { c.controls = controls }
func (c *testContent) Focused()               { c.focused++ }
func (c *testContent) Blurred()               { c.blurred++ }

func TestModalExclusivityEscapeAndFocusRestore(t *testing.T) {
	fallbacks := 0
	manager := NewManager(func() { fallbacks++ })
	main := &testContent{}
	mainHandle, err := manager.Open(main, Placement{Host: "main", Layer: PaneContent})
	if err != nil {
		t.Fatal(err)
	}
	modal := &testContent{}
	modalHandle, err := manager.Open(modal, Placement{Host: "main", Layer: ModalOverlay})
	if err != nil {
		t.Fatal(err)
	}
	if manager.Focus(mainHandle) {
		t.Fatal("focused a surface behind an active modal")
	}
	if _, err := manager.Open(&testContent{}, Placement{Host: "main", Layer: ModalOverlay}); err != ErrModalAlreadyOpen {
		t.Fatalf("second modal error = %v, want %v", err, ErrModalAlreadyOpen)
	}
	if !manager.HandleEscape() || manager.HasModal() {
		t.Fatal("Escape did not consume and close the modal")
	}
	if modal.closed != 1 || main.focused != 2 || main.blurred != 1 || fallbacks != 0 {
		t.Fatalf("lifecycle counts modal=%d main focus=%d blur=%d fallback=%d", modal.closed, main.focused, main.blurred, fallbacks)
	}
	if err := manager.Close(modalHandle); err != nil || modal.closed != 1 {
		t.Fatalf("closing an already closed handle repeated lifecycle: err=%v closed=%d", err, modal.closed)
	}
}

func TestFocusedContentBlursExactlyOnceOnCloseAndCloseAll(t *testing.T) {
	manager := NewManager(nil)
	content := &testContent{}
	handle, err := manager.Open(content, Placement{Host: "main", Layer: Overlay})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Close(handle); err != nil {
		t.Fatal(err)
	}
	_ = manager.Close(handle)
	if content.blurred != 1 || content.closed != 1 {
		t.Fatalf("close lifecycle blurred=%d closed=%d, want 1 each", content.blurred, content.closed)
	}

	closing := &testContent{}
	_, err = manager.Open(closing, Placement{Host: "main", Layer: Overlay})
	if err != nil {
		t.Fatal(err)
	}
	manager.CloseAll()
	if closing.blurred != 1 || closing.closed != 1 {
		t.Fatalf("close-all lifecycle blurred=%d closed=%d, want 1 each", closing.blurred, closing.closed)
	}
}

func TestPlaceIntoModalLayerMovesFocusAndEscapeRestoresPrevious(t *testing.T) {
	manager := NewManager(nil)
	main := &testContent{}
	mainHandle, err := manager.Open(main, Placement{Host: "main", Layer: PaneContent})
	if err != nil {
		t.Fatal(err)
	}
	content := &testContent{}
	handle, err := manager.Open(content, Placement{Host: "main", Layer: Overlay})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Place(handle, Placement{Host: "main", Layer: ModalOverlay}); err != nil {
		t.Fatal(err)
	}
	if content.focused != 1 || !manager.HasModal() {
		t.Fatalf("modal placement focus=%d modal=%v", content.focused, manager.HasModal())
	}
	if !manager.HandleEscape() || main.focused != 2 || content.blurred != 1 {
		t.Fatalf("escape focus restore: consumed modal=%v mainFocus=%d modalBlur=%d", manager.HasModal(), main.focused, content.blurred)
	}
	if !manager.Contains(mainHandle) {
		t.Fatal("main pane was unexpectedly removed")
	}
}

func TestFocusedPaneSurvivesOverlayAndRepairsAfterPaneContentCloses(t *testing.T) {
	manager := NewManager(nil)
	first := &testContent{}
	_, err := manager.Open(first, Placement{Host: "main", Pane: "editor", Layer: PaneContent})
	if err != nil {
		t.Fatal(err)
	}
	second := &testContent{}
	secondHandle, err := manager.Open(second, Placement{Host: "main", Pane: "preview", Layer: PaneContent})
	if err != nil {
		t.Fatal(err)
	}
	if pane, ok := manager.FocusedPane("main"); !ok || pane != "preview" {
		t.Fatalf("focused pane = %q, %v; want preview", pane, ok)
	}
	modal := &testContent{}
	_, err = manager.Open(modal, Placement{Host: "main", Layer: ModalOverlay})
	if err != nil {
		t.Fatal(err)
	}
	if pane, ok := manager.FocusedPane("main"); !ok || pane != "preview" {
		t.Fatalf("overlay changed focused pane context = %q, %v; want preview", pane, ok)
	}
	if !manager.HandleEscape() {
		t.Fatal("modal Escape was not consumed")
	}
	if err := manager.Close(secondHandle); err != nil {
		t.Fatal(err)
	}
	if pane, ok := manager.FocusedPane("main"); !ok || pane != "editor" {
		t.Fatalf("focused pane after closing preview = %q, %v; want editor", pane, ok)
	}
	manager.CloseAll()
	if pane, ok := manager.FocusedPane("main"); ok || pane != "" {
		t.Fatalf("focused pane after CloseAll = %q, %v; want empty", pane, ok)
	}
}

func TestPlacementRenderingAndControls(t *testing.T) {
	manager := NewManager(nil)
	first := &testContent{}
	second := &testContent{}
	firstHandle, err := manager.Open(first, Placement{Host: "main", Layer: Overlay})
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Open(second, Placement{Host: "main", Pane: "editor", Layer: PaneContent})
	if err != nil {
		t.Fatal(err)
	}
	tester := ui.NewTester(func(c *ui.Context) {
		manager.Render(c, "main", Overlay)
	}, 400, 300)
	tester.Frame()
	if first.rendered == 0 || second.rendered != 0 {
		t.Fatalf("placement render counts first=%d second=%d", first.rendered, second.rendered)
	}
	first.controls.CloseSelf()
	if first.closed != 1 || manager.Contains(firstHandle) {
		t.Fatalf("content self-close failed: close count=%d exists=%v", first.closed, manager.Contains(firstHandle))
	}
}

func TestCloseAllClosesEachContentAndRestoresFallbackOnce(t *testing.T) {
	fallbacks := 0
	manager := NewManager(func() { fallbacks++ })
	first, second := &testContent{}, &testContent{}
	_, _ = manager.Open(first, Placement{Host: "main", Layer: PaneContent})
	_, _ = manager.Open(second, Placement{Host: "main", Layer: Overlay})
	manager.CloseAll()
	manager.CloseAll()
	if first.closed != 1 || second.closed != 1 || fallbacks != 1 || manager.HasModal() {
		t.Fatalf("close all counts first=%d second=%d fallbacks=%d modal=%v", first.closed, second.closed, fallbacks, manager.HasModal())
	}
}

type reentrantContent struct {
	testContent
	onRender func()
}

func (c *reentrantContent) Render(*ui.Context) {
	c.rendered++
	if c.onRender != nil {
		fn := c.onRender
		c.onRender = nil
		fn()
	}
}

func TestRenderMayCloseSelfWithoutClosingDuringTraversal(t *testing.T) {
	manager := NewManager(nil)
	content := &reentrantContent{}
	handle, err := manager.Open(content, Placement{Host: "main", Layer: Overlay})
	if err != nil {
		t.Fatal(err)
	}
	content.onRender = func() {
		content.controls.CloseSelf()
		if content.closed != 0 {
			t.Fatal("Close ran inside Render")
		}
	}
	tester := ui.NewTester(func(c *ui.Context) { manager.Render(c, "main", Overlay) }, 400, 300)
	tester.Frame()
	if content.rendered != 1 || content.closed != 1 || manager.Contains(handle) {
		t.Fatalf("render=%d close=%d contains=%v", content.rendered, content.closed, manager.Contains(handle))
	}
	manager.CloseAll()
	if content.closed != 1 {
		t.Fatalf("self close invoked Close %d times", content.closed)
	}
}

func TestRenderMayOpenModalWithoutChangingCurrentSnapshot(t *testing.T) {
	manager := NewManager(nil)
	modal := &testContent{}
	first := &reentrantContent{}
	second := &testContent{}
	_, err := manager.Open(first, Placement{Host: "main", Layer: Overlay})
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Open(second, Placement{Host: "main", Layer: Overlay})
	if err != nil {
		t.Fatal(err)
	}
	first.onRender = func() {
		if _, openErr := manager.Open(modal, Placement{Host: "main", Layer: ModalOverlay}); openErr != nil {
			t.Fatalf("open modal during Render: %v", openErr)
		}
	}
	tester := ui.NewTester(func(c *ui.Context) { manager.Render(c, "main", Overlay) }, 400, 300)
	tester.Frame()
	if first.rendered == 0 || first.rendered != second.rendered {
		t.Fatalf("current snapshot changed: first=%d second=%d", first.rendered, second.rendered)
	}
	if modal.focused != 1 || !manager.HasModal() {
		t.Fatalf("deferred modal attach/focus missing: focus=%d modal=%v", modal.focused, manager.HasModal())
	}
	if !manager.HandleEscape() || modal.closed != 1 {
		t.Fatalf("Escape did not close newly opened modal: consumed=%v close=%d", manager.HandleEscape(), modal.closed)
	}
}

func TestRenderMayChangePlacementWithoutRenderingTwice(t *testing.T) {
	manager := NewManager(nil)
	content := &reentrantContent{}
	var handle Handle
	var err error
	handle, err = manager.Open(content, Placement{Host: "main", Layer: Overlay})
	if err != nil {
		t.Fatal(err)
	}
	content.onRender = func() {
		if placeErr := manager.Place(handle, Placement{Host: "main", Pane: "editor", Layer: PaneContent}); placeErr != nil {
			t.Fatalf("change placement during Render: %v", placeErr)
		}
	}
	frame := 0
	tester := ui.NewTester(func(c *ui.Context) {
		if frame == 0 {
			manager.Render(c, "main", Overlay)
		} else {
			manager.Render(c, "main", PaneContent)
		}
	}, 400, 300)
	tester.Frame()
	frame++
	tester.Frame()
	if content.rendered == 0 {
		t.Fatal("content was not rendered before its placement changed")
	}
}
