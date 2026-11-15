package nativeui

import (
	"os"
	"strings"

	"github.com/egoist/mygo/ui"
	documentsave "muse/application/document"
	"muse/document"
	"muse/locales"
	"muse/nativeui/components/documentdialogs"
)

func (a *App) openRecovery() {
	if a.documentSession == nil {
		a.notice(locales.CommonRecoveryReadError("document services are not configured"))
		return
	}
	drafts, err := a.documentSession.ListDrafts()
	if err != nil {
		a.notice(locales.CommonRecoveryReadError(err.Error()))
		return
	}
	a.documents.OpenRecovery(drafts)
}

func (a *App) openHistory() {
	if a.path == "" {
		a.notice(locales.DocumentUnsaved())
		return
	}
	if a.documentSession == nil {
		a.notice(locales.CommonHistoryReadError("document services are not configured"))
		return
	}
	entries, err := a.documentSession.ListHistory(a.path)
	if err != nil {
		a.notice(locales.CommonHistoryReadError(err.Error()))
		return
	}
	a.documents.OpenHistory(a.path, entries)
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
			a.path = ""
			a.loadDoc(source)
			a.dirty = true
			a.documentChanged()
			a.documents.ConflictOpen = false
			a.updateTitle()
		case documentdialogs.ReloadDisk:
			data, err := os.ReadFile(a.path)
			if err != nil {
				a.notice(err.Error())
				return
			}
			if !isTextData(data) {
				a.notice(locales.DocumentExternalNotText())
				return
			}
			if !a.preserveCurrentDraft() {
				return
			}
			a.loadDoc(string(data))
			a.dirty = false
			a.documents.ConflictOpen = false
			a.updateTitle()
		case documentdialogs.RestoreDraft:
			draft, err := a.documentSession.ReadDraft(action.Key)
			if err != nil {
				a.notice(err.Error())
				return
			}
			if !a.preserveCurrentDraft() {
				return
			}
			a.path = ""
			a.loadDoc(draft.Text)
			a.dirty = true
			a.documentChanged()
			a.documents.RecoveryOpen = false
			a.updateTitle()
		case documentdialogs.RestoreHistory:
			note, err := a.documentSession.ReadHistory(a.documents.HistoryPath, action.ID)
			if err != nil {
				a.notice(err.Error())
				return
			}
			if !a.preserveCurrentDraft() {
				return
			}
			a.path = ""
			a.loadDoc(note.Source)
			a.dirty = true
			a.documentChanged()
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
	if a.documentSession == nil || a.path == "" {
		return false
	}
	state := a.documentSession.Snapshot()
	external, err := os.ReadFile(a.path)
	if err != nil || !isTextData(external) {
		a.notice(locales.DocumentExternalReadFailed())
		return false
	}
	externalSource := string(external)
	merged, ok := documentsave.MergeDisjointEdits(state.BaseSource, a.diskSource(), externalSource)
	if !ok {
		a.notice(locales.DocumentExternalMergeConflict())
		return false
	}
	a.documentSession.Rebase(a.path, document.RevisionOf(externalSource), externalSource)
	a.crlf = strings.Contains(externalSource, "\r\n")
	a.ed.ReplaceAll(strings.ReplaceAll(merged, "\r\n", "\n"))
	a.dirty = a.diskSource() != externalSource
	a.documents.ConflictOpen = false
	if !a.dirty {
		a.documentSession.ClearAutosave()
		a.updateManagedTimestamp()
		a.dirty = a.diskSource() != externalSource
		if !a.dirty {
			a.notice(locales.DocumentExternalLoaded())
			return true
		}
	}
	a.flushManagedTimestamp()
	if err := a.writeDocument(a.path, a.diskSource(), state.Revision); err != nil {
		a.documentSession.SetConflict(a.path)
		a.documents.ConflictOpen = true
		a.notice(locales.CommonMergedSaveError(err.Error()))
		return false
	}
	a.documents.ConflictOpen = false
	a.dirty = false
	_ = a.documentSession.DeleteDraft(a.path)
	a.updateTitle()
	a.notice(locales.DocumentExternalMerged())
	return true
}

func (a *App) preserveCurrentDraft() bool {
	if !a.dirty || a.documentSession == nil {
		return true
	}
	a.flushManagedTimestamp()
	if _, err := a.documentSession.PreserveDraft(a.path, a.diskSource()); err != nil {
		a.notice(locales.CommonDraftPreserveError(err.Error()))
		return false
	}
	return true
}
