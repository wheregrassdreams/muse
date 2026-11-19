package noteeditor

import (
	"errors"
	"testing"
	"time"

	documentapp "muse/application/document"
	"muse/document"
)

type saveActionsEditing struct {
	snapshot documentapp.EditingSnapshot
	state    documentapp.SessionState
}

func (e *saveActionsEditing) Snapshot() documentapp.EditingSnapshot { return e.snapshot }
func (e *saveActionsEditing) PersistenceState() documentapp.SessionState { return e.state }
func (*saveActionsEditing) BeginSave(time.Time) (documentapp.SaveRequest, bool) { return documentapp.SaveRequest{}, false }
func (*saveActionsEditing) StartSave(documentapp.SaveRequest)                  {}
func (*saveActionsEditing) CompleteSave(documentapp.SaveResult) bool             { return false }
func (*saveActionsEditing) InvalidatePendingSave(string) (documentapp.SaveResult, bool, error) {
	return documentapp.SaveResult{}, false, nil
}
func (*saveActionsEditing) DestinationRevision(string, document.Revision) (document.Revision, error) {
	return document.Revision("destination"), nil
}
func (e *saveActionsEditing) Rebase(path string, revision document.Revision, source string) bool {
	e.snapshot.Path, e.snapshot.Revision, e.snapshot.DiskSource = path, revision, source
	return true
}
func (e *saveActionsEditing) Relocate(path string, revision document.Revision, source string) error {
	e.snapshot.Path, e.snapshot.Revision, e.snapshot.DiskSource = path, revision, source
	return nil
}
func (*saveActionsEditing) DeferExternal(document.Note) bool       { return false }
func (*saveActionsEditing) TakeDeferredExternal() (document.Note, bool) { return document.Note{}, false }
func (*saveActionsEditing) ReplaceUser(string) error               { return nil }
func (*saveActionsEditing) ReplaceSystem(string) error             { return nil }
func (*saveActionsEditing) ClearAutosave()                         {}
func (*saveActionsEditing) ClearMetadataDeadline()                 {}
func (*saveActionsEditing) SetConflict(string)                     {}
func (*saveActionsEditing) PreserveDraft() (string, error)          { return "", nil }
func (*saveActionsEditing) DeleteDraft(string) error                { return nil }

func TestSaveActionsRequestsSaveAsForUntitledNote(t *testing.T) {
	editing := &saveActionsEditing{}
	requested := ""
	var reported error
	actions := NewSaveActions(SaveActionPorts{
		Workflows: func() documentapp.SessionWorkflows {
			return documentapp.SessionWorkflows{Editing: editing, Save: func(string, string, document.Revision) error { return nil }}
		},
		CurrentPath:  func() string { return "" },
		UntitledName: func() string { return "Untitled note" },
		RequestSaveAs: func(path string) { requested = path },
		NotifyError:   func(err error) { reported = err },
	})

	actions.Save()

	if requested != "Untitled note.md" {
		t.Fatalf("Save As default path = %q", requested)
	}
	if reported != nil {
		t.Fatalf("Save unexpectedly reported an error: %v", reported)
	}
}

func TestSaveActionsCommitSaveAsThroughApplicationWorkflow(t *testing.T) {
	editing := &saveActionsEditing{snapshot: documentapp.EditingSnapshot{DiskSource: "body"}}
	var savedPath string
	var selectedPath string
	var notificationCount, invalidations int
	actions := NewSaveActions(SaveActionPorts{
		Workflows: func() documentapp.SessionWorkflows {
			return documentapp.SessionWorkflows{
				Editing: editing,
				Save: func(path, source string, expected document.Revision) error {
					if path != "/vault/Note.md" || source != "body" || expected != "destination" {
						return errors.New("unexpected save request")
					}
					return nil
				},
			}
		},
		RecordSavedPath: func(path string) { savedPath = path },
		SelectPath:      func(path string) { selectedPath = path },
		NotifySaved:     func() { notificationCount++ },
		Invalidate:      func() { invalidations++ },
	})

	actions.CommitSaveAs("/vault/Note.md")

	if savedPath != "/vault/Note.md" || selectedPath != savedPath || editing.snapshot.Path != savedPath {
		t.Fatalf("save-as path state: recorded=%q selected=%q editor=%q", savedPath, selectedPath, editing.snapshot.Path)
	}
	if notificationCount != 1 || invalidations != 1 {
		t.Fatalf("save-as effects: notifications=%d invalidations=%d", notificationCount, invalidations)
	}
}
