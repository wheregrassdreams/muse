// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

func (a *App) searchView(c *ui.Context) {
	a.documentSearchView(c)
}

func (a *App) openWorkspaceSearch() {
	a.closeQuickOpen()
	a.commandPalette.Close()
	if a.root == "" && a.documentPath() != "" {
		a.root = filepath.Dir(a.documentPath())
	}
	a.enableDocumentServices()
	a.workspaceSearch.Open()
	a.invalidate()
}

func (a *App) closeWorkspaceSearch() {
	a.workspaceSearch.Close()
	if a.vault != nil {
		literal := a.workspaceSearch.Mode() == searchui.Literal
		a.vault.ResetSearchSchedule()
		_, _ = a.vault.Query("", literal, a.documentPath(), a.diskSource())
		a.vaultSnapshot = a.vault.Snapshot()
	}
	a.invalidate()
}

func (a *App) workspaceSearchDialog(c *ui.Context) {
	if !a.workspaceSearch.IsOpen() {
		return
	}
	if delay := a.syncWorkspaceSearchQuery(); delay > 0 {
		c.After(delay)
	}

	model := searchui.View{
		SearchIcon: componenticons.Search(),
		FileIcon:   componenticons.File(), IconTint: iconTint(c.Theme()),
	}
	if a.root == "" {
		model.Availability = searchui.VaultRequired
	} else if a.vault == nil || !a.vaultSnapshot.SearchAvailable {
		model.Availability = searchui.IndexBuilding
		model.Error = a.vaultSnapshot.Search.Error
		if model.Error == "" {
			model.Error = a.vaultSnapshot.Error
		}
	} else {
		model.Availability = searchui.Ready
		model.Busy = a.vaultSnapshot.Search.Busy
		model.Error = a.vaultSnapshot.Search.Error
	}
	model.CanChangeMode = a.vault != nil
	for _, entry := range a.workspaceRecentNotes() {
		model.Recent = append(model.Recent, searchui.RecentItem{
			Path: entry.Path, Title: filepath.Base(entry.Path), PathLabel: a.workspacePathLabel(entry.Path),
		})
	}
	if a.vault != nil {
		hits := a.vaultSnapshot.Search.Hits
		for _, hit := range hits {
			title := filepath.Base(hit.Path)
			if noteTitle, _, ok := a.vault.NoteContent(hit.Path); ok && strings.TrimSpace(noteTitle) != "" {
				title = noteTitle
			}
			model.Results = append(model.Results, searchui.Result{
				Path: hit.Path, Revision: hit.Revision, Offset: hit.Start,
				Title: title, PathLabel: a.workspacePathLabel(hit.Path), Snippet: hit.Snippet,
			})
		}
	}
	a.workspaceSearch.Render(c, model, func(action searchui.Action) {
		switch action.Kind {
		case searchui.QueryChanged:
		case searchui.ModeChanged:
			if a.vault != nil {
				a.vault.ForceSearch(time.Now(), 0)
			}
			a.vaultSnapshot.Search.Hits = nil
			a.workspaceSearch.FocusInput(c)
		case searchui.MoveSelection:
		case searchui.OpenRecent:
			a.openWorkspaceRecent(recentEntry{Path: action.Recent.Path})
		case searchui.OpenResult:
			a.closeWorkspaceSearch()
			a.navigateSearchHit(vaultsearch.Hit{
				Path: action.Result.Path, Revision: action.Result.Revision, Start: action.Result.Offset,
			})
		case searchui.Dismiss:
			a.closeWorkspaceSearch()
		}
	})
}
