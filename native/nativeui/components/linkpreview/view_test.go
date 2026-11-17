package linkpreview

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/markdown"
	markdownui "muse/nativeui/components/markdown"
)

func TestPlaceKeepsTriggerVisible(t *testing.T) {
	viewport := ui.Rect{W: 800, H: 600}
	link := ui.Rect{X: 650, Y: 130, W: 92, H: 24}
	card := Place(viewport, link, 380, 300)
	if card.X < 0 || card.Y < 0 || card.X+card.W > viewport.W || card.Y+card.H > viewport.H {
		t.Fatalf("preview card escaped viewport: card=%+v viewport=%+v", card, viewport)
	}
	if card.Y < link.Y+link.H && card.Y+card.H > link.Y {
		t.Fatalf("preview card overlaps its trigger: link=%+v card=%+v", link, card)
	}
}

func TestClearReleasesImagePreviewResources(t *testing.T) {
	state := state{
		Target: "Attachments/photo.png", imagePath: "/vault/Attachments/photo.png",
		imageAsset: &markdownui.ImageAsset{}, imageBitmap: &ui.Bitmap{},
	}
	state.Clear()
	if state.Target != "" || state.ImagePath() != "" || state.HasImageAsset() || state.ImageBitmap() != nil {
		t.Fatalf("cleared preview retained image state: %+v", state)
	}

	state = state{Target: "Attachments/photo.png", imagePath: "/vault/Attachments/photo.png", imageAsset: &markdownui.ImageAsset{}, imageBitmap: &ui.Bitmap{}}
	state.Hover("Projects/Target", 1, 2, ui.Rect{}, time.Now(), time.Second)
	if state.ImagePath() != "" || state.HasImageAsset() || state.ImageBitmap() != nil {
		t.Fatalf("changing hover target retained previous image resources: %+v", state)
	}
}

func TestMarkdownPreviewRendersTypedDocument(t *testing.T) {
	model := ViewModel{
		Kind: Markdown, Title: "Target note", Path: "folder/target.md",
		Viewport: ui.Rect{W: 700, H: 500}, Anchor: ui.Rect{X: 20, Y: 20, W: 80, H: 20},
		Document: markdown.Parse("# Preview heading\n\nPreview body."),
	}
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, model, nil)
	}, 700, 500)
	tester.Frame()
	if !tester.HasText("Target note") || !tester.HasText("Preview body.") {
		t.Fatalf("Markdown preview is missing its title or rendered body: %q", tester.Texts())
	}
}

func TestImagePreviewDispatchesHoverTransitions(t *testing.T) {
	model := ViewModel{Kind: Image, Path: "/vault/photo.png", Viewport: ui.Rect{W: 600, H: 400}, Anchor: ui.Rect{X: 30, Y: 20, W: 50, H: 20}, Width: 280, Height: 220}
	var actions []Action
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, model, func(action Action) {
			actions = append(actions, action)
			if action.Kind == HoverChanged {
				model.OverlayHovered = action.Hovered
			}
		})
	}, 600, 400)
	tester.Frame()
	if !tester.HasText(locales.ImagePreviewUnavailable()) {
		t.Fatalf("missing image state not rendered: %q", tester.Texts())
	}
	box, ok := tester.Find(locales.ImageLinkPreviewTitle())
	if !ok {
		t.Fatal("image preview card missing")
	}
	tester.Move(box.X+box.W/2, box.Y+box.H/2)
	tester.Frame()
	if len(actions) != 1 || actions[0].Kind != HoverChanged || !actions[0].Hovered {
		t.Fatalf("hover enter actions = %+v", actions)
	}
	model.OverlayHovered = true
	tester.Move(590, 390)
	tester.Frame()
	if len(actions) != 2 || actions[1].Kind != HoverChanged || actions[1].Hovered {
		t.Fatalf("hover leave actions = %+v", actions)
	}
}
