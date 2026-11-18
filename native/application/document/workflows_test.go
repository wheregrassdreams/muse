package document

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain "muse/document"
)

type workflowEditing struct {
	snapshot    EditingSnapshot
	state       SessionState
	destination domain.Revision
	draftKeys   []string
	deleted     []string
	conflict    string
	external    domain.Note
	hasExternal bool
	completed   SaveResult
	committed   bool
}

func (e *workflowEditing) Snapshot() EditingSnapshot               { return e.snapshot }
func (e *workflowEditing) PersistenceState() SessionState          { return e.state }
func (e *workflowEditing) BeginSave(time.Time) (SaveRequest, bool) { return SaveRequest{}, false }
func (e *workflowEditing) StartSave(SaveRequest)                   {}
func (e *workflowEditing) CompleteSave(SaveResult) bool            { return false }
func (e *workflowEditing) InvalidatePendingSave(key string) (SaveResult, bool, error) {
	e.state.Generation++
	e.state.Saving = false
	e.state.HasInFlight = false
	e.state.InFlight = SaveRequest{}
	e.state.AutosaveDue = time.Time{}
	e.state.MetadataDue = time.Time{}
	e.state.ConflictPath = ""
	e.conflict = ""
	e.deleted = append(e.deleted, key)
	return e.completed, e.committed, nil
}
func (e *workflowEditing) DestinationRevision(string, domain.Revision) (domain.Revision, error) {
	return e.destination, nil
}
func (e *workflowEditing) Rebase(path string, revision domain.Revision, source string) bool {
	if e.snapshot.Path != path {
		return false
	}
	e.snapshot.Revision, e.snapshot.BaseSource = revision, source
	return true
}
func (e *workflowEditing) Relocate(path string, revision domain.Revision, source string) error {
	e.snapshot.Path, e.snapshot.Revision = path, revision
	e.snapshot.Source, e.snapshot.DiskSource = source, source
	e.snapshot.BaseSource, e.snapshot.Dirty = source, false
	e.state.Generation++
	return nil
}
func (e *workflowEditing) DeferExternal(note domain.Note) bool {
	e.external, e.hasExternal = note, true
	return true
}
func (e *workflowEditing) TakeDeferredExternal() (domain.Note, bool) {
	note, ok := e.external, e.hasExternal
	e.external, e.hasExternal = domain.Note{}, false
	return note, ok
}
func (e *workflowEditing) ReplaceUser(source string) error {
	e.snapshot.Source, e.snapshot.DiskSource = source, source
	e.snapshot.Dirty = source != e.snapshot.BaseSource
	return nil
}
func (e *workflowEditing) ReplaceSystem(source string) error {
	crlf := strings.Contains(e.snapshot.DiskSource, "\r\n")
	e.snapshot.Source = strings.ReplaceAll(source, "\r\n", "\n")
	e.snapshot.DiskSource = e.snapshot.Source
	if crlf {
		e.snapshot.DiskSource = strings.ReplaceAll(e.snapshot.Source, "\n", "\r\n")
	}
	e.snapshot.Dirty = source != e.snapshot.BaseSource
	return nil
}
func (e *workflowEditing) ClearAutosave() {
	e.state.AutosaveDue = time.Time{}
	e.state.MetadataDue = time.Time{}
}
func (e *workflowEditing) ClearMetadataDeadline() { e.state.MetadataDue = time.Time{} }
func (e *workflowEditing) SetConflict(path string) {
	e.conflict = path
	e.state.ConflictPath = path
}
func (e *workflowEditing) PreserveDraft() (string, error) {
	key := e.snapshot.Path
	if key == "" {
		key = e.state.DraftKey
	}
	e.draftKeys = append(e.draftKeys, key)
	return key, nil
}
func (e *workflowEditing) DeleteDraft(path string) error {
	e.deleted = append(e.deleted, path)
	return nil
}

func TestSaveAsPlanAppliesCreatedAtPreservingCRLFAndCommitsOldDraftKey(t *testing.T) {
	now := time.Date(2026, 11, 18, 9, 30, 0, 0, time.UTC)
	editing := &workflowEditing{
		snapshot: EditingSnapshot{Source: "# note\nbody\n", DiskSource: "# note\r\nbody\r\n", Dirty: true},
		state:    SessionState{Generation: 7, DraftKey: "untitled-old"},
	}
	var savedPath, savedSource string
	workflow := SessionWorkflows{
		Editing: editing,
		Save: func(path, source string, _ domain.Revision) error {
			savedPath, savedSource = path, source
			return nil
		},
	}
	path := filepath.Join(t.TempDir(), "created.md")
	plan, err := workflow.PrepareSaveAs(path, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Source, "created_at:") || !strings.Contains(plan.Source, "\r\n") {
		t.Fatalf("planned source lost creation metadata or CRLF: %q", plan.Source)
	}
	if err := workflow.ApplySaveAsSource(plan); err != nil {
		t.Fatal(err)
	}
	if got := editing.snapshot.DiskSource; got != plan.Source {
		t.Fatalf("applied disk source = %q, want plan source %q", got, plan.Source)
	}
	if err := workflow.CommitSaveAs(plan); err != nil {
		t.Fatal(err)
	}
	if savedPath != path || savedSource != plan.Source {
		t.Fatalf("save = (%q, %q), want (%q, %q)", savedPath, savedSource, path, plan.Source)
	}
	if editing.snapshot.Path != path || editing.snapshot.Dirty {
		t.Fatalf("save-as did not commit the editor identity: %+v", editing.snapshot)
	}
	if len(editing.deleted) != 1 || editing.deleted[0] != "untitled-old" {
		t.Fatalf("deleted drafts = %v, want only old untitled recovery key", editing.deleted)
	}
}

