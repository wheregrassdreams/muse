package filetree

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/markdown/images"
	markdownui "muse/nativeui/workbench/features/markdown"
)

// ImagePreview is the presentation input for the hover preview rendered next
// to a file-tree row. The caller owns the hovered path and cache capability.
type ImagePreview struct {
	Path       string
	Anchor     ui.Rect
	CacheStore images.CacheStore
	LocalFiles images.LocalFiles
	OnLoad     func()
}

// RenderImagePreview renders an image preview and reports whether the pointer
// is currently over its popup. The panel owns only placement and rendering;
// image loading and persistent cache storage remain injected capabilities.
func RenderImagePreview(c *ui.Context, preview ImagePreview) bool {
	if preview.Path == "" {
		return false
	}
	asset := markdownui.LoadImageAssetWithLocalFiles(preview.Path, "", preview.LocalFiles, preview.CacheStore, nil, preview.OnLoad)
	if asset == nil {
		return false
	}
	if next, _ := asset.Advance(c.Now()); next > 0 {
		c.After(next)
	}
	bmp := asset.Bitmap()
	vw, vh := c.Size()
	boxRect := PlaceImagePreview(ui.Rect{W: vw, H: vh}, preview.Anchor, 280, 220)
	t := c.Theme()
	box := ui.Box(c).Key("tree-image-hover-preview").Label(locales.ImagePreviewTitle()).Absolute().Left(boxRect.X).Top(boxRect.Y).
		ZIndex(100).
		Width(boxRect.W).Height(boxRect.H).Radius(10).Background(t.Background).Border(1, t.Border).
		Shadow(0, 10, 28, -5, ui.RGBA(0, 0, 0, 0.24)).Clip()
	box.Children(func() {
		ui.Column(c).Fill().Padding(10).Gap(7).Children(func() {
			ui.Text(c, filepath.Base(preview.Path)).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
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
	return box.Hovered()
}

// PlaceImagePreview keeps a preview attached to its hovered row, flips it to
// the row's leading edge when there is no room on the right, and clamps it to
// the viewport.
func PlaceImagePreview(viewport, anchor ui.Rect, width, height float32) ui.Rect {
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
