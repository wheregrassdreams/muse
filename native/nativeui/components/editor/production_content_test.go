package editor

import (
	"fmt"
	"strings"
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

func TestProductionContentPaintsInlineMathPreviewFromNeutralFrame(t *testing.T) {
	const source = "当 $A=\\frac{1}{2}$ 时\n\n> 函数要求 domain 中的每一个元素都映射到 codomain。\n> 第二行引用文字。\n"
	theme := ui.DarkTheme()
	value := tappeditor.New(source, tappeditor.Options{FontSize: 16})
	if err := value.Session.SetSelection(tappeditor.Selection{Anchor: len("当 $A"), Head: len("当 $A")}); err != nil {
		t.Fatal(err)
	}
	content := NewProductionContent(value, nil)
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Themed(c, theme).Fill().Background(theme.Background).Children(func() { content.Render(c) })
	}, 1000, 700)
	tester.Frame()
	value.Focus()
	tester.Frame()
	frame, ok := value.InlineMathPreview()
	if !ok || frame.Raster == nil || frame.Failed {
		t.Fatalf("production inline math snapshot = %+v, ok=%v", frame, ok)
	}
	viewport := ui.Rect{X: frame.Viewport.X, Y: frame.Viewport.Y, W: frame.Viewport.W, H: frame.Viewport.H}
	anchor := ui.Rect{X: frame.Anchor.X, Y: frame.Anchor.Y, W: frame.Anchor.W, H: frame.Anchor.H}
	card, ok := mathPreviewBounds(viewport, anchor, frame.Width+24, frame.Height+24)
	if !ok {
		t.Fatal("production inline math card is outside the visible viewport")
	}
	img := tester.Image()
	matched, total := 0, 0
	for y := int(card.Y) + 4; y < int(card.Y+card.H)-4; y++ {
		for x := int(card.X) + 4; x < int(card.X+card.W)-4; x++ {
			total++
			pixel := img.RGBAAt(x, y)
			if pixel.R == theme.Background.R && pixel.G == theme.Background.G && pixel.B == theme.Background.B {
				matched++
			}
		}
	}
	if matched*100 < total*70 {
		t.Fatalf("production preview surface is translucent/occluded: %d of %d opaque surface pixels", matched, total)
	}
}

func TestMathPreviewBounds(t *testing.T) {
	view := ui.Rect{X: 100, Y: 50, W: 400, H: 300}
	for _, tc := range []struct {
		name           string
		anchor         ui.Rect
		w, h           float32
		visible, above bool
	}{
		{"below", ui.Rect{X: 120, Y: 80, W: 2, H: 20}, 200, 60, true, false},
		{"flip", ui.Rect{X: 480, Y: 300, W: 2, H: 20}, 200, 60, true, true},
		{"oversize", ui.Rect{X: 120, Y: 100, W: 2, H: 20}, 900, 900, true, false},
		{"scrolled out", ui.Rect{X: 120, Y: 10, W: 2, H: 20}, 200, 60, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, ok := mathPreviewBounds(view, tc.anchor, tc.w, tc.h)
			if ok != tc.visible {
				t.Fatalf("visible=%v", ok)
			}
			if !ok {
				return
			}
			if r.X < view.X || r.Y < view.Y || r.X+r.W > view.X+view.W || r.Y+r.H > view.Y+view.H {
				t.Fatalf("outside viewport: %+v", r)
			}
			if tc.above && r.Y+r.H > tc.anchor.Y {
				t.Fatalf("did not flip: %+v", r)
			}
		})
	}
}

func TestProductionHeadingRailOwnsHoverScrollAndJump(t *testing.T) {
	var source strings.Builder
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&source, "## Heading %d\n\n", i)
		for j := 0; j < 12; j++ {
			fmt.Fprintf(&source, "Paragraph %d.%d keeps the document tall enough to scroll.\n\n", i, j)
		}
	}
	text := source.String()
	value := tappeditor.New(text, tappeditor.Options{})
	if err := value.Session.SetSelection(tappeditor.Selection{Anchor: 0, Head: 0}); err != nil {
		t.Fatal(err)
	}
	content := NewProductionContent(value, nil)
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { content.Render(c) })
	}, 900, 420)
	tester.Frame()
	if content.rail.rect.W == 0 || content.rail.rect.H == 0 {
		t.Fatal("heading rail was not rendered for a scrollable document")
	}
	if content.rail.active != 0 {
		t.Fatalf("initial rail selection = %d, want first heading", content.rail.active)
	}
	var pitch float32
	ys := headingRailPositions(nil, 8, content.rail.rect.H, &pitch)
	point := 3
	x := content.rail.rect.X + content.rail.rect.W/2
	y := content.rail.rect.Y + ys[point]
	caretBefore := value.Caret()
	tester.Move(x, y)
	tester.Frame()
	if content.rail.hover != point {
		t.Fatalf("hovered heading index = %d, want %d", content.rail.hover, point)
	}
	preview := value.HeadingRailPreview(point)
	if preview.Title != "Heading 3" || !strings.Contains(preview.Excerpt, "Paragraph 3.0") {
		t.Fatalf("rail preview = %+v, want hovered heading and section excerpt", preview)
	}
	if value.Caret() != caretBefore {
		t.Fatal("hovering the rail changed the editor caret")
	}
	tester.ClickAt(x, y)
	for i := 0; i < 3; i++ {
		tester.Frame()
	}
	wantCaret := strings.Index(text, "## Heading 3") + len("## ")
	if got := value.Caret(); got != wantCaret {
		t.Fatalf("rail click caret = %d, want heading offset %d", got, wantCaret)
	}
	if content.rail.active != point {
		t.Fatalf("rail active heading after jump = %d, want %d", content.rail.active, point)
	}

	narrow := tappeditor.New(text, tappeditor.Options{})
	narrowContent := NewProductionContent(narrow, nil)
	narrowTester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { narrowContent.Render(c) })
	}, 48, 180)
	narrowTester.Frame()
	if narrowContent.rail.rect.W != 0 {
		t.Fatalf("narrow window still shows heading rail: %+v", narrowContent.rail.rect)
	}
}
