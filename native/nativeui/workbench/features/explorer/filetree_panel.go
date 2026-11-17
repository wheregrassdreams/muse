package nativeui

import (
	"path/filepath"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"muse/locales"
	doctypes "muse/nativeui/components/doctypes"
	componenticons "muse/nativeui/components/icons"
	"muse/nativeui/interactions"
	"muse/nativeui/interactions/dragpayload"
	"muse/nativeui/panels/filetree"
	vaultfiles "muse/vault/files"
)

const (
	fileTreeCommandCreateFile   = "create-file"
	fileTreeCommandCreateFolder = "create-folder"
	fileTreeCommandPaste        = "paste"
	fileTreeCommandCollapse     = "collapse"
	fileTreeCommandCopy         = "copy"
	fileTreeCommandCut          = "cut"
	fileTreeCommandChangeIcon   = "change-icon"
	fileTreeCommandCopyRelative = "copy-relative"
	fileTreeCommandCopyAbsolute = "copy-absolute"
	fileTreeCommandRename       = "rename"
	fileTreeCommandTrash        = "trash"
	fileTreeCommandReveal       = "reveal"
)

func (a *App) setTreeDragOperationDisplay(operation filetree.DragOperationKind, target string) {
	a.fileTreeState.SetDragOperationDisplay(filetree.DragOperationDisplay{Kind: operation, TargetLabel: target})
}

