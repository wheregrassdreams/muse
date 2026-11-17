// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

// filesView draws the workspace tree. Directories open lazily; the list builds
// only the rows in view, so a large repository is still quick.
func (a *App) filesView(c *ui.Context) {
	a.fileTreePanelView(c)
}

// treeTrashDropTarget covers the sidebar's bottom content while an in-window
// tree drag is active. Its inset keeps it off the panel edges and its weak
// default colors make it read as a drop affordance rather than a button.
func (a *App) treeTrashDropTarget(c *ui.Context) {
	t := c.Theme()
	// Leave the fixed footer (settings gear) outside the drop hit area. The
	// 48 DIP bottom offset is the footer's 38 DIP height plus the usual 10 DIP
	// visual gap.
	trash := ui.Box(c).Absolute().Left(10).Right(10).Bottom(48).Height(64).
		Key("tree-trash-drop-target").Label(locales.TreeTrashDropTarget()).Radius(12).
		Background(t.Background.Alpha(0.04)).Border(1, t.Border.Alpha(0.30)).BorderStyle(ui.BorderDashed).
		Padding(0, 12).AlignItems(ui.Center).Justify(ui.Center)
	_, over := ui.DragOver[dragpayload.VaultItem](trash)
	if over {
		a.setTreeDragOperationDisplay(filetree.DragOperationMoveToTrash, "")
		trash.Background(ui.RGBA(128, 66, 72, 0.10)).BorderColor(t.Border.Alpha(0.48))
	}
	if dropped, ok := ui.Drop[dragpayload.VaultItem](trash); ok {
		a.trashTreeItems(a.treeDraggedPaths(dropped.Path))
	}
	trash.Children(func() {
		ui.Column(c).Width(110).FillHeight().AlignItems(ui.Center).Justify(ui.Center).Gap(4).
			Label(locales.TreeTrashHintColumn()).Children(func() {
			glyph := componenticons.Trash()
			iconColor := t.TextMuted.Alpha(0.18)
			labelColor := t.TextMuted.Alpha(0.23)
			if over {
				glyph = componenticons.TrashOpen()
				iconColor = ui.Hex("#d98b92").Alpha(0.72)
				labelColor = t.Text.Alpha(0.68)
			}
			trashGlyph := ui.Icon(c, glyph).Size(18, 18).TextColor(iconColor)
			if over {
				trashGlyph.Label(locales.TreeTrashHoverIcon())
			} else {
				ui.Text(c, locales.FilesMoveToTrash()).Label(locales.TreeTrashHint()).FontSize(12).TextColor(labelColor)
			}
		})
	})
}

func (a *App) setTreeRootDropTarget(rows []filetree.Row) {
	a.fileTreeState.SetDropTargetRange(a.root, 0, len(rows))
	a.setTreeDragOperationDisplay(filetree.DragOperationMoveToFolder, filepath.Base(a.root))
}

func (a *App) importDroppedInto(paths []string, destination string) {
	_, err := a.vaultFileUseCases().ImportFiles(paths, destination)
	if err != nil {
		a.notice(locales.CommonImportFailed(err.Error()))
	}
	if len(paths) > 0 {
		a.clearTreeCache()
		a.invalidate()
	}
}

func (a *App) beginTreeNativeHandoffItems(c *ui.Context, items []dragpayload.VaultItem) {
	if a.treeNativeHandoff || len(items) == 0 || items[0].Path == "" {
		return
	}
	data, err := treeItemsTransferData(items)
	if err != nil {
		a.notice(locales.CommonMoveAcrossWindowsFailed(err.Error()))
		return
	}
	sourcePath := items[0].Path
	a.treeNativeHandoff = true
	opts := a.treeNativeDragOptions(sourcePath)
	if err := handoffTreeNativeDrag(c, data, opts); err != nil && a.treeNativeHandoff {
		a.treeNativeHandoff = false
		a.notice(locales.CommonMoveAcrossWindowsFailed(err.Error()))
	}
}

