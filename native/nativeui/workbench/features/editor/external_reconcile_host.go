package nativeui


func (a *App) reconcileExternalCurrentDocument(session *applicationvault.Session) {
	if a.documentPath() == "" || !doctypes.IsMarkdownPath(a.documentPath()) {
		return
	}
	root := a.root
	if session != nil && session.Snapshot().Root != "" {
		root = session.Snapshot().Root
	}
	if root == "" {
		return
	}
	rel, err := filepath.Rel(root, a.documentPath())
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return
	}
	// The vault index is an asynchronous catalog, not the source of truth for
	// the active file's existence or bytes. A refresh can temporarily omit a
	// document while rescanning; verify against the repository before reporting
	// a deletion or reconciling a revision.
	if a.documentRepository == nil {
		return
	}
	note, readErr := a.documentRepository.Read(a.documentPath())
	if errors.Is(readErr, os.ErrNotExist) {
		a.handleExternalDocumentMissing()
		return
	}
	if readErr != nil {
		a.notice(locales.WorkspaceIndexRefreshFailed(readErr.Error()))
		return
	}
	a.reconcileExternalNote(note)
}

func (a *App) handleExternalDocumentMissing() {
	if a.noteEditor() == nil || a.documentPath() == "" {
		return
	}
	path := a.documentPath()
	alreadyReported := a.noteEditor().PersistenceState().ConflictPath == path
	_ = a.documentWorkflows().MarkExternalMissing()
	if alreadyReported {
		return
	}
	if a.documents != nil {
		a.documents.Apply(documentdialogs.Action{Kind: documentdialogs.OpenConflict})
	}
	a.notice(locales.VaultCurrentFileDeleted())
}

func (a *App) reconcileExternalNote(fresh document.Note) {
	if a.noteEditor() == nil || fresh.Path == "" {
		return
	}
	caret := a.noteEditor().NavigationCommands().Caret()
	outcome, err := a.documentWorkflows().ReconcileExternal(fresh)
	if err != nil {
		if outcome == documentapp.ExternalChangeConflict {
			a.notice(locales.WorkspaceRecoverySaveFailed(err.Error()))
		} else {
			a.notice(locales.WorkspaceIndexRefreshFailed(err.Error()))
		}
	}
	switch outcome {
	case documentapp.ExternalChangeLoaded:
		a.noteEditor().NavigationCommands().Select(min(caret, len(a.documentSource())), min(caret, len(a.documentSource())))
		a.updateTitle()
		if a.documentDirty() {
			a.notice(locales.VaultExternalUpdatedAt())
		} else {
			a.notice(locales.DocumentExternalLoaded())
		}
	case documentapp.ExternalChangeMergedSaving:
		a.notice(locales.VaultExternalMergedSaving())
	case documentapp.ExternalChangeConflict:
		if a.documents != nil {
			a.documents.Apply(documentdialogs.Action{Kind: documentdialogs.OpenConflict})
		}
	case documentapp.ExternalChangeNoop:
	}
}
