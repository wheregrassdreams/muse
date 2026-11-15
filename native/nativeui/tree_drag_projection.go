package nativeui

import (
	"muse/locales"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/mdedit"
	"muse/mdoc"
)

const (
	treeDragMoveToFolder     = "move-to-folder"
	treeDragMoveToTrash      = "move-to-trash"
	treeDragInsertIntoEditor = "insert-into-editor"
)

func (a *App) setTreeDragMoveOperation(operation, target string) {
	a.treeDragOperation = operation
	a.treeDragTarget = target
}

func (a *App) treeDragOperationLabel() string {
	switch a.treeDragOperation {
	case treeDragMoveToTrash:
		return locales.FilesMoveToTrash()
	case treeDragInsertIntoEditor:
		return locales.FilesInsertIntoEditor()
	case treeDragMoveToFolder:
		return locales.FilesMoveItemToFolder(a.treeDragTarget)
	default:
		return ""
	}
}

// The transparent handle covers the visible row without changing its layout.
// MyGo paints the handle's styled child as the sole drag clone.
func (a *App) treeDragHandle(c *ui.Context, item mdedit.VaultItemDrag) ui.Element {
	w, h := float32(260), float32(28)
	itemID := filepath.ToSlash(item.Path)
	if rel, err := filepath.Rel(a.root, item.Path); err == nil {
		itemID = filepath.ToSlash(rel)
	}
	dragActive := !a.treeDragStarted.IsZero()
	dragPaused := dragActive && c.Now().Sub(a.treeDragStarted) >= 120*time.Millisecond
	showOperation := dragActive && (a.treeDragFrames > 2 || dragPaused || a.treeDragOperation == treeDragMoveToTrash)
	if showOperation {
		w, h = 220, 50
	} else if !a.treeDragStarted.IsZero() {
		c.After(60 * time.Millisecond)
	}
	t := c.Theme()
	handle := ui.Box(c).Key("tree-drag-handle-" + item.Path).Absolute().Left(0).Top(0).
		Width(max(float32(160), a.panelW-16)).Height(28).Center().Cursor(ui.CursorPointer).
		Opacity(0).Label(locales.TreeDragHandleLabel(item.Name)).Drag(item)
	selectedCount, selectedFiles := 0, false
	if handle.Dragging() {
		paths := a.treeDragSelection
		if len(paths) == 0 {
			// App.filesView snapshots the selection after this element is built,
			// so derive it from the live selection on the first drag frame.
			paths = a.selectedTreePaths(item.Path)
		}
		selectedCount, selectedFiles = treeProjectionSelectionLabel(paths)
	}
	imageGhost := handle.Dragging() && selectedCount == 1 && !item.Directory && isImageFile(item.Path)
	if imageGhost {
		w, h = 216, 142
	}
	if item.Path == a.treeDragItem.Path && a.treeAutoExpandDropPath != "" {
		return handle
	}
	handle.Children(func() {
		card := ui.Row(c).Key("tree-drag-projection-card-"+itemID).Label(locales.TreeDragProjectionLabel(itemID)).Width(w).MinWidth(0).Height(h).IgnoreInput(true).Radius(12).Background(t.SurfaceHover).Border(1, t.Border).
			Shadow(0, 7, 20, -4, ui.RGBA(0, 0, 0, 0.14)).
			Padding(0, 8).Gap(8).AlignItems(ui.Center).
			Transition(ui.ElementTransition{Duration: 240 * time.Millisecond, Ease: ui.EaseInOut, Size: true, Colors: true})
		card.Children(func() {
			if imageGhost {
				ui.Column(c).Width(w - 16).MinWidth(0).Gap(4).Children(func() {
					a.treeDragImageThumbnail(c, item.Path)
					ui.Text(c, filepath.Base(item.Path)).FontSize(11.5).FontWeight(600).TextColor(t.Text).SingleLine().Ellipsis("…")
					if showOperation {
						ui.Text(c, locales.FilesInsertIntoEditor()).FontSize(10.5).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
					}
				})
				return
			}
			ui.Column(c).Width(max(float32(20), w-32)).MinWidth(0).Gap(2).Children(func() {
				ui.Row(c).MinWidth(0).Gap(4).AlignItems(ui.Center).Children(func() {
					if selectedCount > 1 {
						drawStackedFileIcon(c, t)
						selectedLabel := locales.FilesSelectedItems(selectedCount)
						if selectedFiles {
							selectedLabel = locales.FilesSelectedFileCount(selectedCount)
						}
						ui.Text(c, selectedLabel).FontSize(12.5).FontWeight(600).TextColor(t.Text).SingleLine().Ellipsis("…")
					} else {
						icon := a.treeIcon(item.Path, item.Directory)
						if isImageFile(item.Path) && a.treeIconID(item.Path) == "" {
							icon = iconImage
						}
						ui.Icon(c, icon).Size(14, 14).TextColor(iconTint(t))
						ui.Text(c, filepath.Base(item.Path)).FontSize(12.5).FontWeight(600).TextColor(t.Text).SingleLine().Ellipsis("…")
					}
				})
				if showOperation {
					ui.Text(c, a.treeDragOperationLabel()).Key("tree-drag-projection-operation-" + itemID).Label(locales.TreeDragProjectionOperationLabel(itemID)).FontSize(10.5).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
				}
			})
		})
	})
	return handle
}

