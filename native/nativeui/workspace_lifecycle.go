package nativeui

import (
	"errors"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
	documentsave "muse/application/document"
	applicationvault "muse/application/vault"
	workspaceapp "muse/application/workspace"
	"muse/document"
	"muse/events"
	"muse/locales"
	"muse/workspace"
)

func (a *App) SetWorkspaceRepository(repository workspace.Repository) {
	if a == nil || repository == nil {
		if a != nil {
			a.workspaceRestorer = nil
		}
		return
	}
	a.workspaceRestorer = workspaceapp.NewRestorer(repository, a.workspaceEvents)
}

func (a *App) setEventDispatchers(workspaceEvents *events.Dispatcher[workspace.WorkspaceRestored], documentEvents *events.Dispatcher[document.DocumentSaved]) {
	a.workspaceSubscription.Close()
	a.documentSubscription.Close()
	a.workspaceEvents = workspaceEvents
	a.documentEvents = documentEvents
	if workspaceEvents != nil {
		a.workspaceSubscription = workspaceEvents.Subscribe(func(event workspace.WorkspaceRestored) {
			a.applyRestoredWorkspace(event.State)
		})
	}
	if documentEvents != nil {
		a.documentSubscription = documentEvents.Subscribe(func(event document.DocumentSaved) {
			a.onMain(func() {
				if a.indexedPath != "" && filepath.Clean(event.Path) == filepath.Clean(a.indexedPath) {
					a.indexedRevision = string(event.Revision)
					a.invalidate()
				}
			})
		})
	}
}

func (a *App) SetDocumentRepositories(repository document.Repository, recovery document.RecoveryRepository, history document.HistoryRepository) {
	if repository == nil || recovery == nil || history == nil {
		a.documentSession = nil
		return
	}
	a.documentSession = documentsave.NewSession(repository, recovery, history, a.documentEvents)
	if a.path != "" {
		a.documentSession.Open(a.path, a.diskSource())
	}
}

func (a *App) enableDocumentServices() {
	if a.documentSession != nil && a.documentSession.Snapshot().Generation == 0 && a.path != "" {
		a.documentSession.Open(a.path, a.diskSource())
	}
}

func (a *App) documentLoaded(source string) {
	if a.documentSession != nil {
		a.documentSession.Open(a.path, source)
	}
}

func (a *App) documentChanged() {
	if a.documentSession != nil {
		a.documentSession.Changed(time.Now())
	}
}

func (a *App) SetDependencies(dependencies Dependencies) {
	a.setEventDispatchers(dependencies.WorkspaceEvents, dependencies.DocumentEvents)
	a.SetWorkspaceRepository(dependencies.Workspace)
	a.SetDocumentRepositories(dependencies.Documents, dependencies.Recovery, dependencies.History)
	a.vaultSearchStore = dependencies.VaultSearch
	a.newVaultIndex = dependencies.NewVaultIndex
	a.newVaultCatalog = dependencies.NewVaultCatalog
	a.fileOperations = dependencies.FileOperations
	a.recentItems = dependencies.RecentItems
	if a.recentItems != nil {
		a.recents = a.recentItems.Entries()
	} else {
		a.recents = nil
	}
	a.imageCacheStore = dependencies.ImageCacheStore
	a.watcherBackendFactory = dependencies.NewWatcherBackend
	if dependencies.NavIcons != nil {
		a.navIconStore, _ = applicationvault.NewIconPreferences(dependencies.NavIcons)
	}
}

func (a *App) writeDocument(path, source string, expected document.Revision) error {
	if a.documentSession == nil {
		return errors.New("document repository is not configured")
	}
	_, err := a.documentSession.SaveNow(path, path, source, expected)
	return err
}

func (a *App) documentTick(c *ui.Context) {
	session := a.documentSession
	if session == nil {
		return
	}
	c.After(100 * time.Millisecond)
	select {
	case result := <-session.Results():
		if !session.CompleteSave(result, a.path, a.diskSource()) {
			break
		}
		if result.Err != nil {
			if errors.Is(result.Err, document.ErrConflict) {
				if a.documents != nil {
					a.documents.ConflictOpen = true
				}
				a.notice(locales.DocumentExternalChangedAutosavePaused())
			} else {
				a.notice(locales.CommonAutosaveError(result.Err.Error()))
			}
		} else if result.Request.Path != "" && result.Note.Revision != "" && result.Request.Source == a.diskSource() {
			a.dirty = false
			a.updateTitle()
		}
	default:
	}
	state := session.Snapshot()
	if !state.MetadataDue.IsZero() && !time.Now().Before(state.MetadataDue) {
		session.ClearMetadataDeadline()
		a.updateManagedTimestamp()
		return
	}
	request, ok := session.BeginSave(a.path, a.diskSource(), a.dirty, time.Now())
	if !ok {
		return
	}
	session.StartSave(request)
}

func (a *App) flushManagedTimestamp() {
	if a.documentSession == nil || a.documentSession.Snapshot().MetadataDue.IsZero() {
		return
	}
	a.documentSession.ClearMetadataDeadline()
	a.updateManagedTimestamp()
}

func (a *App) updateManagedTimestamp() {
	if a.ed == nil || a.documentSession == nil || codeLanguage(a.path) != "" {
		return
	}
	source := a.ed.Source()
	next, err := documentsave.UpdateManagedTimestamp(source, time.Now())
	if err != nil {
		a.notice(locales.CommonTimestampError(err.Error()))
		return
	}
	a.documentSession.SetApplyingMetadata(true)
	a.ed.ReplaceSystemAll(next)
	a.documentSession.SetApplyingMetadata(false)
}

func (a *App) persistWorkspaceState() {
	if a == nil {
		return
	}
	state := workspace.State{Version: 1, VaultPath: a.root}
	if a.path != "" && a.ed != nil {
		const tabID, paneID = "main", "main"
		state.ActiveTab = tabID
		state.Tabs = []workspace.Tab{{
			ID:           tabID,
			ActivePaneID: paneID,
			Layout: workspace.Leaf(workspace.Pane{
				ID:       paneID,
				Document: workspace.Document{Path: a.path, CursorOffset: a.ed.SelectionRange().End},
			}),
		}}
	}
	if a.workspaceRestorer != nil {
		_ = a.workspaceRestorer.Persist(state)
	}
}

func (a *App) restoreWorkspaceState() {
	if a.workspaceRestorer == nil {
		return
	}
	_, _ = a.workspaceRestorer.Restore()
}

func (a *App) applyRestoredWorkspace(state workspace.State) {
	if state.VaultPath != "" {
		a.setWorkspace(state.VaultPath)
	}
	if doc, ok := state.ActiveDocument(); ok {
		a.openPathNow(doc.Path)
		if filepath.Clean(a.path) == filepath.Clean(doc.Path) && a.ed != nil {
			a.ed.Select(doc.CursorOffset, doc.CursorOffset)
			a.ed.ScrollToOffset(doc.CursorOffset)
		}
	}
}

func (a *App) closeVaultRuntimeNow() {
	if a == nil || a.closeVaultRuntime == nil {
		return
	}
	closeRuntime := a.closeVaultRuntime
	a.closeVaultRuntime = nil
	closeRuntime()
}

func (a *App) closeLifecycle() {
	if a == nil {
		return
	}
	a.workspaceSubscription.Close()
	a.documentSubscription.Close()
	if a.documentSession != nil {
		a.documentSession.Close()
	}
	if a.documents != nil {
		a.documents.CloseAll()
	}
	if a.windows != nil {
		a.windows.CloseAll()
	}
	a.closeVaultRuntimeNow()
}
