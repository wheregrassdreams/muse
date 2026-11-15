package nativeui

import (
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	mdedit "muse/editor"
	"muse/locales"
	doctypes "muse/nativeui/components/doctypes"
	"muse/nativeui/panels/filetree"
)

const (
	treeDragMoveToFolder     = "move-to-folder"
	treeDragMoveToTrash      = "move-to-trash"
	treeDragInsertIntoEditor = "insert-into-editor"

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

type treeDropTarget struct {
	path       string
	start, end int
}

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

func (a *App) fileTreePanelView(c *ui.Context) {
	wasDragging := a.treeDragging
	a.treeDragging = false
	a.treeTrashTargetActive = false
	a.nativeDropHandled = false
	a.treeNativeDropSuggested = transfer.None
	if !wasDragging {
		a.setTreeDragMoveOperation(treeDragMoveToFolder, filepath.Base(a.root))
	}
	if a.root == "" {
		a.emptyWorkspaceFilesView(c)
		return
	}

	rows := a.fileRows()
	entries := make([]filetree.Entry, 0, len(rows))
	for _, row := range rows {
		path := row.path
		kind := filetree.File
		if row.dir {
			kind = filetree.Directory
		}
		icon := a.treeIcon(path, row.dir)
		if !row.dir && doctypes.IsImagePath(path) && a.treeIconID(path) == "" {
			icon = iconImage
		}
		selected := path == a.selectedPath || a.treeSelection[path]
		_, inDropRange := a.treeDropTargetRange(path, rows)
		entries = append(entries, filetree.Entry{
			ID: path, Label: filepath.Base(path), Kind: kind, Depth: row.depth,
			Expanded: a.expanded[path], Selected: selected, Focused: path == a.treeFocusPath,
			CanSelect: true, CanExpand: row.dir, CanOpen: !row.dir, CanDrag: true, CanDrop: true,
			DragIDs: a.selectedTreePaths(path), DragActive: wasDragging,
			ContextActions: a.fileTreeContextActions(path, row.dir), Previewable: !row.dir && doctypes.IsImagePath(path),
			DropHint: inDropRange, Icon: icon, IconTint: iconTint(c.Theme()),
		})
	}
	model := filetree.ViewModel{
		Entries:            entries,
		EmptyMessage:       locales.FilesNoEditableText(),
		Focused:            a.treeFocusHandle.Focused(c),
		PreviewID:          a.treePreviewPath,
		RootContextActions: a.fileTreeRootContextActions(),
		RootCanDrop:        true,
		NativeDrop: filetree.NativeDropPort{
			Formats:    []transfer.Format{transfer.Format("application/x-muse-vault-item+json")},
			Operations: transfer.Copy | transfer.Move,
		},
		Scroll: filetree.ScrollPort{List: &a.fileList, State: &a.fileScroll, Indicator: &a.fileIndicator},
	}
	drag := filetree.DragPort[mdedit.VaultItemDrag]{
		Make: func(entryID string, _ []string) mdedit.VaultItemDrag {
			info, err := os.Stat(entryID)
			return mdedit.VaultItemDrag{Path: entryID, Name: filepath.Base(entryID), Directory: err == nil && info.IsDir()}
		},
		IDs: func(payload mdedit.VaultItemDrag) []string {
			if payload.Path == "" {
				return nil
			}
			return []string{payload.Path}
		},
	}
	result := filetree.Render(c, model, drag, func(action filetree.Action[mdedit.VaultItemDrag]) {
		a.handleFileTreeAction(c, action, rows)
	})
	filetree.RenderDragProjection(c, a.fileTreeDragProjection(c, rows, wasDragging))
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
		a.setFocusedPanel(PanelIDFiles)
	}

	if !a.treeDragStarted.IsZero() && !a.treePointerReleased {
		if x, y, over := c.Root().PointerPosition(); over {
			a.treeDragPointer = ui.Point{X: x, Y: y}
			a.treeDragPointerValid = true
		} else if !a.treeNativeHandoff {
			a.beginTreeNativeHandoffItems(c, a.treeDragItems(a.treeDragSelection))
		}
	}
	if a.treePreviewPath != "" && result.HoveredID != a.treePreviewPath && !a.treePreviewPopupHovered {
		if a.treePreviewLeave.IsZero() {
			a.treePreviewLeave = c.Now().Add(220 * time.Millisecond)
		}
		if remaining := a.treePreviewLeave.Sub(c.Now()); remaining <= 0 {
			a.treePreviewPath = ""
		} else {
			c.After(remaining)
		}
	}
	if !a.treeDragging && !a.treeNativeHandoff && (a.treePointerReleased || (!wasDragging && a.treeDragStarted.IsZero())) {
		a.treeDragStarted = time.Time{}
		a.treeDragFrames = 0
		a.treeDragSelection = nil
		a.treeDragItem = mdedit.VaultItemDrag{}
		a.treeDragPointerValid = false
		a.treePointerReleased = false
		a.treePressPath = ""
		a.treePressMods = 0
		a.treePressApplied = false
	}
	a.treeTrashTargetActive = a.treeDragging || wasDragging
}

