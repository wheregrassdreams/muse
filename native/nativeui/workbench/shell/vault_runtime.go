package nativeui


func (a *App) activeVaultRoot() string {
	if a.vault != nil && a.vaultSnapshot.Root != "" {
		return a.vaultSnapshot.Root
	}
	return a.root
}

func (a *App) vaultTick(c *ui.Context) {
	root := a.root
	if root == "" && a.documentPath() != "" {
		root = filepath.Dir(a.documentPath())
	}
	if a.noteEditor() == nil || root == "" || a.newVaultIndex == nil || a.newVaultCatalog == nil {
		a.clearWikiSuggestionCache()
		if a.vault != nil {
			old := a.vault
			a.vault = nil
			a.vaultSnapshot = applicationvault.Snapshot{}
			a.directoryCatalog.CloseSession(old)
			go old.Close()
		}
		return
	}
	if a.vault == nil || filepath.Clean(a.vaultSnapshot.Root) != filepath.Clean(root) {
		a.clearWikiSuggestionCache()
		if a.vault != nil {
			old := a.vault
			a.vault = nil
			a.directoryCatalog.CloseSession(old)
			go old.Close()
		}
		newWatcher := a.newWatcher
		session, err := applicationvault.NewSession(root, applicationvault.SessionDependencies{
			NewIndex: a.newVaultIndex, NewCatalog: a.newVaultCatalog,
			NewWatcher: newWatcher, SearchReconciler: a.vaultSearchReconciler,
		})
		if err != nil {
			a.notice(locales.VaultIndexWatchError(err.Error()))
			return
		}
		a.vault = session
		a.vaultSnapshot = session.Snapshot()
		a.vaultResultNavigation = make(chan vaultNavigationResult, 1)
		a.invalidate()
	}
	if a.vault == nil {
		return
	}
	c.After(100 * time.Millisecond)
	for n := 0; n < 32; n++ {
		select {
		case result, ok := <-a.vault.Results():
			if !ok {
				return
			}
			a.handleVaultResult(a.vault, result)
		default:
			n = 32
		}
	}
	if a.vaultResultNavigation != nil {
		select {
		case result := <-a.vaultResultNavigation:
			if result.err != nil {
				a.notice(locales.WorkspaceSearchResultUpdated(result.err.Error()))
			} else {
				a.acceptSearchHit(result.hit)
			}
		default:
		}
	}
	a.linkIndexTick(c)
}

func (a *App) handleVaultResult(session *applicationvault.Session, result applicationvault.Result) {
	if session == nil || a.vault != session {
		return
	}
	a.vaultSnapshot = result.Snapshot
	switch result.Kind {
	case applicationvault.SessionReady:
		// setWorkspace can be called before vaultTick creates the session. In
		// that interval children(root) caches an empty result; invalidate it as
		// soon as the initial catalog is ready so the first tree render sees it.
		a.clearTreeCache()
		if result.Err != nil {
			a.notice(locales.VaultIndexWatchError(result.Err.Error()))
		}
		if a.noteEditor() != nil {
			a.configureActiveNote(0)
		}
		if a.pendingLinks {
			a.pendingLinks = false
			a.openLinks()
		}
		if a.pendingWiki != "" {
			target := a.pendingWiki
			a.pendingWiki = ""
			a.openWiki(target)
		}
		if a.documentPath() != "" {
			_ = session.WatchFile(a.documentPath())
		}
	case applicationvault.FilesRefreshed:
		a.clearTreeCache()
		if result.Err != nil && !errors.Is(result.Err, os.ErrNotExist) {
			a.notice(locales.WorkspaceIndexRefreshFailed(result.Err.Error()))
		}
		if a.workspaceSearch.IsOpen() {
			a.vault.ForceSearch(time.Now(), 0)
		}
		if result.Full || result.TouchesPath(a.documentPath()) {
			a.reconcileExternalCurrentDocument(session)
		}
		if a.noteEditor() != nil {
			a.configureActiveNote(0)
		}
	case applicationvault.ActiveDocumentApplied:
		if a.noteEditor() != nil {
			a.configureActiveNote(0)
		}
	case applicationvault.QueryCompleted:
		if session.IsCurrentSearchResult(result) && result.Query == a.workspaceSearch.Query() && result.Literal == (a.workspaceSearch.Mode() == searchui.Literal) {
			a.vaultSnapshot = result.Snapshot
			if len(result.Snapshot.Search.Hits) > 0 {
				a.workspaceSearch.Apply(searchui.Action{Kind: searchui.MoveSelection, Index: min(a.workspaceSearch.Selection(), len(result.Snapshot.Search.Hits)-1)})
			}
			if pending := a.vaultPendingNavigation; pending != nil {
				if hit, ok := result.Snapshot.Search.SearchHitAtPath(pending.Path); ok {
					a.vaultPendingNavigation = nil
					a.acceptSearchHit(hit)
				}
			}
		}
	case applicationvault.LinksRefreshed:
		a.vaultSnapshot = result.Snapshot
	}
	a.invalidate()
}

func (a *App) closeVaultSessionNow() {
	if a == nil {
		return
	}
	a.clearWikiSuggestionCache()
	if a.vault == nil {
		return
	}
	session := a.vault
	a.vault = nil
	a.vaultSnapshot = applicationvault.Snapshot{}
	a.directoryCatalog.CloseSession(session)
	_ = session.Close()
}
