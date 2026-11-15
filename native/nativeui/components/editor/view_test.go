package editor

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"muse/editor"
)

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
