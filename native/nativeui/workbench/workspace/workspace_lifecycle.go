package nativeui

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
	applicationconfig "muse/application/config"
	documentsave "muse/application/document"
	applicationvault "muse/application/vault"
	workspaceapp "muse/application/workspace"
	"muse/document"
	doctypes "muse/document/types"
	"muse/events"
	"muse/locales"
	documentdialogs "muse/nativeui/workbench/features/dialogs/document"
	"muse/nativeui/workbench/hosting"
	"muse/workspace"
)

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
		a.documentRepository, a.documentRecovery, a.documentHistory = nil, nil, nil
		return
	}
	a.documentRepository, a.documentRecovery, a.documentHistory = repository, recovery, history
	if a.noteEditor() != nil {
		snapshot := a.noteEditor().Snapshot()
		if !a.editorPaneHandle.IsZero() && a.windows != nil {
			_ = a.windows.Close(a.editorPaneHandle)
			a.editorPaneHandle = window.Handle{}
		}
		if err := a.createActiveNote(document.NewNote(snapshot.Path, snapshot.DiskSource, snapshot.Revision), snapshot.CursorOffset); err != nil {
			a.notice(err.Error())
		}
	}
}

func (a *App) enableDocumentServices() {
	// Persistence is initialized with the pane-owned noteeditor.Content.
}

func (a *App) SetDependencies(dependencies Dependencies) {
	if a.assistant != nil {
		if err := a.assistant.SetClient(dependencies.AssistantStream); err != nil {
			a.notice(err.Error())
			return
		}
	}
	a.configRepository = dependencies.Configuration
	a.assistantSettingsLocation = dependencies.AssistantSettingsLocation
	a.assistantProvider = dependencies.AssistantProvider
	a.assistantStream = dependencies.AssistantStream
	if a.configRepository != nil {
		settings, _ := applicationconfig.NewService(a.configRepository).LoadAssistantSettings()
		a.ai = settings
		a.applyEditorSettings(a.loadEditorSettings())
	}
	a.setEventDispatchers(dependencies.WorkspaceEvents, dependencies.DocumentEvents)
	a.SetWorkspaceRepository(dependencies.Workspace)
	a.SetDocumentRepositories(dependencies.Documents, dependencies.Recovery, dependencies.History)
	a.documentExportWriter = dependencies.DocumentExportWriter
	a.metadataReader = dependencies.DocumentMetadata
	a.vaultSearchReconciler = dependencies.VaultSearchReconciler
	a.newVaultIndex = dependencies.NewVaultIndex
	a.newVaultCatalog = dependencies.NewVaultCatalog
	a.fileOperations = dependencies.FileOperations
	a.recentItems = dependencies.RecentItems
	a.imageCacheStore = dependencies.ImageCacheStore
	a.imageLocalFiles = dependencies.ImageLocalFiles
	a.temporaryImageStore = dependencies.TemporaryImages
	a.openExternal = dependencies.OpenExternal
	a.newWatcher = dependencies.NewWatcher
	a.newAgentWorkspace = dependencies.NewAgentWorkspace
	if dependencies.NavIcons != nil {
		a.navIconStore, _ = applicationvault.NewIconPreferences(dependencies.NavIcons)
	}
}

func (a *App) writeDocument(path, source string, expected document.Revision) error {
	if a.noteEditor() == nil {
		return errors.New("document repository is not configured")
	}
	_, err := a.noteEditor().SaveNow(path, expected)
	return err
}

func (a *App) readDocument(path string) (document.Note, error) {
	if a == nil || a.documentRepository == nil {
		return document.Note{}, errors.New("document repository is not configured")
	}
	return a.documentRepository.Read(path)
}

func (a *App) destinationRevision(path string) (document.Revision, error) {
	if a == nil || a.documentRepository == nil {
		return "", errors.New("document repository is not configured")
	}
	note, err := a.documentRepository.Read(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return note.Revision, err
}

func (a *App) saveExternalDocument(path, source string, expected document.Revision) error {
	if a == nil || a.documentRepository == nil || a.documentRecovery == nil {
		return errors.New("document persistence is not configured")
	}
	saver := documentsave.NewSaver(a.documentRepository, a.documentRecovery, a.documentEvents)
	_, err := saver.Save("", path, source, expected)
	return err
}

func (a *App) documentTick(c *ui.Context) {
	content := a.noteEditor()
	if content == nil {
		return
	}
	c.After(100 * time.Millisecond)
	select {
	case result := <-content.Results():
		if !content.CompleteSave(result) {
			break
		}
		if result.Err != nil {
			if errors.Is(result.Err, document.ErrConflict) {
				if a.documents != nil {
					a.documents.Apply(documentdialogs.Action{Kind: documentdialogs.OpenConflict})
				}
				a.notice(locales.DocumentExternalChangedAutosavePaused())
			} else {
				a.notice(locales.CommonAutosaveError(result.Err.Error()))
			}
		} else if result.Request.Path != "" && result.Note.Revision != "" && result.Request.Source == a.diskSource() {
			a.updateTitle()
		}
	default:
	}
	state := content.PersistenceState()
	if !state.MetadataDue.IsZero() && !time.Now().Before(state.MetadataDue) {
		content.ClearMetadataDeadline()
		a.updateManagedTimestamp()
		return
	}
	request, ok := content.BeginSave(time.Now())
	if !ok {
		return
	}
	content.StartSave(request)
}

func (a *App) flushManagedTimestamp() {
	if a.noteEditor() == nil || a.noteEditor().PersistenceState().MetadataDue.IsZero() {
		return
	}
	a.noteEditor().ClearMetadataDeadline()
	a.updateManagedTimestamp()
}

func (a *App) updateManagedTimestamp() {
	if a.noteEditor() == nil || !doctypes.IsMarkdownPath(a.documentPath()) {
		return
	}
	source := a.noteEditor().Snapshot().Source
	next, err := documentsave.UpdateManagedTimestamp(source, time.Now())
	if err != nil {
		a.notice(locales.CommonTimestampError(err.Error()))
		return
	}
	_ = a.noteEditor().ReplaceSystem(next)
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
	if a.assistantModelCancel != nil {
		a.assistantModelCancel()
		a.assistantModelCancel = nil
	}
	if a.aiModelCatalog != nil {
		a.aiModelCatalog.Reset()
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
