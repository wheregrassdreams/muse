// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

// focusedPane returns the window host's active pane, falling back to the
// initial editor pane before any pane content has been opened.
func (a *App) focusedPane() window.PaneID {
	if a.windows != nil {
		if pane, ok := a.windows.FocusedPane("main"); ok {
			if handle := a.workspacePaneHandles[pane]; !handle.IsZero() {
				a.editorPaneHandle = handle
			}
			return pane
		}
	}
	if a.workspacePane != "" {
		return a.workspacePane
	}
	return "editor"
}

func (a *App) renderFocusedPane(c *ui.Context) {
	if a.windows == nil {
		return
	}
	a.windows.RenderPane(c, "main", a.focusedPane())
}

func (a *App) setWorkspace(root string) {
	// A workspace is the start of a new editing context. Do not leave the
	// previous vault's document or pane descriptors visible after switching folders.
	documentOutsideWorkspace := false
	if root != "" && a.documentPath() != "" {
		relative, err := filepath.Rel(root, a.documentPath())
		documentOutsideWorkspace = err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative)
	}
	if filepath.Clean(root) != filepath.Clean(a.root) || documentOutsideWorkspace {
		a.activeLinkIndex.Clear()
		a.clearWikiSuggestionCache()
		a.closeWorkspacePaneSurfaces()
		a.workspacePane = "editor"
		if a.workspaceSession != nil {
			if err := a.workspaceSession.Restore(workspace.State{Version: 1, VaultPath: root}); err != nil {
				a.notice(err.Error())
			}
		}
		if err := a.createActiveNote(document.NewNote("", "", document.RevisionOf("")), 0); err != nil {
			a.notice(err.Error())
		}
		a.resetDoc()
		if a.vault != nil {
			old := a.vault
			a.vault = nil
			a.vaultSnapshot = applicationvault.Snapshot{}
			if a.quickOpenCandidates != nil {
				a.quickOpenCandidates.SetVault(nil)
			}
			a.directoryCatalog.CloseSession(old)
			go old.Close()
		}
	}
	a.root = root
	a.clearTreeCache()
	a.fileTreeState.CollapseAll()
	a.fileTreeState.SetSelection(nil)
	a.fileTreeState.SetFocusID("")
	a.fileTreeState.SetScrollOffset(ui.Point{})
	a.selectedPath = ""
	a.children(root)
	a.addRecent(root, true)
	a.panel = panelFiles
	a.panelOpen = true
}

func (a *App) workspaceRecentNotes() []recentEntry {
	if a.recentItems == nil {
		return nil
	}
	return a.recentItems.LiveNotes(a.root, workspaceapp.MaxVisibleRecent)
}

func (a *App) openWorkspaceRecent(entry recentEntry) {
	a.closeWorkspaceSearch()
	if entry.IsDir {
		a.setWorkspace(entry.Path)
		return
	}
	a.requestOpen(entry.Path)
}