func (a *App) fileTreePanelView(c *ui.Context) {
	dragStateAtFrameStart := a.fileTreeState.DragPreview()
	wasDragging := dragStateAtFrameStart.Active
	dragStateAtFrameStart.Active = false
	a.fileTreeState.SetDragPreview(dragStateAtFrameStart)
	a.fileTreeState.SetTrashTargetActive(false)
	a.nativeDropHandled = false
	a.treeNativeDropSuggested = transfer.None
	if !wasDragging {
		a.setTreeDragOperationDisplay(filetree.DragOperationMoveToFolder, filepath.Base(a.root))
	}
	if a.root == "" {
		a.emptyWorkspaceFilesView(c)
		return
	}

	rows := a.fileRows()
	entries := make([]filetree.Entry, 0, len(rows))
	for _, row := range rows {
		path := row.Path
		kind := filetree.File
		if row.Directory {
			kind = filetree.Directory
		}
		icon := a.treeIcon(path, row.Directory)
		if !row.Directory && doctypes.IsImagePath(path) && a.treeIconID(path) == "" {
			icon = componenticons.Image()
		}
		_, inDropRange := a.treeDropTargetRange(path, rows)
		entries = append(entries, filetree.Entry{
			ID: path, Label: filepath.Base(path), Kind: kind, Depth: row.Depth,
			CanSelect: true, CanExpand: row.Directory, CanOpen: !row.Directory, CanDrag: true, CanDrop: true,
			DragIDs: a.fileTreeState.SelectedPaths(rows, path), DragActive: wasDragging,
			ContextActions: a.fileTreeContextActions(path, row.Directory), Previewable: !row.Directory && doctypes.IsImagePath(path),
			DropHint: inDropRange, Icon: icon, IconTint: iconTint(c.Theme()),
		})
	}
	a.fileTreeState.SetFocused(a.treeFocusHandle.Focused(c))
	model := filetree.ViewModel{
		Entries:            entries,
		EmptyMessage:       locales.FilesNoEditableText(),
		RootContextActions: a.fileTreeRootContextActions(),
		RootCanDrop:        true,
		NativeDrop: filetree.NativeDropPort{
			Formats:    []transfer.Format{transfer.Format("application/x-muse-vault-item+json")},
			Operations: transfer.Copy | transfer.Move,
		},
	}
	drag := filetree.DragPort[dragpayload.VaultItem]{
		Make: func(entryID string, _ []string) dragpayload.VaultItem {
			entry, ok := a.catalogEntry(entryID)
			return dragpayload.VaultItem{Path: entryID, Name: filepath.Base(entryID), Directory: ok && entry.Kind == vaultfiles.EntryDirectory}
		},
		IDs: func(payload dragpayload.VaultItem) []string {
			if payload.Path == "" {
				return nil
			}
			return []string{payload.Path}
		},
	}
	result := filetree.Render(c, a.fileTreeState, model, drag, func(action filetree.Action[dragpayload.VaultItem]) {
		a.handleFileTreeAction(c, action, rows)
	})
	filetree.RenderDragProjection(c, a.fileTreeState, a.fileTreeDragProjection(c, rows))
	if result.NativeOffer != nil {
		a.treeNativeDropSuggested = result.NativeOffer.Suggested
	}
	result.List.Focusable().Bind(&a.treeFocusHandle).Label(locales.FilesTreeLabel()).HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputKeyDown || a.anyModalOwnsKeyboard() {
			return false
		}
		return a.handleTreeKey(ev.Mods, ev.Key)
	})
	if a.treeFocusRequested {
		a.treeFocusHandle.Focus(c)
		if a.treeFocusHandle.Focused(c) {
			a.treeFocusRequested = false
		}
	}
	if a.treeFocusHandle.Focused(c) {
		a.fileTreeState.SetFocused(true)
		a.setFocusedPanel(interactions.PanelIDFiles)
	}

	dragState := a.fileTreeState.DragPreview()
	if !dragState.StartedAt.IsZero() && !a.fileTreeState.PointerReleased() {
		if x, y, over := c.Root().PointerPosition(); over {
			dragState.Pointer = ui.Point{X: x, Y: y}
			a.fileTreeState.SetDragPreview(dragState)
		} else if !a.treeNativeHandoff {
			a.beginTreeNativeHandoffItems(c, a.treeDragItems(a.fileTreeState.SelectedIDs()))
		}
	}
	previewID := a.fileTreeState.PreviewID()
	if previewID != "" && result.HoveredID == previewID {
		a.fileTreeState.CancelPreviewLeave()
	} else if previewID != "" && !a.fileTreeState.PreviewPopupHovered() {
		if a.fileTreeState.PreviewLeaveAt().IsZero() {
			a.fileTreeState.BeginPreviewLeave(c.Now(), 220*time.Millisecond)
		}
		if a.fileTreeState.PreviewLeaveExpired(c.Now()) {
			a.fileTreeState.ClearPreview()
		} else {
			if deadline := a.fileTreeState.PreviewLeaveAt(); !deadline.IsZero() {
				c.After(deadline.Sub(c.Now()))
			}
		}
	}
	if !a.fileTreeState.DragPreview().Active && !a.treeNativeHandoff && (a.fileTreeState.PointerReleased() || (!wasDragging && a.fileTreeState.DragPreview().StartedAt.IsZero())) {
		a.fileTreeState.SetDragPreview(filetree.DragPreviewState{})
		a.treeDragItem = dragpayload.VaultItem{}
		a.fileTreeState.SetPointerReleased(false)
		a.fileTreeState.ClearPointerPress()
	}
	a.fileTreeState.SetTrashTargetActive(a.fileTreeState.DragPreview().Active || wasDragging)
}

func (a *App) fileTreeDragProjection(c *ui.Context, rows []filetree.Row) filetree.DragProjection {
	item := a.treeDragItem
	if item.Path == "" {
		return filetree.DragProjection{}
	}
	selectedIDs := a.fileTreeState.SelectedIDs()
	selectedCount := len(selectedIDs)
	selectedFilesOnly := selectedCount > 0
	visible := make(map[string]filetree.Row, len(rows))
	for _, row := range rows {
		visible[row.Path] = row
	}
	for _, path := range selectedIDs {
		if row, ok := visible[path]; ok {
			if row.Directory {
				selectedFilesOnly = false
				break
			}
			continue
		}
		entry, ok := a.catalogEntry(path)
		if !ok || entry.Kind != vaultfiles.EntryRegular {
			selectedFilesOnly = false
			break
		}
	}
	icon := a.treeIcon(item.Path, item.Directory)
	imagePath := ""
	if selectedCount == 1 && !item.Directory && doctypes.IsImagePath(item.Path) {
		imagePath = item.Path
		if a.treeIconID(item.Path) == "" {
			icon = componenticons.Image()
		}
	}
	itemKind := filetree.File
	if item.Directory {
		itemKind = filetree.Directory
	}
	return filetree.DragProjection{
		ItemName: item.Name, ItemKind: itemKind, ItemIcon: icon, FileIcon: componenticons.File(),
		IconTint: iconTint(c.Theme()), SelectedCount: selectedCount, SelectedFilesOnly: selectedFilesOnly,
		OperationKind:  a.fileTreeState.DragOperationDisplay().Kind,
		OperationLabel: a.fileTreeState.DragOperationDisplay().Label(),
		ImagePath:      imagePath, ImageCacheStore: a.imageCacheStore, LocalFiles: a.imageLocalFiles, OnImageLoad: a.invalidate,
	}
}

