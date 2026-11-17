package filetree

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/markdown/images"
	markdownui "muse/nativeui/components/markdown"
)

// CatalogEntry is a path and kind projected from the vault's directory
// catalog. The host owns catalog access; this panel owns which catalog
// entries become visible rows and their presentation order.
type CatalogEntry struct {
	Path string
	Kind CatalogKind
}

type CatalogKind uint8

const (
	CatalogFile CatalogKind = iota + 1
	CatalogDirectory
)

// SelectionSummary reports the number of dragged paths and whether the
// selection contains only files. The caller supplies catalog knowledge so the
// panel remains independent of vault discovery and filesystem access.
func SelectionSummary(paths []string, isDirectory func(string) bool) (count int, filesOnly bool) {
	if len(paths) == 0 {
		return 0, false
	}
	for _, path := range paths {
		if isDirectory == nil || isDirectory(path) {
			return len(paths), false
		}
	}
	return len(paths), true
}

// ProjectChildren filters and orders one directory's catalog snapshot. File
// type recognition stays with the host and is supplied as a narrow
// capability; hidden entries and directory-first name ordering belong to the
// file-tree projection.
func ProjectChildren(root string, entries []CatalogEntry, visibleFile func(string) bool) []string {
	if root == "" || len(entries) == 0 {
		return nil
	}
	names := make([]string, 0, len(entries))
	directories := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		rel := filepath.Clean(filepath.FromSlash(entry.Path))
		if rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		name := filepath.Base(rel)
		if strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}
		fullPath := filepath.Join(root, rel)
		switch entry.Kind {
		case CatalogDirectory:
			directories[fullPath] = struct{}{}
			names = append(names, fullPath)
		case CatalogFile:
			if visibleFile == nil || visibleFile(fullPath) {
				names = append(names, fullPath)
			}
		}
	}
	sort.SliceStable(names, func(i, j int) bool {
		_, directoryI := directories[names[i]]
		_, directoryJ := directories[names[j]]
		if directoryI != directoryJ {
			return directoryI
		}
		return strings.ToLower(filepath.Base(names[i])) < strings.ToLower(filepath.Base(names[j]))
	})
	return names
}

// DragProjection contains the immutable visual state for the current tree
// drag. The caller resolves selection and icons; operation kind and label
// come from the file-tree presentation state.
type DragProjection struct {
	ItemName          string
	ItemKind          EntryKind
	ItemIcon          *ui.SVG
	FileIcon          *ui.SVG
	IconTint          ui.Color
	SelectedCount     int
	SelectedFilesOnly bool
	OperationKind     DragOperationKind
	OperationLabel    string
	ImagePath         string
	ImageCacheStore   images.CacheStore
	LocalFiles        images.LocalFiles
	OnImageLoad       func()
}

// RenderDragProjection draws one stable drag card while the virtual list
// replaces rows. All selection and operation semantics arrive as values.
func RenderDragProjection(c *ui.Context, handle *Handle, model DragProjection) {
	var state *state
	if handle != nil {
		state = handle.state
	}
	drag := state.DragPreview()
	if !drag.Active || drag.ItemID == "" {
		return
	}
	w, h := float32(220), float32(50)
	showOperation := drag.Frames > 2 || !drag.StartedAt.IsZero() && c.Now().Sub(drag.StartedAt) >= 120*time.Millisecond || model.OperationKind == DragOperationMoveToTrash
	isImage := model.SelectedCount == 1 && model.ItemKind == File && model.ImagePath != ""
	if isImage {
		w, h = 216, 142
	} else if showOperation {
		w, h = 220, 50
	}
	t := c.Theme()
	card := ui.Row(c).Absolute().Left(drag.Pointer.X+12).Top(drag.Pointer.Y-58).
		Key("tree-drag-session-projection").Label(locales.TreeDragProjectionLabel(drag.ItemID)).
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
				ui.Text(c, model.OperationLabel).Key("tree-drag-projection-operation-" + drag.ItemID).
					Label(locales.TreeDragProjectionOperationLabel(drag.ItemID)).FontSize(10.5).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
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
