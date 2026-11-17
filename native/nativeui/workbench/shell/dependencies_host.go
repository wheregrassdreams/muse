package nativeui

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
		if a.assistant != nil {
			a.assistant.SetSettings(settings)
		}
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
	if a.imagePreview != nil {
		a.imagePreview.Close()
		a.imagePreview = nil
	}
	a.temporaryImageStore = dependencies.TemporaryImages
	a.openExternal = dependencies.OpenExternal
	a.newWatcher = dependencies.NewWatcher
	a.newAgentWorkspace = dependencies.NewAgentWorkspace
	if dependencies.NavIcons != nil {
		a.navIconStore, _ = applicationvault.NewIconPreferences(dependencies.NavIcons)
	}
}
