package nativeui

import (
	"muse/locales"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"muse/mdoc"
)

// treeImagePreview draws a compact image popup adjacent to a hovered tree
// row. ImageAsset is shared by the renderer's filesystem cache, so invoking
// this on each hovered frame does not reread or decode the image repeatedly.
func (a *App) treeImagePreview(c *ui.Context, path string, anchor ui.Rect) {
	if path == "" || !isImageFile(path) {
		return
	}
	asset := mdoc.LoadImageAsset(path, "", a.invalidate)
	if asset == nil {
		return
	}
	if next, _ := asset.Advance(c.Now()); next > 0 {
		c.After(next)
	}
	bmp := asset.Bitmap()
	vw, vh := c.Size()
	boxRect := placeTreeImagePreview(ui.Rect{W: vw, H: vh}, anchor, 280, 220)
	t := c.Theme()
	box := ui.Box(c).Key("tree-image-hover-preview").Label(locales.ImagePreviewTitle()).Absolute().Left(boxRect.X).Top(boxRect.Y).
		ZIndex(100).
		Width(boxRect.W).Height(boxRect.H).Radius(10).Background(t.Background).Border(1, t.Border).
		Shadow(0, 10, 28, -5, ui.RGBA(0, 0, 0, 0.24)).Clip()
	a.treePreviewPopupHovered = box.Hovered()
	box.Children(func() {
		ui.Column(c).Fill().Padding(10).Gap(7).Children(func() {
			ui.Text(c, filepath.Base(path)).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
			ui.Box(c).FillWidth().Height(1).Background(t.Border)
			if bmp == nil {
				ui.Text(c, locales.ImagePreviewUnavailable()).Fill().Center().TextColor(t.TextMuted)
				return
			}
			bw, bh := ui.ImageSize(bmp)
			if bw <= 0 || bh <= 0 {
				return
			}
			fit := min(254/bw, 178/bh)
			if fit <= 0 {
				fit = 1
			}
			ui.Box(c).Fill().Center().Children(func() {
				ui.Image(c, bmp).Key("tree-image-hover-thumbnail").Size(bw*fit, bh*fit)
			})
		})
	})
}

// placeTreeImagePreview keeps the preview attached to the hovered row. Prefer
// the row's trailing edge, so the card sits just outside the sidebar instead
// of at a fixed distance from the editor origin. Flip to the leading edge when
// the viewport has no room on the right, and clamp both axes to the window.
func placeTreeImagePreview(viewport, anchor ui.Rect, width, height float32) ui.Rect {
	const (
		edge = float32(8)
		gap  = float32(8)
	)
	width = min(width, max(viewport.W-2*edge, 1))
	height = min(height, max(viewport.H-2*edge, 1))
	right := anchor.X + anchor.W + gap
	left := anchor.X - width - gap
	x := right
	if right+width > viewport.X+viewport.W-edge {
		if left >= viewport.X+edge {
			x = left
		} else {
			x = max(viewport.X+edge, min(right, viewport.X+viewport.W-width-edge))
		}
	}
	y := max(viewport.Y+edge, min(anchor.Y, viewport.Y+viewport.H-height-edge))
	return ui.Rect{X: x, Y: y, W: width, H: height}
}
