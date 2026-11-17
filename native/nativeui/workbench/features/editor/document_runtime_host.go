package nativeui

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
		if !a.documentWorkflows().CompleteSave(result) {
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
		if _, ok := content.TakeDeferredExternal(); ok {
			// The watcher event is an invalidation, and may have been queued
			// before a later write. Re-read the repository after the in-flight
			// save completes instead of applying a potentially stale indexed
			// snapshot.
			a.reconcileExternalCurrentDocument(a.vault)
		}
	default:
	}
	if err := a.documentWorkflows().Advance(time.Now()); err != nil {
		a.notice(locales.CommonTimestampError(err.Error()))
	}
}

func (a *App) flushManagedTimestamp() {
	if a.noteEditor() == nil {
		return
	}
	if err := a.documentWorkflows().FlushManagedTimestamp(); err != nil {
		a.notice(locales.CommonTimestampError(err.Error()))
	}
}