func (a *App) treeNativeDragOptions(sourcePath string) mygo.DragOptions {
	return mygo.DragOptions{
		Operations: transfer.Copy | transfer.Move,
		Done: func(result transfer.Result) {
			a.treeNativeHandoff = false
			if a.fileTreeState != nil {
				a.fileTreeState.SetSelection(nil)
				a.fileTreeState.ClearPointerPress()
				a.fileTreeState.SetPointerReleased(true)
			}
			a.fileTreeState.SetDragPreview(filetree.DragPreviewState{})
			a.treeDragItem = dragpayload.VaultItem{}
			if result.Err == nil && result.Operation != transfer.None && a.vaultFileUseCases().OwnsPath(sourcePath) {
				// The receiver owns any filesystem move. Refresh the source vault
				// snapshot as well as the Explorer projection so the next listing
				// reads current catalog data after an accepted native drop.
				if a.vault != nil {
					if err := a.vault.Refresh(); err != nil {
						a.notice(locales.WorkspaceIndexRefreshFailed(err.Error()))
					}
				}
				a.clearTreeCache()
			}
			if result.Err != nil {
				a.notice(locales.CommonWindowDragFailed(result.Err.Error()))
			}
			a.invalidate()
		},
	}
}

// applyTreeNativeDrop handles Muse's cross-window payload before the platform
// file-list fallback. The native source never removes its item in Done; the
// receiving window performs a same-vault Move exactly once here.
func (a *App) applyTreeNativeDrop(drop transfer.Drop, destination string) {
	b, err := drop.Data.Read(transfer.Format("application/x-muse-vault-item+json"))
	if err != nil {
		return
	}
	var payload struct {
		Path      string                  `json:"path"`
		Name      string                  `json:"name"`
		Directory bool                    `json:"directory"`
		Items     []dragpayload.VaultItem `json:"items"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		a.notice(locales.FilesReadDropFailed())
		return
	}
	items := payload.Items
	if len(items) == 0 && payload.Path != "" {
		items = []dragpayload.VaultItem{{Path: payload.Path, Name: payload.Name, Directory: payload.Directory}}
	}
	if len(items) == 0 {
		a.notice(locales.FilesReadDropFailed())
		return
	}
	// Native negotiation defaults to Copy when no modifier is suggested.
	// Muse's same-vault tree semantics are Move unless the user explicitly
	// requested Copy in the native drag session.
	move := drop.Operation == transfer.Move || (drop.Operation == transfer.Copy && a.treeNativeDropSuggested != transfer.Copy)
	paths := make([]string, 0, len(items))
	for _, item := range items {
		if item.Path != "" {
			paths = append(paths, item.Path)
		}
	}
	intent := applicationvault.DropCopy
	if move {
		intent = applicationvault.DropMove
	}
	a.applyVaultDrop(paths, destination, intent)
}

// moveTreeItem moves a dragged item into a directory in the workspace.
// Existing names are preserved; collisions get a numbered suffix.
func (a *App) moveTreeItem(source, destination string) {
	a.moveTreeItems([]string{source}, destination)
}

func (a *App) moveTreeItems(paths []string, destination string) {
	mutation, err := a.vaultFileUseCases().MoveFiles(paths, destination)
	moves := mutation.Moves
	if len(moves) > 0 {
		a.remapTreePaths(moves)
		a.clearTreeCache()
		a.notice(locales.FilesMovedToFolder(filepath.Base(destination)))
		a.invalidate()
	}
	if err != nil {
		a.notice(locales.CommonMoveFailed(err.Error()))
	}
}

func (a *App) remapTreePaths(moves []applicationvault.FileMove) {
	remap := func(path string) string {
		for _, move := range moves {
			if rel, err := filepath.Rel(move.Source, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				path = filepath.Join(move.Target, rel)
			}
		}
		return path
	}
	if next := remap(a.documentPath()); next != a.documentPath() {
		if note, err := a.readDocument(next); err == nil && a.noteEditor() != nil {
			_ = a.noteEditor().Relocate(note)
		}
	}
	a.selectedPath = remap(a.selectedPath)
	a.fileTreeState.RemapIDs(remap)
	if a.treeDragItem.Path != "" {
		a.treeDragItem.Path = remap(a.treeDragItem.Path)
	}
}

func (a *App) fileRows() []filetree.Row {
	if a == nil || a.fileTreeState == nil {
		return nil
	}
	return filetree.BuildRows(a.fileTreeState, a.root, filetree.CatalogPort{
		Children:    a.treeCatalogChildren,
		VisibleFile: doctypes.IsVisibleWorkspacePath,
	})
}

func (a *App) selectTreeRow(path string, mods ui.Modifiers, rows []filetree.Row) {
	entries := make([]filetree.Entry, len(rows))
	for i, row := range rows {
		entries[i].ID = row.Path
	}
	a.fileTreeState.SelectID(path, mods, entries)
}

func (a *App) treeDraggedPaths(primary string) []string {
	var paths []string
	if a.fileTreeState.IsSelected(primary) {
		paths = a.fileTreeState.SelectedPaths(a.fileRows(), primary)
	} else if primary != "" {
		paths = []string{primary}
	}
	return a.vaultFileUseCases().NormalizeTargets(paths)
}

func (a *App) children(dir string) []string {
	if a == nil || a.root == "" {
		return nil
	}
	return filetree.ProjectChildren(a.root, a.treeCatalogChildren(dir), doctypes.IsVisibleWorkspacePath)
}

// treeCatalogChildren adapts the active vault catalog to the explorer's
// narrow, backend-neutral catalog facts. Filtering, ordering, and flattening
// remain owned by the explorer feature.
func (a *App) treeCatalogChildren(dir string) []filetree.CatalogEntry {
	if a == nil || a.root == "" || a.vault == nil || a.directoryCatalog == nil || filepath.Clean(a.vaultSnapshot.Root) != filepath.Clean(a.root) {
		return nil
	}
	rel, err := filepath.Rel(a.root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil
	}
	entries := a.directoryCatalog.Children(a.vault, filepath.ToSlash(rel))
	children := make([]filetree.CatalogEntry, 0, len(entries))
	for _, entry := range entries {
		switch entry.Kind {
		case vaultfiles.EntryDirectory:
			children = append(children, filetree.CatalogEntry{Path: entry.Path, Kind: filetree.CatalogDirectory})
		case vaultfiles.EntryRegular:
			children = append(children, filetree.CatalogEntry{Path: entry.Path, Kind: filetree.CatalogFile})
		}
	}
	return children
}

func (a *App) catalogEntry(path string) (vaultfiles.Entry, bool) {
	if a == nil || a.vault == nil || a.root == "" || filepath.Clean(a.vaultSnapshot.Root) != filepath.Clean(a.root) {
		return vaultfiles.Entry{}, false
	}
	rel, err := filepath.Rel(a.root, path)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return vaultfiles.Entry{}, false
	}
	return a.vault.Entry(filepath.ToSlash(rel))
}

func (a *App) catalogIsDirectory(path string) bool {
	entry, ok := a.catalogEntry(path)
	return ok && entry.Kind == vaultfiles.EntryDirectory
}

func (a *App) pathKind(path string) (vaultfiles.EntryKind, error) {
	if entry, ok := a.catalogEntry(path); ok {
		return entry.Kind, nil
	}
	if a == nil || a.fileOperations == nil {
		return vaultfiles.EntryOther, errors.New("vault file metadata is not configured")
	}
	return a.fileOperations.Kind(path)
}

// collapseAll folds every expanded directory in the tree.
func (a *App) collapseAll() {
	a.fileTreeState.CollapseAll()
	a.invalidate()
}

func (a *App) clearTreeCache() {
	a.fileTreeState.InvalidateRows()
	a.directoryCatalog.Invalidate()
}

func (a *App) copyPath(path string, relative bool) {
	text := path
	if relative {
		if rel, err := filepath.Rel(a.root, path); err == nil && !strings.HasPrefix(rel, "..") {
			text = rel
		}
	}
	clipboardWrite(text)
	if relative {
		a.notice(locales.DocumentCopiedRelativePath())
	} else {
		a.notice(locales.DocumentCopiedAbsolutePath())
	}
	a.invalidate()
}

func (a *App) trash(path string) {
	if a.busy {
		return
	}
	a.startBusy()
	go func() {
		name := filepath.Base(path)
		res, err := messageDialog(mygo.MessageOptions{
			Parent:        a.win,
			Type:          mygo.MessageWarning,
			Message:       locales.CommonTrashConfirm(name),
			Detail:        locales.CommonTrashRecoverable(),
			Buttons:       []string{locales.CommonTrashButton(), locales.CommonCancel()},
			DefaultButton: 0,
			CancelButton:  1,
		})
		if err != nil || res.Button != 0 {
			a.finish(nil)
			return
		}
		if _, err := a.vaultFileUseCases().TrashFiles([]string{path}); err != nil {
			errorDialog(locales.DialogDeleteFailed(), err.Error())
			a.finish(nil)
			return
		}
		a.finish(func() {
			if docPath := a.documentPath(); docPath == path || strings.HasPrefix(docPath, path+string(filepath.Separator)) {
				a.selectedPath = ""
				a.loadDocumentAt("", "")
				a.updateTitle()
			}
			a.clearTreeCache()
			a.notice(locales.FilesMovedToTrash())
		})
	}()
}

func (a *App) trashTreeItems(paths []string) {
	if len(paths) == 0 {
		return
	}
	if len(paths) == 1 {
		a.trash(paths[0])
		return
	}
	if a.busy {
		return
	}
	a.startBusy()
	go func() {
		res, err := messageDialog(mygo.MessageOptions{
			Parent:        a.win,
			Type:          mygo.MessageWarning,
			Message:       locales.CommonTrashManyConfirm(len(paths)),
			Detail:        locales.CommonTrashRecoverable(),
			Buttons:       []string{locales.CommonTrashButton(), locales.CommonCancel()},
			DefaultButton: 0,
			CancelButton:  1,
		})
		if err != nil || res.Button != 0 {
			a.finish(nil)
			return
		}
		mutation, trashErr := a.vaultFileUseCases().TrashFiles(paths)
		trashed := mutation.Trashed
		a.finish(func() {
			for _, path := range trashed {
				a.fileTreeState.RemoveSelected(path)
				if docPath := a.documentPath(); docPath == path || strings.HasPrefix(docPath, path+string(filepath.Separator)) {
					a.selectedPath = ""
					a.loadDocumentAt("", "")
					a.updateTitle()
				}
			}
			a.clearTreeCache()
			if len(trashed) > 0 {
				a.notice(locales.FilesMovedToTrash())
			}
			if trashErr != nil {
				errorDialog(locales.DialogDeleteFailed(), trashErr.Error())
			}
		})
	}()
}

func (a *App) renameDialog(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.renameOpen, func() {
		ui.Column(c).Width(420).Gap(12).Padding(18).Radius(12).
			Background(t.Background).Border(1, t.Border).
			Shadow(0, 18, 50, 0, ui.RGBA(0, 0, 0, 0.22)).Children(func() {
			ui.Text(c, locales.FilesRenameTitle()).FontSize(14).FontWeight(600)
			input := ui.TextInput(c, &a.renameValue).AutoFocus().Label(locales.FilesNewName())
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, locales.CommonCancel()).Clicked() {
					a.renameOpen = false
				}
				if ui.PrimaryButton(c, locales.CommonRenameButton()).Clicked() || input.Submitted() {
					a.applyRename()
				}
			})
		})
	})
}

func (a *App) applyRename() {
	name := strings.TrimSpace(a.renameValue)
	if name == "" || a.renamePath == "" {
		a.renameOpen = false
		return
	}
	old := a.renamePath
	mutation, err := a.vaultFileUseCases().RenameFile(old, name)
	if err != nil {
		a.notice(locales.CommonRenameFailed(err.Error()))
		a.renameOpen = false
		a.invalidate()
		return
	}
	if len(mutation.Moves) == 0 {
		a.renameOpen = false
		return
	}
	a.remapTreePaths(mutation.Moves)
	a.renameOpen = false
	a.renamePath = ""
	a.clearTreeCache()
	a.notice(locales.FilesRenamed())
	a.updateTitle()
	a.invalidate()
}

// openDropped opens the first document among files dropped on the editor.
func (a *App) openDropped(paths []string) {
	for _, p := range paths {
		kind, err := a.pathKind(p)
		if err == nil && kind == vaultfiles.EntryDirectory {
			a.setWorkspace(p)
			a.invalidate()
			return
		}
		if doctypes.IsEditablePath(p) {
			a.requestOpen(p)
			return
		}
	}
}

func (a *App) vaultRelativePath(path string) (string, bool) {
	if a == nil || a.root == "" || path == "" {
		return "", false
	}
	rel, err := filepath.Rel(a.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func (a *App) treeIconID(path string) string {
	if a == nil || a.navIconPicker == nil {
		return ""
	}
	return a.navIconPicker.IconID(path)
}

func (a *App) treeIcon(path string, directory bool) *ui.SVG {
	if a == nil || a.navIconPicker == nil {
		if directory {
			return componenticons.Folder()
		}
		return componenticons.File()
	}
	return a.navIconPicker.IconForPath(path, directory)
}

// clipboardWrite is a seam for tests: the real clipboard waits on the app's
// main loop, which a headless test does not run.
var clipboardWrite = mygo.Clipboard.WriteText

// handoffTreeNativeDrag promotes the active UI drag. Keep this seam so tests
// can inspect the serialized payload without opening an OS drag session.
var handoffTreeNativeDrag = func(c *ui.Context, data transfer.Data, opts mygo.DragOptions) error {
	// Serialized metadata carries the complete selected set. A process-local
	// single-item value would make same-process native drops bypass it.
	return c.HandoffDataDrag(data, nil, opts)
}
