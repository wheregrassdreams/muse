package mdedit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTableOuterCornersAreRounded(t *testing.T) {
	const src = "| Header | Second |\n| --- | --- |\n| Cell | Value |"
	for _, dark := range []bool{false, true} {
		name, theme := "light", ui.LightTheme()
		if dark {
			name, theme = "dark", ui.DarkTheme()
		}
		t.Run(name, func(t *testing.T) {
			h := &harness{t: t, e: New(src, Options{FontSize: 16})}
			h.tt = ui.NewTester(func(c *ui.Context) {
				ui.Themed(c, theme).Fill().Background(theme.Background).Children(func() {
					h.e.View(c).Grow(1)
				})
			}, 1000, 700)
			h.frame()

			var table *leaf
			for _, l := range h.e.leaves {
				if l.tab != nil {
					table = l
					break
				}
			}
			if table == nil || table.geom == nil {
				t.Fatal("table geometry was not laid out")
			}
			h.shot("table-rounded-" + name)

			img := h.tt.Image()
			bg := img.RGBAAt(5, 5)
			corner := img.RGBAAt(int(table.x), int(table.y))
			if corner != bg {
				t.Fatalf("outer corner pixel = %v, want page background %v", corner, bg)
			}
			outline := img.RGBAAt(int(table.x)+7, int(table.y))
			if outline == bg {
				t.Fatal("rounded top outline is missing")
			}
		})
	}
}
