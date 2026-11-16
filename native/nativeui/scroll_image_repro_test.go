package nativeui

import (
	"image"
	"testing"
	"time"
)

// TestMouseWheelHomeFixtureImagesStayRendered follows the app-level path of
// opening Home.md and scrolling its editor with animated mouse-wheel events.
func TestMouseWheelHomeFixtureImagesStayRendered(t *testing.T) {
	a, tt, _ := runtimeFixtureCopy(t, "../testdata/vault-fixture")
	waitDocument(t, tt, func() bool {
		_, ok := a.vault.Document("Projects/Atlas/Overview.md")
		return ok
	})
	tt.Frame()
	runtimeShot(t, tt, "home-mouse-wheel-000-top")
	for i := 1; i <= 10; i++ {
		tt.Wheel(900, 420, 0, 120)
		tt.Advance(180 * time.Millisecond)
		tt.Frame()
		if i == 10 {
			runtimeShot(t, tt, "home-mouse-wheel-wiki-embeds")
			if ink := contentInkPixels(tt.Image()); ink < 20_000 {
				t.Fatalf("Home.md image content stayed blank after mouse wheel reached scroll %.1f: content ink pixels=%d", a.activeNote.NavigationCommands().ScrollY(), ink)
			}
		}
	}
	if a.activeNote.NavigationCommands().ScrollY() < 1200 {
		t.Fatalf("mouse wheel stopped before the Home.md attachment image: scroll=%v", a.activeNote.NavigationCommands().ScrollY())
	}
}

func contentInkPixels(img image.Image) int {
	count := 0
	for y := 70; y < min(img.Bounds().Dy()-20, 780); y++ {
		for x := 300; x < min(img.Bounds().Dx()-20, 1180); x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < 56000 || g < 56000 || b < 56000 {
				count++
			}
		}
	}
	return count
}
