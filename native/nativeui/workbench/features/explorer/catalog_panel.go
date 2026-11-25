package explorer

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"muse/locales"
	componenticons "muse/nativeui/workbench/framework/components/icons"
)

// CatalogPanelPorts are the detached catalog facts and workbench capabilities
// needed to build one Explorer panel frame. The feature owns row flattening,
// icon projection and panel rendering; the application supplies catalog data.
type CatalogPanelPorts struct {
	Root        string
	RootLabel   string
	ActivePath  string
	Loading     bool
	LoadError   string
	Revision    uint64
	Children    func(directory string) []CatalogEntry
	VisibleFile func(path string) bool
	IsDirectory func(path string) bool
	Icon        func(path string, directory bool) *ui.SVG
	Controller  *Controller
	Focus       *ui.Handle
	Drag        DragPort
	DragPreview *DragSnapshot
	OnAction    func(Action)
}

type SurfacePorts struct {
	CatalogPanelPorts
	OnNewDocument func()
	OnOpenFile    func()
	OnOpenFolder  func()
}

// RenderSurface selects Explorer's empty-vault landing actions or its normal
// catalog-backed tree. It owns this feature-level view transition.
func RenderSurface(c *ui.Context, ports SurfacePorts) {
	if ports.Root == "" {
		ui.Column(c).Fill().Center().Gap(10).Padding(20).Children(func() {
			ui.Icon(c, componenticons.FolderOpen()).Size(30, 30).TextColor(c.Theme().TextMuted)
			open := ui.Button(c, locales.CommonOpenFileOrFolder())
			open.Menu(func(menu *ui.Menu) {
				if menu.Item(locales.FilesOpenFileMenu()).Chosen() && ports.OnOpenFile != nil {
					ports.OnOpenFile()
				}
				if menu.Item(locales.FilesOpenFolderMenu()).Chosen() && ports.OnOpenFolder != nil {
					ports.OnOpenFolder()
				}
			})
		})
		return
	}
	if ports.RootLabel == "" {
		ports.RootLabel = RootLabel(ports.Root)
	}
	RenderCatalogPanel(c, ports.CatalogPanelPorts)
}

func RootLabel(root string) string {
	label := filepath.Base(root)
	if label == "." || label == string(filepath.Separator) {
		return root
	}
	return label
}

func RenderCatalogPanel(c *ui.Context, ports CatalogPanelPorts) {
	if ports.Controller == nil {
		return
	}
	rows := ports.Controller.Rows(ports.Root, CatalogPort{
		Children: ports.Children, VisibleFile: ports.VisibleFile,
		Pending: ports.Loading || ports.LoadError != "", Revision: ports.Revision,
	})
	entries := make([]EntrySnapshot, 0, len(rows))
	iconTint := c.Theme().TextMuted
	for _, row := range rows {
		icon := componenticons.File()
		if row.Directory {
			icon = componenticons.Folder()
		}
		if ports.Icon != nil {
			if custom := ports.Icon(row.Path, row.Directory); custom != nil {
				icon = custom
			}
		}
		entries = append(entries, EntrySnapshot{Row: row, Icon: icon, IconTint: iconTint})
	}
	label := ports.RootLabel
	if label == "" {
		label = ports.Root
	}
	emptyMessage := "No files"
	if ports.Loading {
		emptyMessage = locales.FilesLoading()
	} else if ports.LoadError != "" {
		emptyMessage = ports.LoadError
	}
	ports.Controller.RenderPanel(c, PanelSnapshot{
		Entries: entries, RootLabel: label, ActivePath: ports.ActivePath, RootCanDrop: true,
		EmptyMessage: emptyMessage, Drag: ports.DragPreview,
	}, PanelCapabilities{
		Focus: ports.Focus, Drag: ports.Drag, IsDirectory: ports.IsDirectory, OnAction: ports.OnAction,
	})
}
