package nativeui

import (
	"errors"

	"github.com/egoist/mygo/ui"

	documentapp "muse/application/document"
	"muse/document"
	"muse/locales"
	"muse/nativeui/components/documentdialogs"
	"muse/nativeui/surfaces/noteeditor"
)

type noteEditorWorkflowPort struct{ editor noteeditor.Commands }

func (p noteEditorWorkflowPort) Snapshot() documentapp.EditingSnapshot {
	if p.editor == nil {
		return documentapp.EditingSnapshot{}
	}
	snapshot := p.editor.Snapshot()
	persistence := p.editor.PersistenceState()
	return documentapp.EditingSnapshot{
		Path: snapshot.Path, Source: snapshot.Source, DiskSource: snapshot.DiskSource,
		BaseSource: persistence.BaseSource, Revision: snapshot.Revision, Dirty: snapshot.Dirty,
		Saving: persistence.Saving,
	}
}

func (p noteEditorWorkflowPort) DestinationRevision(path string, expected document.Revision) (document.Revision, error) {
	if p.editor == nil {
		return "", documentapp.ErrWorkflowUnavailable
	}
	return p.editor.DestinationRevision(path, expected)
}

func (p noteEditorWorkflowPort) Rebase(path string, revision document.Revision, source string) bool {
	return p.editor != nil && p.editor.Rebase(path, revision, source)
}
func (p noteEditorWorkflowPort) ReplaceUser(source string) error {
	if p.editor == nil {
		return documentapp.ErrWorkflowUnavailable
	}
	return p.editor.ReplaceUser(source)
}
func (p noteEditorWorkflowPort) ClearAutosave() {
	if p.editor != nil {
		p.editor.ClearAutosave()
	}
}
func (p noteEditorWorkflowPort) SetConflict(path string) {
	if p.editor != nil {
		p.editor.SetConflict(path)
	}
}
func (p noteEditorWorkflowPort) PreserveDraft() (string, error) {
	if p.editor == nil {
		return "", documentapp.ErrWorkflowUnavailable
	}
	return p.editor.PreserveDraft()
}
func (p noteEditorWorkflowPort) DeleteDraft(path string) error {
	if p.editor == nil {
		return documentapp.ErrWorkflowUnavailable
	}
	return p.editor.DeleteDraft(path)
}

func (a *App) documentWorkflows() documentapp.SessionWorkflows {
	var active documentapp.EditingPort
	if editor := a.noteEditor(); editor != nil {
		active = noteEditorWorkflowPort{editor: editor}
	}
	return documentapp.SessionWorkflows{
		Repository: a.documentRepository,
		Recovery:   a.documentRecovery,
		History:    a.documentHistory,
		Editing:    active,
		FlushTime:  a.flushManagedTimestamp,
		UpdateTime: a.updateManagedTimestamp,
		Replace: func(path, source string, dirty bool) error {
			if a.noteEditor() == nil {
				return documentapp.ErrWorkflowUnavailable
			}
			a.loadDocumentAt(path, source)
			if dirty {
				a.noteEditor().MarkDirty()
			}
			a.updateTitle()
			return nil
		},
		Save: func(path, source string, expected document.Revision) error {
			return a.writeDocument(path, source, expected)
		},
	}
}

func (a *App) openRecovery() {
	drafts, err := a.documentWorkflows().ListDrafts()
	if err != nil {
		a.notice(locales.CommonRecoveryReadError(err.Error()))
		return
	}
	a.documents.OpenRecovery(drafts)
}

func (a *App) openHistory() {
	entries, err := a.documentWorkflows().ListHistory(a.documentPath())
	if err != nil {
		a.notice(locales.CommonHistoryReadError(err.Error()))
		return
	}
	a.documents.OpenHistory(a.documentPath(), entries)
}

func (a *App) documentDialogs(c *ui.Context) {
	documentdialogs.Render(c, a.documents, func(action documentdialogs.Action) {
		workflows := a.documentWorkflows()
		switch action.Kind {
		case documentdialogs.TryMerge:
			outcome, err := workflows.TryMergeExternalChanges()
			if err != nil {
				switch {
				case errors.Is(err, documentapp.ErrMergeConflict):
					a.notice(locales.DocumentExternalMergeConflict())
				case errors.Is(err, documentapp.ErrNotText):
					a.notice(locales.DocumentExternalReadFailed())
				default:
					a.notice(locales.CommonMergedSaveError(err.Error()))
				}
				return
			}
			a.documents.Apply(documentdialogs.Action{Kind: documentdialogs.CloseConflict})
			a.updateTitle()
			if outcome == documentapp.MergeLoadedExternal {
				a.notice(locales.DocumentExternalLoaded())
			} else if outcome == documentapp.MergeSaved {
				a.notice(locales.DocumentExternalMerged())
			}
		case documentdialogs.KeepCopy:
			if err := workflows.KeepCopy(); err != nil {
				a.notice(locales.CommonDraftPreserveError(err.Error()))
				return
			}
			a.documents.Apply(documentdialogs.Action{Kind: documentdialogs.CloseConflict})
		case documentdialogs.ReloadDisk:
			if err := workflows.ReloadDisk(); err != nil {
				if errors.Is(err, documentapp.ErrNotText) {
					a.notice(locales.DocumentExternalNotText())
				} else {
					a.notice(err.Error())
				}
				return
			}
			a.documents.Apply(documentdialogs.Action{Kind: documentdialogs.CloseConflict})
		case documentdialogs.RestoreDraft:
			if err := workflows.RestoreDraft(action.Key); err != nil {
				a.notice(err.Error())
				return
			}
			a.documents.Apply(action)
		case documentdialogs.RestoreHistory:
			if err := workflows.RestoreHistory(a.documents.HistoryPath(), action.ID); err != nil {
				a.notice(err.Error())
				return
			}
			a.documents.Apply(action)
		case documentdialogs.CloseConflict, documentdialogs.CloseRecovery, documentdialogs.CloseHistory:
			a.documents.Apply(action)
		}
	})
}
