package editor

import (
	"strings"
	"testing"
)

func TestVirtualScrollShapesEveryVisibleProseLeafAfterLargeScrolls(t *testing.T) {
	const paragraph = "这是一个用于检查虚拟滚动布局是否漏掉可见内容的中文段落。连续滚动时，屏幕里的每一段都应该及时完成布局并绘制出来。\n\n"
	e := New(strings.Repeat(paragraph, 1000), Options{FontSize: 16})
	tt := stressTester(e)
	tt.Frame()
	tt.Frame()

	// Reproduce ordinary scrolling followed by a larger jump that crosses the
	// cached layout band, then scroll back across the same boundary.
	for _, delta := range []float32{500, 500, 900, -300, 10000, -800} {
		tt.Scroll(600, 400, 0, delta)
		tt.Frame()

		top, bottom := e.scroll.Y, e.scroll.Y+e.viewH
		for i, leaf := range e.visibleLeaves {
			y := leaf.y
			if e.virtualIndexReady {
				y = e.virtualY(i)
			}
			if leaf.kind == kPara && y+leaf.h > top && y < bottom && leaf.view == nil {
				t.Fatalf("visible paragraph %d at y=%.1f h=%.1f is still estimated after scroll to %.1f (viewport %.1f..%.1f)",
					i, y, leaf.h, e.scroll.Y, top, bottom)
			}
		}

		img := tt.Image()
		for y := 50; y < int(e.viewH)-25; y += 100 {
			ink := 0
			for row := y; row < min(y+100, img.Bounds().Dy()); row++ {
				for x := 40; x < 900; x++ {
					r, g, b, _ := img.At(x, row).RGBA()
					if r < 30000 && g < 30000 && b < 30000 {
						ink++
					}
				}
			}
			if ink == 0 {
				t.Fatalf("rendered viewport has a blank 100px band at y=%d after scroll to %.1f", y, e.scroll.Y)
			}
		}
	}
}
