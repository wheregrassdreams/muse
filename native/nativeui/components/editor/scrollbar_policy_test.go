package editor

import (
	"github.com/egoist/mygo/ui"
	"muse/ui/scrollbar"
	"strings"
	"testing"
	"time"
)

func TestEditorScrollbarOnlyAfterScrolling(t *testing.T) {
	e := New(strings.Repeat("paragraph content\n\n", 100), Options{})
	var now time.Time
	tt := ui.NewTester(func(c *ui.Context) { now = c.Now(); ui.Column(c).Fill().Children(func() { e.View(c).Grow(1) }) }, 1000, 700)
	tt.Frame()
	tt.Frame()
	tt.Move(990, 100)
	tt.Frame()
	if e.scrollIndicator.Visible(now, e.scroll.X, e.scroll.Y) {
		t.Fatal("hover showed scrollbar")
	}
	tt.Scroll(500, 200, 0, 120)
	tt.Frame()
	tt.Frame()
	if e.scroll.Y == 0 || !e.scrollIndicator.Visible(now, e.scroll.X, e.scroll.Y) {
		t.Fatal("actual scrolling did not show scrollbar")
	}
	tt.Move(990, 200)
	tt.Advance(scrollbar.VisibleFor + time.Second)
	tt.Frame()
	if e.scrollIndicator.Visible(now, e.scroll.X, e.scroll.Y) {
		t.Fatal("hover kept scrollbar alive")
	}
}
