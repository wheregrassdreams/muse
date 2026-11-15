package editor

import (
	"testing"

	"github.com/egoist/mygo/ui"

	tappeditor "muse/editor"
	"muse/nativeui/window"
)

func TestProductionContentHostsVirtualizedEditorAndClosesOnce(t *testing.T) {
	value := tappeditor.New("hello", tappeditor.Options{})
	if err := value.Session.SetSelection(tappeditor.Selection{Anchor: len(value.Source()), Head: len(value.Source())}); err != nil {
		t.Fatal(err)
	}
	closed := 0
	content := NewProductionContent(value, func() { closed++ })
	var pane window.Content = content
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { pane.Render(c) })
	}, 1000, 700)
	tester.Frame()
	value.Focus()
	tester.Frame()
	tester.Type("!")
	tester.Frame()
	if got, want := value.Session.Source(), "hello!"; got != want {
		t.Fatalf("production editor source = %q, want %q", got, want)
	}
	pane.Close()
	pane.Close()
	tester.Frame()
	if closed != 1 {
		t.Fatalf("OnClose calls = %d, want 1", closed)
	}
}
