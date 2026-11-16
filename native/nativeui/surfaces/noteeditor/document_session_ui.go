package nativeui

import (
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	documentsave "muse/application/document"
	"muse/locales"
	"muse/nativeui/components/documentdialogs"
)

func (a *App) openRecovery() {
	if a.activeNote == nil {
		a.notice(locales.CommonRecoveryReadError("document services are not configured"))
		return
	}
	drafts, err := a.activeNote.ListDrafts()
	if err != nil {
		a.notice(locales.CommonRecoveryReadError(err.Error()))
		return
	}
	a.documents.OpenRecovery(drafts)
}

func (a *App) openHistory() {
	if a.documentPath() == "" {
		a.notice(locales.DocumentUnsaved())
		return
	}
	if a.activeNote == nil {
		a.notice(locales.CommonHistoryReadError("document services are not configured"))
		return
	}
	entries, err := a.activeNote.ListHistory(a.documentPath())
	if err != nil {
		a.notice(locales.CommonHistoryReadError(err.Error()))
		return
	}
	a.documents.OpenHistory(a.documentPath(), entries)
}

func (a *App) documentDialogs(c *ui.Context) {
	documentdialogs.Render(c, a.documents, func(action documentdialogs.Action) {
		switch action.Kind {
		case documentdialogs.TryMerge:
			a.tryMergeExternalChanges()
		case documentdialogs.KeepCopy:
			if !a.preserveCurrentDraft() {
				return
			}
			source := a.diskSource()
			a.loadDocumentAt("", source)
			a.activeNote.MarkDirty()
			a.documents.ConflictOpen = false
			a.updateTitle()
		case documentdialogs.ReloadDisk:
			note, err := documentsave.Read(a.documentRepository, a.documentPath())
			if err != nil {
				a.notice(err.Error())
				return
			}
			if !isTextSource(note.Source) {
				a.notice(locales.DocumentExternalNotText())
				return
			}
			if !a.preserveCurrentDraft() {
				return
			}
			a.loadDoc(note.Source)
			a.documents.ConflictOpen = false
			a.updateTitle()
		case documentdialogs.RestoreDraft:
			draft, err := a.activeNote.ReadDraft(action.Key)
			if err != nil {
				a.notice(err.Error())
				return
			}
			if !a.preserveCurrentDraft() {
				return
			}
			a.loadDocumentAt("", draft.Text)
			a.activeNote.MarkDirty()
			a.documents.RecoveryOpen = false
			a.updateTitle()
		case documentdialogs.RestoreHistory:
			note, err := a.activeNote.ReadHistory(a.documents.HistoryPath, action.ID)
			if err != nil {
				a.notice(err.Error())
				return
			}
			if !a.preserveCurrentDraft() {
				return
			}
			a.loadDocumentAt("", note.Source)
			a.activeNote.MarkDirty()
			a.documents.HistoryOpen = false
			a.updateTitle()
		case documentdialogs.CloseConflict:
			a.documents.ConflictOpen = false
		case documentdialogs.CloseRecovery:
			a.documents.RecoveryOpen = false
		case documentdialogs.CloseHistory:
			a.documents.HistoryOpen = false
		}
	})
}

func (a *App) tryMergeExternalChanges() bool {
	if a.activeNote == nil || a.documentPath() == "" {
		return false
	}
	state := a.activeNote.PersistenceState()
	external, err := documentsave.Read(a.documentRepository, a.documentPath())
	if err != nil || !isTextSource(external.Source) {
		a.notice(locales.DocumentExternalReadFailed())
		return false
	}
	externalSource := external.Source
	merged, ok := documentsave.MergeDisjointEdits(state.BaseSource, a.diskSource(), externalSource)
	if !ok {
		a.notice(locales.DocumentExternalMergeConflict())
		return false
	}
	if !a.activeNote.Rebase(external.Path, external.Revision, externalSource) {
		a.notice(locales.DocumentExternalReadFailed())
		return false
	}
	_ = a.activeNote.ReplaceUser(strings.ReplaceAll(merged, "\r\n", "\n"))
	a.documents.ConflictOpen = false
	if !a.documentDirty() {
		a.activeNote.ClearAutosave()
		a.updateManagedTimestamp()
		if !a.documentDirty() {
			a.notice(locales.DocumentExternalLoaded())
			return true
		}
	}
	a.flushManagedTimestamp()
	if err := a.writeDocument(external.Path, a.diskSource(), external.Revision); err != nil {
		a.activeNote.SetConflict(a.documentPath())
		a.documents.ConflictOpen = true
		a.notice(locales.CommonMergedSaveError(err.Error()))
		return false
	}
	a.documents.ConflictOpen = false
	_ = a.activeNote.DeleteDraft(a.documentPath())
	a.updateTitle()
	a.notice(locales.DocumentExternalMerged())
	return true
}

func isTextSource(source string) bool {
	if strings.IndexByte(source, 0) >= 0 {
		return false
	}
	return utf8.ValidString(source)
}

func (a *App) preserveCurrentDraft() bool {
	if a.activeNote == nil || !a.documentDirty() {
		return true
	}
	a.flushManagedTimestamp()
	if _, err := a.activeNote.PreserveDraft(a.documentPath(), a.diskSource()); err != nil {
		a.notice(locales.CommonDraftPreserveError(err.Error()))
		return false
	}
	return true
}
