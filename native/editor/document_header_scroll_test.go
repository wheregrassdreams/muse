package mdedit

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
	"time"
)

func TestDocumentHeaderScrollsWithPage(t *testing.T) {
	e := New(strings.Repeat("body paragraph\n\n", 80), Options{HeaderHeight: 60})
	var h ui.Handle
	var headerY float32
	opts := e.Options()
	opts.Header = func(c *ui.Context) { ui.Text(c, "Page title").Bind(&h); headerY = h.Bounds(c).Y }
	e.SetOptions(opts)
	tt := ui.NewTester(func(c *ui.Context) { ui.Column(c).Fill().Children(func() { e.View(c).Grow(1) }) }, 800, 600)
	tt.Frame()
	tt.Frame()
	before := headerY
	tt.Wheel(500, 400, 0, 180)
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	tt.Frame()
	if e.scroll.Y <= 0 || headerY >= before-20 {
		t.Fatalf("header fixed: before=%v after=%v scroll=%v", before, headerY, e.scroll.Y)
	}
}
