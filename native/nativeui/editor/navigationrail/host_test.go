package navigationrail

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

type testHost struct {
	frame       Frame
	previews    []int
	jumpRevs    []uint64
	jumpOffsets []int
}

func TestHandleConsumesDetachedFrameAndTypedNavigationActions(t *testing.T) {
	host := &testHost{frame: Frame{
		Visible: true, Active: 1,
		Items: []Item{{Offset: 10, Level: 1}, {Offset: 20, Level: 2}, {Offset: 30, Level: 3}},
	}}
	handle := NewHandle()
	bounds := ui.Rect{X: 20, Y: 30, W: 320, H: 240}
	tester := ui.NewTester(func(c *ui.Context) {
		handle.Draw(c, bounds, host.frame, func(index int) Preview {
			host.previews = append(host.previews, index)
			return Preview{Title: "Heading"}
		}, func(revision uint64, offset int) {
			host.jumpRevs = append(host.jumpRevs, revision)
			host.jumpOffsets = append(host.jumpOffsets, offset)
		})
	}, 400, 320)
	tester.Frame()
	if got := handle.Snapshot().Active; got != 1 {
		t.Fatalf("active item = %d, want 1", got)
	}

	var pitch float32
	ys := Positions(nil, len(host.frame.Items), bounds.H-2*Inset, &pitch)
	index := 2
	x := bounds.X + bounds.W - Inset - Width/2
	y := bounds.Y + Inset + ys[index]
	tester.Move(x, y)
	tester.Frame()
	if got := handle.Snapshot().Hover; got != index {
		t.Fatalf("hovered item = %d, want %d", got, index)
	}
	if len(host.previews) == 0 || host.previews[len(host.previews)-1] != index {
		t.Fatalf("preview requests = %v, want hovered index %d", host.previews, index)
	}
	tester.ClickAt(x, y)
	tester.Frame()
	if len(host.jumpOffsets) != 1 || host.jumpOffsets[0] != host.frame.Items[index].Offset || host.jumpRevs[0] != host.frame.Revision {
		t.Fatalf("jump offsets = %v, want heading offset %d", host.jumpOffsets, host.frame.Items[index].Offset)
	}
}