func (a *App) emptyWorkspaceFilesView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(10).Padding(20).Children(func() {
		ui.Icon(c, componenticons.FolderOpen()).Size(30, 30).TextColor(iconTint(t))
		open := ui.Button(c, locales.CommonOpenFileOrFolder())
		open.Menu(func(m *ui.Menu) {
			if m.Item(locales.FilesOpenFileMenu()).Chosen() {
				a.openFileDialog()
			}
			if m.Item(locales.FilesOpenFolderMenu()).Chosen() {
				a.openFolderDialog()
			}
		})
	})
}

func (a *App) fileTreeRootContextActions() []filetree.Command {
	commands := []filetree.Command{
		{ID: fileTreeCommandCreateFile, Label: locales.FilesNewFile(), Enabled: true},
		{ID: fileTreeCommandCreateFolder, Label: locales.FilesNewFolderMenu(), Enabled: true},
	}
	if len(a.fileTreeState.Clipboard().IDs) > 0 {
		commands = append(commands, filetree.Command{ID: fileTreeCommandPaste, Label: locales.FilesPasteItem(), Enabled: true})
	}
	return append(commands, filetree.Command{ID: fileTreeCommandCollapse, Label: locales.FilesCollapseMenu(), Enabled: true})
}

func (a *App) fileTreeContextActions(path string, isDir bool) []filetree.Command {
	commands := []filetree.Command{{ID: fileTreeCommandCollapse, Label: locales.FilesCollapseMenu(), Enabled: true}}
	if isDir {
		commands = append(commands,
			filetree.Command{ID: fileTreeCommandCreateFile, Label: locales.FilesNewFile(), Enabled: true},
			filetree.Command{ID: fileTreeCommandCreateFolder, Label: locales.FilesNewFolderMenu(), Enabled: true},
		)
	}
	commands = append(commands,
		filetree.Command{ID: fileTreeCommandCopy, Label: locales.FilesCopyItem(), Enabled: true},
		filetree.Command{ID: fileTreeCommandCut, Label: locales.FilesCutItem(), Enabled: true},
	)
	if len(a.fileTreeState.Clipboard().IDs) > 0 {
		commands = append(commands, filetree.Command{ID: fileTreeCommandPaste, Label: locales.FilesPasteItem(), Enabled: true})
	}
	commands = append(commands,
		filetree.Command{ID: fileTreeCommandChangeIcon, Label: locales.FilesChangeIcon(), Enabled: true},
		filetree.Command{ID: fileTreeCommandCopyRelative, Label: locales.FilesCopyRelativePath(), Enabled: true},
		filetree.Command{ID: fileTreeCommandCopyAbsolute, Label: locales.FilesCopyAbsolutePath(), Enabled: true},
		filetree.Command{ID: fileTreeCommandRename, Label: locales.FilesRenameEllipsis(), Enabled: true},
		filetree.Command{ID: fileTreeCommandTrash, Label: locales.FilesMoveToTrash(), Enabled: true},
		filetree.Command{ID: fileTreeCommandReveal, Label: locales.FilesRevealInFinder(), Enabled: true},
	)
	return commands
}