func (a *App) fileTreeDragProjection(c *ui.Context, rows []fileRow, wasDragging bool) filetree.DragProjection {
	item := a.treeDragItem
	if item.Path == "" {
		return filetree.DragProjection{}
	}
	itemID := filepath.ToSlash(item.Path)
	if rel, err := filepath.Rel(a.root, item.Path); err == nil {
		itemID = filepath.ToSlash(rel)
	}
	selectedCount := len(a.treeDragSelection)
	selectedFilesOnly := selectedCount > 0
	visible := make(map[string]fileRow, len(rows))
	for _, row := range rows {
		visible[row.path] = row
	}
	for _, path := range a.treeDragSelection {
		if row, ok := visible[path]; ok {
			if row.dir {
				selectedFilesOnly = false
				break
			}
			continue
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			selectedFilesOnly = false
			break
		}
	}
	icon := a.treeIcon(item.Path, item.Directory)
	imagePath := ""
	if selectedCount == 1 && !item.Directory && doctypes.IsImagePath(item.Path) {
		imagePath = item.Path
		if a.treeIconID(item.Path) == "" {
			icon = iconImage
		}
	}
	itemKind := filetree.File
	if item.Directory {
		itemKind = filetree.Directory
	}
	return filetree.DragProjection{
		Active: wasDragging || a.treeDragging, StartedAt: a.treeDragStarted, Now: c.Now(),
		Frames: a.treeDragFrames, Pointer: a.treeDragPointer, ItemID: itemID,
		ItemName: item.Name, ItemKind: itemKind, ItemIcon: icon, FileIcon: iconFile,
		IconTint: iconTint(c.Theme()), SelectedCount: selectedCount, SelectedFilesOnly: selectedFilesOnly,
		OperationLabel: a.treeDragOperationLabel(), OperationID: a.treeDragOperation,
		ImagePath: imagePath, ImageCacheStore: a.imageCacheStore, OnImageLoad: a.invalidate,
	}
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
	return len(paths), allFiles
}

