package filetree

import (
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/markdown/images"
	markdownui "muse/nativeui/components/markdown"
)

// DragProjection contains the immutable visual state for the current tree
// drag. The caller resolves selection, labels, icons, and operation policy.
type DragProjection struct {
	Active            bool
	StartedAt         time.Time
	Now               time.Time
	Frames            int
	Pointer           ui.Point
	ItemID            string
	ItemName          string
	ItemKind          EntryKind
	ItemIcon          *ui.SVG
	FileIcon          *ui.SVG
	IconTint          ui.Color
	SelectedCount     int
	SelectedFilesOnly bool
	OperationLabel    string
	OperationID       string
	ImagePath         string
	ImageCacheStore   images.CacheStore
	LocalFiles        images.LocalFiles
	OnImageLoad       func()
}

// RenderDragProjection draws one stable drag card while the virtual list
// replaces rows. All selection and operation semantics arrive as values.
func RenderDragProjection(c *ui.Context, model DragProjection) {
	if !model.Active || model.ItemID == "" {
		return
	}
	w, h := float32(220), float32(50)
	showOperation := model.Frames > 2 || !model.StartedAt.IsZero() && model.Now.Sub(model.StartedAt) >= 120*time.Millisecond || model.OperationID == "move-to-trash"
	isImage := model.SelectedCount == 1 && model.ItemKind == File && model.ImagePath != ""
	if isImage {
		w, h = 216, 142
	} else if showOperation {
		w, h = 220, 50
	}
	t := c.Theme()
	card := ui.Row(c).Absolute().Left(model.Pointer.X+12).Top(model.Pointer.Y-58).
		Key("tree-drag-session-projection").Label(locales.TreeDragProjectionLabel(model.ItemID)).
		Width(w).MinWidth(0).Height(h).IgnoreInput(true).Radius(12).
		Background(t.SurfaceHover).Border(1, t.Border).
		Shadow(0, 7, 20, -4, ui.RGBA(0, 0, 0, 0.14)).Padding(0, 8).Gap(8).AlignItems(ui.Center)
	card.Children(func() {
		if isImage {
			ui.Column(c).Width(w - 16).MinWidth(0).Gap(4).Children(func() {
				renderDragImage(c, model, t)
				ui.Text(c, filepath.Base(model.ItemName)).FontSize(11.5).FontWeight(600).TextColor(t.Text).SingleLine().Ellipsis("…")
				if showOperation && model.OperationLabel != "" {
					ui.Text(c, model.OperationLabel).FontSize(10.5).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
				}
			})
			return
		}
		ui.Column(c).Width(w - 32).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).MinWidth(0).Gap(4).AlignItems(ui.Center).Children(func() {
				if model.SelectedCount > 1 {
					drawStackedFileIcon(c, model.FileIcon, t)
					label := locales.FilesSelectedItems(model.SelectedCount)
					if model.SelectedFilesOnly {
						label = locales.FilesSelectedFileCount(model.SelectedCount)
					}
					ui.Text(c, label).FontSize(12.5).FontWeight(600).TextColor(t.Text).SingleLine().Ellipsis("…")
				} else {
					if model.ItemIcon != nil {
						ui.Icon(c, model.ItemIcon).Size(14, 14).TextColor(model.IconTint)
					}
					ui.Text(c, filepath.Base(model.ItemName)).FontSize(12.5).FontWeight(600).TextColor(t.Text).SingleLine().Ellipsis("…")
				}
			})
			if showOperation && model.OperationLabel != "" {
				ui.Text(c, model.OperationLabel).Key("tree-drag-projection-operation-" + model.ItemID).
					Label(locales.TreeDragProjectionOperationLabel(model.ItemID)).FontSize(10.5).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
			}
		})
	})
}

func renderDragImage(c *ui.Context, model DragProjection, t *ui.Theme) {
	asset := markdownui.LoadImageAssetWithLocalFiles(model.ImagePath, "", model.LocalFiles, model.ImageCacheStore, nil, model.OnImageLoad)
	if asset == nil {
		return
	}
	if next, _ := asset.Advance(c.Now()); next > 0 {
		c.After(next)
	}
	bmp := asset.Bitmap()
	box := ui.Box(c).Key("tree-drag-image-thumbnail").Label(locales.FilesDragImageThumbnail()).FillWidth().Height(88).
		Radius(7).Background(t.Background).Clip().Center()
	box.Children(func() {
		if bmp == nil {
			if model.ItemIcon != nil {
				ui.Icon(c, model.ItemIcon).Size(22, 22).TextColor(t.TextMuted)
			}
			return
		}
		width, height := ui.ImageSize(bmp)
		if width <= 0 || height <= 0 {
			return
		}
		fit := min(184/width, 88/height)
		if fit <= 0 {
			fit = 1
		}
		ui.Image(c, bmp).Size(width*fit, height*fit)
	})
}

func drawStackedFileIcon(c *ui.Context, icon *ui.SVG, t *ui.Theme) {
	ui.Box(c).Size(18, 18).Shrink(0).Children(func() {
		if icon == nil {
			return
		}
		ui.Icon(c, icon).Absolute().Left(5).Top(0).Size(11, 13).TextColor(t.TextMuted)
		ui.Icon(c, icon).Absolute().Left(0).Top(4).Size(13, 14).TextColor(t.Text)
	})
}