// treeDragSessionProjection survives virtual-list row replacement. MyGo owns
// the active value drag on the pressed handle, but that handle's transient
// visual clone can disappear when expanding a folder rebuilds the list.
func (a *App) treeDragSessionProjection(c *ui.Context) {
	item := a.treeDragItem
	if item.Path == "" {
		return
	}
	itemID := filepath.ToSlash(item.Path)
	if rel, err := filepath.Rel(a.root, item.Path); err == nil {
		itemID = filepath.ToSlash(rel)
	}
	name := filepath.Base(item.Path)
	selectedCount, selectedFiles := treeProjectionSelectionLabel(a.treeDragSelection)
	showSelection := selectedCount > 1
	t := c.Theme()
	card := ui.Row(c).Absolute().Left(a.treeDragPointer.X+12).Top(a.treeDragPointer.Y-58).
		Key("tree-drag-session-projection").Label(locales.TreeDragProjectionLabel(itemID)).
		Width(220).MinWidth(0).Height(50).IgnoreInput(true).Radius(12).
		Background(t.SurfaceHover).Border(1, t.Border).
		Shadow(0, 7, 20, -4, ui.RGBA(0, 0, 0, 0.14)).Padding(0, 8).Gap(8).AlignItems(ui.Center)
	card.Children(func() {
		ui.Column(c).Width(188).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).MinWidth(0).Gap(4).AlignItems(ui.Center).Children(func() {
				if showSelection {
					drawStackedFileIcon(c, t)
					label := locales.FilesSelectedItems(selectedCount)
					if selectedFiles {
						label = locales.FilesSelectedFileCount(selectedCount)
					}
					ui.Text(c, label).FontSize(12.5).FontWeight(600).TextColor(t.Text).SingleLine().Ellipsis("…")
				} else {
					ui.Icon(c, a.treeIcon(item.Path, item.Directory)).Size(14, 14).TextColor(iconTint(t))
					ui.Text(c, name).FontSize(12.5).FontWeight(600).TextColor(t.Text).SingleLine().Ellipsis("…")
				}
			})
			ui.Text(c, a.treeDragOperationLabel()).Label(locales.TreeDragProjectionOperationLabel(itemID)).
				FontSize(10.5).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
		})
	})
}

func (a *App) treeDragImageThumbnail(c *ui.Context, path string) {
	asset := mdoc.LoadImageAsset(path, "", a.invalidate)
	if asset == nil {
		return
	}
	if next, _ := asset.Advance(c.Now()); next > 0 {
		c.After(next)
	}
	bmp := asset.Bitmap()
	t := c.Theme()
	box := ui.Box(c).Key("tree-drag-image-thumbnail").Label(locales.FilesDragImageThumbnail()).FillWidth().Height(88).
		Radius(7).Background(t.Background).Clip().Center()
	box.Children(func() {
		if bmp == nil {
			ui.Icon(c, iconImage).Size(22, 22).TextColor(t.TextMuted)
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

func treeProjectionSelectionLabel(paths []string) (int, bool) {
	if len(paths) == 0 {
		return 0, false
	}
	allFiles := true
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			allFiles = false
			break
		}
	}
	if allFiles {
		return len(paths), true
	}
	return len(paths), false
}

func drawStackedFileIcon(c *ui.Context, t *ui.Theme) {
	ui.Box(c).Size(18, 18).Shrink(0).Children(func() {
		ui.Icon(c, iconFile).Absolute().Left(5).Top(0).Size(11, 13).TextColor(t.TextMuted)
		ui.Icon(c, iconFile).Absolute().Left(0).Top(4).Size(13, 14).TextColor(iconTint(t))
	})
}
