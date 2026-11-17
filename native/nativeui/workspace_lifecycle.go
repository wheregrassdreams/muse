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
	"muse/events"
	"muse/locales"
	doctypes "muse/nativeui/components/doctypes"
	"muse/nativeui/components/documentdialogs"
	"muse/nativeui/window"
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
	if a == nil {
		return
	}
	state := workspace.State{Version: 1, VaultPath: a.root}
	if a.documentPath() != "" && a.noteEditor() != nil {
		const tabID = "main"
		paneID := workspace.PaneID(a.focusedPane())
		state.ActiveTab = tabID
		state.Tabs = []workspace.Tab{{
			ID:           tabID,
			ActivePaneID: paneID,
			Layout: workspace.Leaf(workspace.Pane{
				ID:       paneID,
				Document: workspace.Document{Path: a.documentPath(), CursorOffset: a.noteEditor().NavigationCommands().SelectionRange().End},
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
	for _, tab := range state.Tabs {
		if tab.ID == state.ActiveTab && tab.ActivePaneID != "" {
			a.workspacePane = window.PaneID(tab.ActivePaneID)
			break
		}
	}
	if state.VaultPath != "" {
		a.setWorkspace(state.VaultPath)
	}
	if doc, ok := state.ActiveDocument(); ok {
		a.openPathNow(doc.Path)
		if filepath.Clean(a.documentPath()) == filepath.Clean(doc.Path) && a.noteEditor() != nil {
			a.noteEditor().NavigationCommands().Select(doc.CursorOffset, doc.CursorOffset)
			a.noteEditor().NavigationCommands().ScrollToOffset(doc.CursorOffset)
		}
	}
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
	a.closeVaultSessionNow()
}
