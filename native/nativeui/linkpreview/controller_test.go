package linkpreview

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestControllerOwnsHoverLifecycle(t *testing.T) {
	controller := NewController(ControllerOptions{})
	if controller.IsOpen() {
		t.Fatal("new controller is open")
	}

	controller.Hover("Notes/Target#Section", 30, 40, ui.Rect{X: 30, Y: 40, W: 80, H: 18})
	snapshot := controller.Snapshot()
	if !controller.IsOpen() || snapshot.Target != "Notes/Target#Section" || snapshot.LinkBounds.W != 80 {
		t.Fatalf("hover state was not recorded: %+v", snapshot)
	}
	if snapshot.Entered.IsZero() || time.Since(snapshot.Entered) > time.Second {
		t.Fatalf("hover entry time was not captured: %+v", snapshot)
	}

	controller.Clear()
	if controller.IsOpen() || controller.Snapshot().Target != "" {
		t.Fatal("clear left the preview active")
	}
}

func TestControllerRenderUsesInjectedResolutionAndSuppression(t *testing.T) {
	var suppressed bool
	controller := NewController(ControllerOptions{Ports: Ports{
		Suppressed: func() bool { return suppressed },
		ResolveNote: func(target string) (NoteTarget, bool) {
			if target != "Notes/Target" {
				return NoteTarget{}, false
			}
			return NoteTarget{Title: "Target", Path: "Notes/Target.md", Source: "Preview body"}, true
		},
	}})
	controller.Hover("Notes/Target", 20, 20, ui.Rect{})

	tester := ui.NewTester(func(context *ui.Context) {
		controller.Render(context)
	}, 800, 600)
	tester.Frame()
	if tester.HasText("Preview body") {
		t.Fatal("preview rendered before the dwell delay")
	}

	time.Sleep(DwellDelay + 10*time.Millisecond)
	tester.Frame()
	if !tester.HasText("Preview body") {
		t.Fatalf("injected note was not rendered after dwell delay: %q", tester.Texts())
	}

	suppressed = true
	tester.Frame()
	if tester.HasText("Preview body") {
		t.Fatal("suppressed preview remained visible")
	}
}

func TestControllerClosesAfterLeavingScrolledOverlay(t *testing.T) {
	controller := NewController(ControllerOptions{Ports: Ports{
		ResolveNote: func(target string) (NoteTarget, bool) {
			return NoteTarget{Title: "Target", Path: "Target.md", Source: "Preview body"}, target == "Target"
		},
	}})
	controller.Hover("Target", 0, 0, ui.Rect{})
	tester := ui.NewTester(func(context *ui.Context) {
		controller.Render(context)
	}, 240, 160)
	tester.Frame()
	time.Sleep(DwellDelay + 10*time.Millisecond)
	tester.Frame()
	if !tester.HasText("Preview body") {
		t.Fatalf("preview did not render after dwell delay: %q", tester.Texts())
	}
	card, ok := tester.Find("链接预览")
	if !ok {
		t.Fatalf("preview card missing: %q", tester.Texts())
	}
	tester.Move(card.X+card.W/2, card.Y+card.H/2)
	tester.Frame()
	if !controller.Snapshot().OverlayHovered || !controller.Snapshot().LeaveAt.IsZero() {
		t.Fatalf("entering preview started its dismissal: %+v", controller.Snapshot())
	}
	tester.Scroll(card.X+card.W/2, card.Y+card.H/2, 0, 90)
	tester.Frame()
	if !controller.IsOpen() || !controller.Snapshot().OverlayHovered || !controller.Snapshot().LeaveAt.IsZero() {
		t.Fatalf("scrolling inside preview lost its hover grace: %+v", controller.Snapshot())
	}
	tester.Move(1, 1)
	tester.Frame()
	if !controller.IsOpen() || controller.Snapshot().OverlayHovered || controller.Snapshot().LeaveAt.IsZero() {
		t.Fatalf("leaving preview did not start dismissal: %+v", controller.Snapshot())
	}
	time.Sleep(LeaveDelay + 10*time.Millisecond)
	tester.Frame()
	if controller.IsOpen() {
		t.Fatalf("preview remained after pointer left: %+v", controller.Snapshot())
	}
}
