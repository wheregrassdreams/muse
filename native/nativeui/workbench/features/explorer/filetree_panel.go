package nativeui

import (
	"path/filepath"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	doctypes "muse/document/types"
	"muse/locales"
	componenticons "muse/nativeui/components/icons"
	"muse/nativeui/workbench/commands"
	"muse/nativeui/workbench/dragdrop"
	filetree "muse/nativeui/workbench/features/explorer"
	vaultfiles "muse/vault/files"
)

// fileTreePanelView supplies catalog and host facts to Explorer and dispatches
// typed effects back into application use cases. Tree rendering and transient
// presentation policy live in the Explorer feature.
func (a *App) fileTreePanelView(c *ui.Context) {
	a.nativeDropHandled = false
	a.treeNativeDropSuggested = transfer.None
	rows := a.fileRows()
	entries := make([]filetree.ModelEntryInput, 0, len(rows))
	for _, row := range rows {
		icon := a.treeIcon(row.Path, row.Directory)
		if !row.Directory && doctypes.IsImagePath(row.Path) && a.treeIconID(row.Path) == "" {
			icon = componenticons.Image()
		}
		entries = append(entries, filetree.ModelEntryInput{
			Row: row, Icon: icon, IconTint: iconTint(c.Theme()),
			Previewable: !row.Directory && doctypes.IsImagePath(row.Path),
		})
	}
	model := filetree.ModelInput{
		Entries: entries, EmptyMessage: locales.FilesNoEditableText(),
		RootLabel: a.root, RootCanDrop: a.root != "",
		NativeDrop: filetree.NativeDropPort{
			Formats:    []transfer.Format{transfer.Format("application/x-muse-vault-item+json")},
			Operations: transfer.Copy | transfer.Move,
		},
	}
	if a.root == "" {
		model.EmptyMessage = ""
		model.EmptyActionLabel = locales.CommonOpenFileOrFolder()
		model.EmptyActions = []filetree.Command{
			{ID: filetree.CommandOpenFile, Label: locales.FilesOpenFileMenu(), Enabled: true},
			{ID: filetree.CommandOpenFolder, Label: locales.FilesOpenFolderMenu(), Enabled: true},
		}
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
	item := a.treeDragItem
	dragProjection := filetree.DragProjection{}
	if item.Path != "" {
		itemIcon := a.treeIcon(item.Path, item.Directory)
		imagePath := ""
		if !item.Directory && doctypes.IsImagePath(item.Path) {
			imagePath = item.Path
			if a.treeIconID(item.Path) == "" {
				itemIcon = componenticons.Image()
			}
		}
		dragProjection = filetree.BuildDragProjection(a.fileTreeState, rows, filetree.DragProjectionInput{
			ItemName: item.Name, ItemDirectory: item.Directory, ItemIcon: itemIcon, FileIcon: componenticons.File(),
			IconTint: iconTint(c.Theme()), ImagePath: imagePath, ImageCacheStore: a.imageCacheStore,
			LocalFiles: a.imageLocalFiles, OnImageLoad: a.invalidate, IsDirectory: a.catalogIsDirectory,
		})
	}
	result := filetree.RenderPanel(c, a.fileTreeState, filetree.PanelInput[dragpayload.VaultItem]{
		Model: model, Drag: drag, DragProjection: dragProjection,
		Focus: &a.treeFocusHandle, FocusRequested: a.treeFocusRequested,
		NativeHandoffPending: a.treeNativeHandoff,
		KeyInput: func(ev ui.InputEvent) bool {
			return ev.Kind == ui.InputKeyDown && !a.anyModalOwnsKeyboard() && a.handleTreeKey(ev.Mods, ev.Key)
		},
		Dispatch: func(action filetree.Action[dragpayload.VaultItem]) {
			if action.Kind == filetree.StartDrag {
				a.treeDragItem = action.Payload
			}
			a.handleFileTreeAction(c, action)
		},
	})
	a.treeFocusRequested = result.FocusRequested
	if result.Focused {
		a.setFocusedPanel(commands.PanelIDFiles)
	}
	if result.NativeOffer != nil {
		a.treeNativeDropSuggested = result.NativeOffer.Suggested
	}
}

func (a *App) handleFileTreeAction(c *ui.Context, action filetree.Action[dragpayload.VaultItem]) {
	switch action.Kind {
	case filetree.SelectEntry, filetree.ToggleEntry:
		a.setFocusedPanel(commands.PanelIDFiles)
	case filetree.OpenEntry:
		a.setFocusedPanel(commands.PanelIDFiles)
		a.selectedPath = action.ID
		a.requestOpen(action.ID)
	case filetree.InvokeCommand:
		switch action.CommandID {
		case filetree.CommandOpenFile:
			a.openFileDialog()
		case filetree.CommandOpenFolder:
			a.openFolderDialog()
		}
	case filetree.InvokeContextCommand:
		a.invokeFileTreeContextCommand(action.ID, action.CommandID)
	case filetree.StartDrag:
		a.treeDragItem = action.Payload
	case filetree.BeginNativeHandoff:
		a.beginTreeNativeHandoffItems(c, a.treeDragItems(action.ItemIDs))
	case filetree.EndDrag:
		a.treeDragItem = dragpayload.VaultItem{}
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
	}
}

func (a *App) invokeFileTreeContextCommand(path string, command filetree.CommandID) {
	if path != "" {
		a.setFocusedPanel(commands.PanelIDFiles)
		a.fileTreeState.SetFocusID(path)
		if !a.fileTreeState.IsSelected(path) {
			a.fileTreeState.SetSelection([]string{path})
		}
	}
	switch command {
	case filetree.CommandCreateFile:
		a.dispatchTreeAction(pathOrRoot(path, a.root), commands.ActionCreateFile, nil)
	case filetree.CommandCreateFolder:
		a.dispatchTreeAction(pathOrRoot(path, a.root), commands.ActionCreateFolder, nil)
	case filetree.CommandPaste:
		a.dispatchTreeAction(pathOrRoot(path, a.root), commands.ActionPaste, nil)
	case filetree.CommandCollapse:
		a.collapseAll()
	case filetree.CommandCopy:
		a.dispatchTreeAction(path, commands.ActionCopy, nil)
	case filetree.CommandCut:
		a.dispatchTreeAction(path, commands.ActionCut, nil)
	case filetree.CommandChangeIcon:
		a.openNavIconPicker(path)
	case filetree.CommandCopyRelative:
		a.dispatchTreeAction(path, commands.ActionCopyPath, map[string]any{"relative": true})
	case filetree.CommandCopyAbsolute:
		a.dispatchTreeAction(path, commands.ActionCopyPath, map[string]any{"relative": false})
	case filetree.CommandRename:
		a.dispatchTreeAction(path, commands.ActionRename, nil)
	case filetree.CommandTrash:
		a.dispatchTreeAction(path, commands.ActionTrash, nil)
	case filetree.CommandReveal:
		a.dispatchTreeAction(path, commands.ActionRevealInFinder, nil)
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
