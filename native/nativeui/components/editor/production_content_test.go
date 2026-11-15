package editor

import (
	"testing"

	"github.com/egoist/mygo/ui"

	tappeditor "muse/editor"
	texmath "muse/markdown/math"
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

func TestProductionContentTranslatesVimInputThroughSession(t *testing.T) {
	value := tappeditor.New("abc", tappeditor.Options{})
	value.SetKeymap("vim")
	content := NewProductionContent(value, nil)
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { content.Render(c) })
	}, 1000, 700)
	tester.Frame()
	value.Focus()
	tester.Frame()
	tester.Type("l")
	tester.Type("i")
	tester.Type("X")
	tester.KeyDown(ui.KeyEscape)
	tester.Frame()
	if got, want := value.Session.Source(), "aXbc"; got != want {
		t.Fatalf("source after Vim input = %q, want %q", got, want)
	}
}

func TestProductionContentPaintsNeutralDiagramModel(t *testing.T) {
	source := "```mermaid\ngraph LR\nA[one] --> B[two]\n```\n\nend"
	value := tappeditor.New(source, tappeditor.Options{})
	if err := value.Session.SetSelection(tappeditor.Selection{Anchor: len(source), Head: len(source)}); err != nil {
		t.Fatal(err)
	}
	content := NewProductionContent(value, nil)
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { content.Render(c) })
	}, 1000, 700)
	tester.Frame()
	tester.Frame()
	if len(content.diagrams) == 0 {
		t.Fatal("production editor did not send its neutral diagram model to the component painter")
	}
}

func TestProductionContentRendersNeutralMathRaster(t *testing.T) {
	source := "$$\n\\sum_{k=1}^{7} k^2\n$$\n\nend"
	if _, ok := texmath.TypesetCached("\\sum_{k=1}^{7} k^2", 19.2, true); !ok {
		t.Fatal("math fixture did not typeset")
	}
	value := tappeditor.New(source, tappeditor.Options{})
	if err := value.Session.SetSelection(tappeditor.Selection{Anchor: len(source), Head: len(source)}); err != nil {
		t.Fatal(err)
	}
	content := NewProductionContent(value, nil)
	drawn := 0
	value.SetMathRenderer(func(p *ui.Painter, r ui.Rect, raster *texmath.Raster, dark bool, font string) {
		drawn++
		drawMathRaster(p, r, raster, dark, font)
	})
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { content.Render(c) })
	}, 1000, 700)
	tester.Frame()
	tester.Frame()
	if drawn == 0 {
		t.Fatal("production editor did not send a neutral math raster to the component renderer")
	}
}
