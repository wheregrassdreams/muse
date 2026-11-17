package nativeui

func (a *App) SetWorkspaceRepository(repository workspace.Repository) {
	if a == nil {
		return
	}
	if repository == nil {
		a.workspaceRestorer = nil
	} else {
		a.workspaceRestorer = workspaceapp.NewRestorer(repository, a.workspaceEvents)
	}
	state := workspace.State{Version: 1}
	if a.workspaceSession != nil {
		state = a.workspaceSession.Snapshot()
	}
	if session, err := workspaceapp.NewSession(repository, state); err == nil {
		a.workspaceSession = session
	}
}

func newWorkspaceSession(repository workspace.Repository) *workspaceapp.Session {
	session, _ := workspaceapp.NewSession(repository, workspace.State{Version: 1})
	return session
}

func (a *App) persistWorkspaceState() {
	if a == nil || a.workspaceSession == nil {
		return
	}
	if err := a.syncWorkspaceDocument(); err != nil {
		a.notice(err.Error())
		return
	}
	if err := a.workspaceSession.SetVaultPath(a.root); err != nil {
		a.notice(err.Error())
		return
	}
	if err := a.workspaceSession.Persist(); err != nil && a.workspaceRestorer != nil {
		a.notice(err.Error())
	}
}

func (a *App) syncWorkspaceDocument() error {
	if a == nil || a.workspaceSession == nil || a.noteEditor() == nil || a.documentPath() == "" {
		return nil
	}
	state := a.workspaceSession.Snapshot()
	tabID := state.ActiveTab
	paneID := workspace.PaneID(a.focusedPane())
	doc := workspace.Document{
		Path:         a.documentPath(),
		CursorOffset: a.noteEditor().NavigationCommands().SelectionRange().End,
	}
	if tabID == "" || !workspaceHasTab(state, tabID) {
		tabID = "main"
		state.ActiveTab = tabID
		state.Tabs = append(state.Tabs, workspace.Tab{
			ID: tabID, ActivePaneID: paneID,
			Layout: workspace.Leaf(workspace.Pane{ID: paneID, Document: doc}),
		})
		return a.workspaceSession.Restore(state)
	}
	if err := a.workspaceSession.UpdateDocument(tabID, paneID, doc); err != nil {
		return err
	}
	return a.workspaceSession.Focus(tabID, paneID)
}

func workspaceHasTab(state workspace.State, id workspace.TabID) bool {
	for _, tab := range state.Tabs {
		if tab.ID == id {
			return true
		}
	}
	return false
}

func (a *App) restoreWorkspaceState() {
	if a.workspaceRestorer == nil {
		return
	}
	state, err := a.workspaceRestorer.Restore()
	if err != nil {
		a.notice(err.Error())
		return
	}
	if a.workspaceEvents == nil {
		a.applyRestoredWorkspace(state)
	}
}

func (a *App) applyRestoredWorkspace(state workspace.State) {
	if err := state.Validate(); err != nil {
		a.notice(err.Error())
		return
	}
	if filepath.Clean(state.VaultPath) != filepath.Clean(a.root) {
		a.setWorkspace(state.VaultPath)
	}
	if a.workspaceSession != nil {
		if err := a.workspaceSession.Restore(state); err != nil {
			a.notice(err.Error())
			return
		}
	}
	a.closeWorkspacePaneSurfaces()
	a.editorPaneHandle = window.Handle{}
	for _, tab := range state.Tabs {
		if tab.ID == state.ActiveTab && tab.ActivePaneID != "" {
			a.workspacePane = window.PaneID(tab.ActivePaneID)
			break
		}
	}
	active, ok := workspaceActiveTab(state)
	if !ok {
		if err := a.createActiveNote(document.NewNote("", "", document.RevisionOf("")), 0); err != nil {
			a.notice(err.Error())
		}
		return
	}
	var activeHandle window.Handle
	for _, pane := range active.Panes() {
		note, err := a.readDocument(pane.Document.Path)
		if err != nil {
			a.notice(locales.CommonOpenFileFailed(err.Error()))
			continue
		}
		if _, err := a.createNotePane(window.PaneID(pane.ID), note, pane.Document.CursorOffset); err != nil {
			a.notice(err.Error())
			continue
		}
		if workspace.PaneID(a.workspacePane) == pane.ID {
			activeHandle = a.workspacePaneHandles[window.PaneID(pane.ID)]
		}
	}
	if activeHandle.IsZero() {
		activeHandle = a.workspacePaneHandles[window.PaneID(a.workspacePane)]
	}
	if !activeHandle.IsZero() {
		a.editorPaneHandle = activeHandle
		a.windows.Focus(activeHandle)
		a.selectedPath = a.documentPath()
		if a.selectedPath != "" {
			a.addRecent(a.selectedPath, false)
			if a.vault != nil {
				_ = a.vault.WatchFile(a.selectedPath)
				if doctypes.IsMarkdownPath(a.selectedPath) {
					a.queueLinkIndexSnapshot()
				}
			}
		}
		a.updateTitle()
		a.invalidate()
	}
}

func (a *App) closeWorkspacePaneSurfaces() {
	if a == nil {
		return
	}
	closed := make(map[window.Handle]struct{})
	if a.windows != nil {
		for pane, handle := range a.workspacePaneHandles {
			if !handle.IsZero() {
				if _, ok := closed[handle]; !ok {
					_ = a.windows.Close(handle)
					closed[handle] = struct{}{}
				}
			}
			delete(a.workspacePaneHandles, pane)
		}
		if !a.editorPaneHandle.IsZero() {
			if _, ok := closed[a.editorPaneHandle]; !ok {
				_ = a.windows.Close(a.editorPaneHandle)
			}
		}
	}
	a.editorPaneHandle = window.Handle{}
}

func workspaceActiveTab(state workspace.State) (workspace.Tab, bool) {
	for _, tab := range state.Tabs {
		if tab.ID == state.ActiveTab {
			return tab, true
		}
	}
	return workspace.Tab{}, false
}

func (a *App) closeLifecycle() {
	if a == nil {
		return
	}
	if a.assistant != nil {
		a.assistant.Close()
	}
	a.workspaceSubscription.Close()
	a.documentSubscription.Close()
	if a.documents != nil {
		a.documents.Close()
	}
	if a.windows != nil {
		a.windows.CloseAll()
	}
	if a.workspaceSession != nil {
		a.workspaceSession.Close()
	}
	a.closeVaultSessionNow()
}