func (a *App) handleFileTreeAction(c *ui.Context, action filetree.Action[dragpayload.VaultItem], rows []filetree.Row) {
	switch action.Kind {
	case filetree.SelectEntry:
		a.setFocusedPanel(interactions.PanelIDFiles)
	case filetree.ToggleEntry:
		a.setFocusedPanel(interactions.PanelIDFiles)
	case filetree.OpenEntry:
		a.setFocusedPanel(interactions.PanelIDFiles)
		a.selectedPath = action.ID
		a.requestOpen(action.ID)
	case filetree.InvokeContextCommand:
		a.invokeFileTreeContextCommand(action.ID, action.CommandID)
	case filetree.StartDrag:
		a.treeDragItem = action.Payload
		drag := a.fileTreeState.DragPreview()
		drag.Active = true
		drag.Frames++
		drag.ItemID = action.ID
		if drag.StartedAt.IsZero() {
			drag.StartedAt = c.Now()
		}
		a.fileTreeState.SetDragPreview(drag)
	case filetree.DropEntries:
		destination := a.root
		if action.ID != "" {
			destination = a.treeDropDestination(action.ID)
		}
		a.moveTreeItems(a.treeDraggedPaths(firstPath(action.ItemIDs)), destination)
	case filetree.DropNative:
		destination := a.root
		if action.ID != "" {
			destination = a.treeDropDestination(action.ID)
		}
		a.applyTreeNativeDrop(action.Drop, destination)
	case filetree.ImportFiles:
		destination := a.root
		if action.ID != "" {
			destination = a.treeDropDestination(action.ID)
		}
		a.importDroppedInto(action.Paths, destination)
	case filetree.HoverEntry:
	case filetree.LeaveEntry:
		if a.fileTreeState.PreviewLeaveAt().IsZero() {
			a.fileTreeState.BeginPreviewLeave(c.Now(), 220*time.Millisecond)
		}
	}
}

func (a *App) invokeFileTreeContextCommand(path, command string) {
	if path != "" {
		a.setFocusedPanel(interactions.PanelIDFiles)
		a.fileTreeState.SetFocusID(path)
		if !a.fileTreeState.IsSelected(path) {
			a.fileTreeState.SetSelection([]string{path})
		}
	}
	switch command {
	case fileTreeCommandCreateFile:
		a.dispatchTreeAction(pathOrRoot(path, a.root), interactions.ActionCreateFile, nil)
	case fileTreeCommandCreateFolder:
		a.dispatchTreeAction(pathOrRoot(path, a.root), interactions.ActionCreateFolder, nil)
	case fileTreeCommandPaste:
		a.dispatchTreeAction(pathOrRoot(path, a.root), interactions.ActionPaste, nil)
	case fileTreeCommandCollapse:
		a.collapseAll()
	case fileTreeCommandCopy:
		a.dispatchTreeAction(path, interactions.ActionCopy, nil)
	case fileTreeCommandCut:
		a.dispatchTreeAction(path, interactions.ActionCut, nil)
	case fileTreeCommandChangeIcon:
		a.openNavIconPicker(path)
	case fileTreeCommandCopyRelative:
		a.dispatchTreeAction(path, interactions.ActionCopyPath, map[string]any{"relative": true})
	case fileTreeCommandCopyAbsolute:
		a.dispatchTreeAction(path, interactions.ActionCopyPath, map[string]any{"relative": false})
	case fileTreeCommandRename:
		a.dispatchTreeAction(path, interactions.ActionRename, nil)
	case fileTreeCommandTrash:
		a.dispatchTreeAction(path, interactions.ActionTrash, nil)
	case fileTreeCommandReveal:
		a.dispatchTreeAction(path, interactions.ActionRevealInFinder, nil)
	}
}

func (a *App) treeDropDestination(path string) string {
	return a.vaultFileUseCases().DestinationForPath(path)
}

func (a *App) treeDragItems(paths []string) []dragpayload.VaultItem {
	items := make([]dragpayload.VaultItem, 0, len(paths))
	for _, path := range paths {
		entry, ok := a.catalogEntry(path)
		if ok {
			items = append(items, dragpayload.VaultItem{Path: path, Name: filepath.Base(path), Directory: entry.Kind == vaultfiles.EntryDirectory})
		}
	}
	return items
}

func (a *App) treeDropTargetRange(path string, rows []filetree.Row) (int, bool) {
	rangeHint := a.fileTreeState.DropTargetRange()
	for i, row := range rows {
		if row.Path == path {
			return i, rangeHint.TargetID != "" && i >= rangeHint.StartRow && i < rangeHint.EndRow && !a.fileTreeState.IsSelected(path)
		}
	}
	return 0, false
}

func firstPath(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}

func pathOrRoot(path, root string) string {
	if path == "" {
		return root
	}
	return path
}
