package nativeui


type vaultNavigationResult struct {
	hit vaultsearch.Hit
	err error
}

func (a *App) syncWorkspaceSearchQuery() time.Duration {
	if a.vault == nil || !a.vaultSnapshot.Ready {
		return 0
	}
	query := a.workspaceSearch.Query()
	delay, err := a.vault.ScheduleSearch(query, a.workspaceSearch.Mode() == searchui.Literal, a.documentPath(), a.diskSource(), time.Now())
	if err != nil && !errors.Is(err, applicationvault.ErrSessionNotReady) {
		a.notice(locales.WorkspaceSearchFailed(err.Error()))
	}
	a.vaultSnapshot = a.vault.Snapshot()
	return delay
}

func (a *App) navigateIndexed(path string, offset int) {
	root := a.activeVaultRoot()
	if root == "" {
		return
	}
	full := filepath.Join(root, filepath.FromSlash(path))
	if filepath.Clean(full) == filepath.Clean(a.documentPath()) {
		a.noteEditor().NavigationCommands().ScrollToOffset(editorsource.OffsetInNormalizedSource(a.diskSource(), offset))
		return
	}
	a.indexedPath, a.indexedOffset = full, offset
	a.requestOpen(full)
}

func (a *App) navigateSearchHit(hit vaultsearch.Hit) {
	if a.vault == nil || a.noteEditor() == nil || !a.vaultSnapshot.SearchAvailable {
		return
	}
	query, literal := a.workspaceSearch.Query(), a.workspaceSearch.Mode() == searchui.Literal
	currentPath, currentSource := a.documentPath(), a.diskSource()
	session := a.vault
	go func() {
		status, err := session.ValidateSearchNavigation(a.documentRepository, hit, currentPath, document.RevisionOf(currentSource))
		if err == nil && status == applicationvault.SearchHitCurrent {
			a.postVaultNavigation(session, vaultNavigationResult{hit: hit})
			return
		}
		if err != nil && errors.Is(err, os.ErrNotExist) {
			a.postVaultNavigation(session, vaultNavigationResult{err: err})
			return
		}
		if err != nil {
			a.postVaultNavigation(session, vaultNavigationResult{err: err})
			return
		}
		pending := hit
		a.onMain(func() {
			if a.vault != session {
				return
			}
			a.vaultPendingNavigation = &pending
			a.workspaceSearch.Apply(searchui.Action{Kind: searchui.QueryChanged, Query: query})
			if literal {
				a.workspaceSearch.Apply(searchui.Action{Kind: searchui.ModeChanged, Mode: searchui.Literal})
			} else {
				a.workspaceSearch.Apply(searchui.Action{Kind: searchui.ModeChanged, Mode: searchui.FullText})
			}
			session.ForceSearch(time.Now(), 120*time.Millisecond)
			a.invalidate()
		})
	}()
}

func (a *App) postVaultNavigation(session *applicationvault.Session, result vaultNavigationResult) {
	a.onMain(func() {
		if a.vault != session || a.vaultResultNavigation == nil {
			return
		}
		select {
		case a.vaultResultNavigation <- result:
		default:
			select {
			case <-a.vaultResultNavigation:
			default:
			}
			select {
			case a.vaultResultNavigation <- result:
			default:
			}
		}
		a.invalidate()
	})
}

func (a *App) acceptSearchHit(hit vaultsearch.Hit) {
	full := filepath.Join(a.activeVaultRoot(), filepath.FromSlash(hit.Path))
	if filepath.Clean(full) == filepath.Clean(a.documentPath()) && document.RevisionOf(a.diskSource()) == document.Revision(hit.Revision) {
		a.noteEditor().NavigationCommands().NavigateToRange(
			editorsource.OffsetInNormalizedSource(a.diskSource(), hit.Start),
			editorsource.OffsetInNormalizedSource(a.diskSource(), hit.End),
		)
		return
	}
	a.indexedPath, a.indexedOffset, a.indexedRevision = full, hit.Start, hit.Revision
	a.requestOpen(full)
}