func TestDiscardReportsAlreadyCommittedCurrentBufferAsSaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	source := "latest buffer"
	editing := &workflowEditing{
		snapshot: EditingSnapshot{Path: path, Dirty: true, Source: source, DiskSource: source},
		state:    SessionState{Generation: 5, DraftKey: path},
		completed: SaveResult{
			Request: SaveRequest{Path: path, Source: source},
			Note:    domain.NewNote(path, source, domain.RevisionOf(source)),
		},
		committed: true,
	}
	outcome, err := (SessionWorkflows{Editing: editing}).ResolveDirtyTransition(DirtyTransitionDiscard, "")
	if err != nil || !outcome.Allowed || !outcome.Saved || outcome.Path != path {
		t.Fatalf("discard of committed buffer outcome=%+v err=%v", outcome, err)
	}
}

func TestSaveAsPlanRejectsChangedBufferAndGenerationBeforeWriting(t *testing.T) {
	for _, mutate := range []func(*workflowEditing){
		func(e *workflowEditing) { e.snapshot.DiskSource = "newer source" },
		func(e *workflowEditing) { e.state.Generation++ },
	} {
		editing := &workflowEditing{snapshot: EditingSnapshot{Source: "draft", DiskSource: "draft"}, state: SessionState{Generation: 3, DraftKey: "draft"}}
		writes := 0
		workflow := SessionWorkflows{Editing: editing, Save: func(string, string, domain.Revision) error { writes++; return nil }}
		plan, err := workflow.PrepareSaveAs(filepath.Join(t.TempDir(), "note.txt"), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := workflow.ApplySaveAsSource(plan); err != nil {
			t.Fatal(err)
		}
		mutate(editing)
		if err := workflow.CommitSaveAs(plan); !errors.Is(err, ErrStaleSavePlan) {
			t.Fatalf("stale plan commit error = %v, want ErrStaleSavePlan", err)
		}
		if writes != 0 {
			t.Fatalf("stale plan wrote %d times", writes)
		}
	}
}

func TestDiscardTransitionClearsRecoveryAndPersistenceState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	editing := &workflowEditing{
		snapshot: EditingSnapshot{Path: path, Dirty: true, Source: "draft", DiskSource: "draft"},
		state: SessionState{
			Generation: 1, DraftKey: "untitled", AutosaveDue: time.Now(),
			MetadataDue: time.Now(), ConflictPath: path,
		},
	}
	workflow := SessionWorkflows{Editing: editing}
	outcome, err := workflow.ResolveDirtyTransition(DirtyTransitionDiscard, "")
	if err != nil || !outcome.Allowed {
		t.Fatalf("discard outcome=%+v err=%v", outcome, err)
	}
	if len(editing.deleted) != 1 || editing.deleted[0] != path {
		t.Fatalf("deleted drafts = %v, want current note %q", editing.deleted, path)
	}
	if !editing.state.AutosaveDue.IsZero() || !editing.state.MetadataDue.IsZero() || editing.conflict != "" {
		t.Fatalf("discard left persistence work/conflict active: state=%+v conflict=%q", editing.state, editing.conflict)
	}
	if editing.state.Saving || editing.state.HasInFlight || editing.state.Generation != 2 {
		t.Fatalf("discard did not invalidate claimed save: %+v", editing.state)
	}
}

type workflowReadRepository struct {
	note domain.Note
	err  error
}

func (r workflowReadRepository) Read(string) (domain.Note, error) { return r.note, r.err }
func (r workflowReadRepository) Save(string, string, domain.Revision) (domain.Revision, error) {
	return "", nil
}

func TestReconcileCurrentRereadsRepositoryAndMarksMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	root := filepath.Dir(path)
	editing := &workflowEditing{snapshot: EditingSnapshot{Path: path, BaseSource: "old", Source: "old", DiskSource: "old"}}
	workflow := SessionWorkflows{
		Editing:    editing,
		Repository: workflowReadRepository{note: domain.NewNote(path, "new on disk", domain.RevisionOf("new on disk"))},
		Replace: func(path, source string, dirty bool) error {
			editing.snapshot.Path = path
			editing.snapshot.Source, editing.snapshot.DiskSource = source, source
			editing.snapshot.Dirty = dirty
			return nil
		},
	}
	result, err := workflow.ReconcileCurrent(path, root)
	if err != nil || result.Change != ExternalChangeLoaded || result.Missing {
		t.Fatalf("reload outcome=%+v err=%v", result, err)
	}
	if editing.snapshot.DiskSource != "new on disk" {
		t.Fatalf("repository source was not loaded: %q", editing.snapshot.DiskSource)
	}

	editing.snapshot.Path = path
	workflow.Repository = workflowReadRepository{err: os.ErrNotExist}
	result, err = workflow.ReconcileCurrent(path, root)
	if err != nil || !result.Missing || editing.conflict != path {
		t.Fatalf("missing outcome=%+v conflict=%q err=%v", result, editing.conflict, err)
	}
}