func (a *App) emptyWorkspaceFilesView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(10).Padding(20).Children(func() {
		ui.Icon(c, iconFolderOpen).Size(30, 30).TextColor(iconTint(t))
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
	if len(a.treeClipboard) > 0 {
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
	if len(a.treeClipboard) > 0 {
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

func (a *App) handleFileTreeAction(c *ui.Context, action filetree.Action[mdedit.VaultItemDrag], rows []fileRow) {
	switch action.Kind {
	case filetree.SelectEntry:
		a.setFocusedPanel(PanelIDFiles)
		a.treeFocusPath = action.ID
		a.selectTreeRow(action.ID, action.Modifiers, rows)
	case filetree.ToggleEntry:
		a.setFocusedPanel(PanelIDFiles)
		a.treeFocusPath = action.ID
		a.expanded[action.ID] = !a.expanded[action.ID]
		a.fileRowsDirty = true
	case filetree.OpenEntry:
		a.treeFocusPath = action.ID
		a.setFocusedPanel(PanelIDFiles)
		a.selectedPath = action.ID
		a.requestOpen(action.ID)
	case filetree.InvokeContextCommand:
		a.invokeFileTreeContextCommand(action.ID, action.CommandID)
	case filetree.StartDrag:
		a.treeDragging = true
		a.treeDragFrames++
		a.treeDragSelection = append(a.treeDragSelection[:0], action.ItemIDs...)
		if len(action.ItemIDs) > 0 {
			if info, err := os.Stat(action.ItemIDs[0]); err == nil {
				a.treeDragItem = mdedit.VaultItemDrag{Path: action.ItemIDs[0], Name: filepath.Base(action.ItemIDs[0]), Directory: info.IsDir()}
			}
		}
		a.treeDragItem = action.Payload
		if a.treeDragStarted.IsZero() {
			a.treeDragStarted = c.Now()
		}
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
		a.treePreviewPath, a.treePreviewRect = action.ID, action.Bounds
		a.treePreviewLeave = time.Time{}
	case filetree.LeaveEntry:
		if a.treePreviewPath == action.ID {
			a.treePreviewLeave = c.Now().Add(220 * time.Millisecond)
		}
	}
}

func (a *App) invokeFileTreeContextCommand(path, command string) {
	if path != "" {
		a.setFocusedPanel(PanelIDFiles)
		a.treeFocusPath = path
		if a.treeSelection == nil {
			a.treeSelection = make(map[string]bool)
		}
		if !a.treeSelection[path] {
			clear(a.treeSelection)
			a.treeSelection[path] = true
			a.treeSelectionAnchor = path
		}
	}
	switch command {
	case fileTreeCommandCreateFile:
		a.dispatchTreeAction(pathOrRoot(path, a.root), ActionCreateFile, nil)
	case fileTreeCommandCreateFolder:
		a.dispatchTreeAction(pathOrRoot(path, a.root), ActionCreateFolder, nil)
	case fileTreeCommandPaste:
		a.dispatchTreeAction(pathOrRoot(path, a.root), ActionPaste, nil)
	case fileTreeCommandCollapse:
		a.collapseAll()
	case fileTreeCommandCopy:
		a.dispatchTreeAction(path, ActionCopy, nil)
	case fileTreeCommandCut:
		a.dispatchTreeAction(path, ActionCut, nil)
	case fileTreeCommandChangeIcon:
		a.openNavIconPicker(path)
	case fileTreeCommandCopyRelative:
		a.dispatchTreeAction(path, ActionCopyPath, map[string]any{"relative": true})
	case fileTreeCommandCopyAbsolute:
		a.dispatchTreeAction(path, ActionCopyPath, map[string]any{"relative": false})
	case fileTreeCommandRename:
		a.dispatchTreeAction(path, ActionRename, nil)
	case fileTreeCommandTrash:
		a.dispatchTreeAction(path, ActionTrash, nil)
	case fileTreeCommandReveal:
		a.dispatchTreeAction(path, ActionRevealInFinder, nil)
	}
}

func (a *App) treeDropDestination(path string) string {
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return path
	}
	return filepath.Dir(path)
}

func (a *App) treeDragItems(paths []string) []mdedit.VaultItemDrag {
	items := make([]mdedit.VaultItemDrag, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err == nil {
			items = append(items, mdedit.VaultItemDrag{Path: path, Name: filepath.Base(path), Directory: info.IsDir()})
		}
	}
	return items
}

func (a *App) treeDropTargetRange(path string, rows []fileRow) (int, bool) {
	for i, row := range rows {
		if row.path == path {
			return i, a.treeDropTarget.path != "" && i >= a.treeDropTarget.start && i < a.treeDropTarget.end && !a.treeSelection[path]
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
