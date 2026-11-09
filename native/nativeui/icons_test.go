package nativeui

import (
	"image/color"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestDarkIconsStayReadable checks that the shell's thin line icons keep
// enough contrast in the dark theme: the muted grey that reads well on
// white nearly disappears against the near-black surface, so the icons
// draw lighter than the labels beside them there.
func TestDarkIconsStayReadable(t *testing.T) {
	a := New()
	a.themeMode = 1
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()

	r, ok := tt.Find("AI 助手")
	if !ok {
		t.Fatal("the AI button is not on screen")
	}
	img := tt.Image()
	brightest := 0.0
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := int(r.X); x < int(r.X+r.W); x++ {
			if l := iconLuminance(img.RGBAAt(x, y)); l > brightest {
				brightest = l
			}
		}
	}
	if brightest < 0.5 {
		t.Fatalf("the darkest-reading stroke tops out at %.2f luminance, want at least 0.50", brightest)
	}
}

func iconLuminance(c color.RGBA) float64 {
	linear := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
}
