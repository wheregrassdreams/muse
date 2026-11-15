package editor

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"muse/editor"
	"muse/nativeui/window"
)

func TestSurfaceImplementsPaneContentLifecycle(t *testing.T) {
	session := editor.NewSession("# title")
	closed := 0
	surface := NewSurface(session, Options{OnClose: func() { closed++ }})
	var content window.Content = surface
	tester := ui.NewTester(func(c *ui.Context) { content.Render(c) }, 640, 480)
	tester.Frame()
	content.Close()
	content.Close()
	tester.Frame()
	if closed != 1 {
		t.Fatalf("OnClose calls = %d, want 1", closed)
	}
}

func TestViewInputEditsSessionAndRendersUpdatedMarkdown(t *testing.T) {
	session := editor.NewSession("# title\n")
	if err := session.SetSelection(editor.Selection{Anchor: len(session.Source()), Head: len(session.Source())}); err != nil {
		t.Fatal(err)
	}
	changed := 0
	tester := ui.NewTester(func(c *ui.Context) {
		View(c, session, Options{OnChange: func() { changed++ }})
	}, 640, 480)
	tester.Frame()
	tester.Type("body")
	tester.Frame()

	if got, want := session.Source(), "# title\nbody"; got != want {
		t.Fatalf("source = %q, want %q", got, want)
	}
	if changed == 0 {
		t.Fatal("OnChange was not called")
	}
	if frame := session.Frame(); frame.Rendered == nil || frame.Source != session.Source() {
		t.Fatalf("render frame is stale: %+v", frame)
	}
}

func TestViewDispatchesHorizontalMovementThroughProductionSessionOperation(t *testing.T) {
	production := editor.New("abc", editor.Options{})
	if err := production.Session.SetSelection(editor.Selection{Anchor: 1, Head: 1}); err != nil {
		t.Fatal(err)
	}
	tester := ui.NewTester(func(c *ui.Context) {
		View(c, production.Session, Options{})
	}, 640, 480)
	tester.Frame()
	tester.KeyDown(ui.KeyRight)
	tester.Frame()
	if got := production.Session.Selection().Head; got != 2 {
		t.Fatalf("selection after component key dispatch = %d, want 2", got)
	}
}
