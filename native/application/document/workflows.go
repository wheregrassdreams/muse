package document

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	domain "muse/document"
	doctypes "muse/document/types"
)

var (
	ErrWorkflowUnavailable = errors.New("document workflow is unavailable")
	ErrNotText             = errors.New("document source is not valid text")
	ErrMergeConflict       = errors.New("external changes overlap with local edits")
	ErrSaveAsRequired      = errors.New("document needs a destination path")
	ErrStaleSavePlan       = errors.New("document changed while preparing the save")
	ErrUnsupportedPath     = errors.New("document path is not editable")
)

type EditingSnapshot struct {
	Path        string
	Source      string
	DiskSource  string
	BaseSource  string
	Revision    domain.Revision
	Dirty       bool
	Saving      bool
	InFlight    SaveRequest
	HasInFlight bool
}

// EditingPort is the active mutable note capability. It contains only source
// and persistence operations; it does not expose widgets, focus, or selection.
type EditingPort interface {
	Snapshot() EditingSnapshot
	PersistenceState() SessionState
	BeginSave(time.Time) (SaveRequest, bool)
	StartSave(SaveRequest)
	CompleteSave(SaveResult) bool
	InvalidatePendingSave(draftKey string) (SaveResult, bool, error)
	DestinationRevision(path string, expected domain.Revision) (domain.Revision, error)
	Rebase(path string, revision domain.Revision, source string) bool
	Relocate(path string, revision domain.Revision, source string) error
	DeferExternal(note domain.Note) bool
	TakeDeferredExternal() (domain.Note, bool)
	ReplaceUser(source string) error
	ReplaceSystem(source string) error
	ClearAutosave()
	ClearMetadataDeadline()
	SetConflict(path string)
	PreserveDraft() (string, error)
	DeleteDraft(path string) error
}

type DirtyTransitionDecision uint8

const (
	DirtyTransitionCancel DirtyTransitionDecision = iota
	DirtyTransitionDiscard
	DirtyTransitionSave
)

type DirtyTransitionOutcome struct {
	Allowed bool
	Saved   bool
	Path    string
}

// SaveAsPlan is a revision-checked snapshot prepared after the UI chooses a
// destination. The UI applies Source through the editor's system-edit port on
// its owner thread, then asks the workflow to commit the same snapshot.
type SaveAsPlan struct {
	Path           string
	Source         string
	OriginalPath   string
	OriginalSource string
	Expected       domain.Revision
	Generation     uint64
}

type ReconcileCurrentOutcome struct {
	Change  ExternalChangeOutcome
	Missing bool
}

type ReplaceDocument func(path, source string, dirty bool) error
type SaveMergedDocument func(path, source string, expected domain.Revision) error

// SessionWorkflows coordinates recovery, history, and external-change
// workflows through repository and editing capabilities. The UI adapter
// supplies only the active session, document replacement, timestamp and save
// capabilities, then presents the returned outcome.
type SessionWorkflows struct {
	Repository domain.Repository
	Recovery   domain.RecoveryRepository
	History    domain.HistoryRepository
	Editing    EditingPort
	Replace    ReplaceDocument
	Save       SaveMergedDocument
}

// Advance moves the active document's autosave state machine forward. The
// UI only schedules this call and presents the resulting errors; deadline
// ordering, managed metadata and save dispatch stay in the application layer.
func (w SessionWorkflows) Advance(now time.Time) error {
	if w.Editing == nil {
		return ErrWorkflowUnavailable
	}
	state := w.Editing.PersistenceState()
	// Autosave must wait for the timestamp patch, but the patch itself should
	// happen at the autosave boundary instead of adding a second full debounce
	// interval. This keeps the established 750 ms save latency and yields one
	// durable write containing both the edit and its metadata.
	if !state.MetadataDue.IsZero() &&
		((!state.AutosaveDue.IsZero() && !now.Before(state.AutosaveDue)) || !now.Before(state.MetadataDue)) {
		return w.FlushManagedTimestampAt(now)
	}
	if request, ok := w.Editing.BeginSave(now); ok {
		w.Editing.StartSave(request)
	}
	return nil
}

// FlushManagedTimestampAt applies the managed timestamp through the editor's
// system-edit capability before a pending save can be claimed.
func (w SessionWorkflows) FlushManagedTimestampAt(now time.Time) error {
	if w.Editing == nil {
		return ErrWorkflowUnavailable
	}
	snapshot := w.Editing.Snapshot()
	if !doctypes.IsMarkdownPath(snapshot.Path) {
		w.Editing.ClearMetadataDeadline()
		return nil
	}
	next, err := UpdateManagedTimestamp(snapshot.Source, now)
	if err != nil {
		return err
	}
	if err := w.Editing.ReplaceSystem(next); err != nil {
		return err
	}
	w.Editing.ClearMetadataDeadline()
	return nil
}

// FlushManagedTimestamp applies a pending managed timestamp immediately,
// typically before a user-requested save or dirty-workspace transition.
func (w SessionWorkflows) FlushManagedTimestamp() error {
	if w.Editing == nil {
		return ErrWorkflowUnavailable
	}
	if w.Editing.PersistenceState().MetadataDue.IsZero() {
		return nil
	}
	return w.FlushManagedTimestampAt(time.Now())
}

// CompleteSave applies a worker result to the active editor session. Stale
// generations and mismatched requests are rejected by the session adapter.
func (w SessionWorkflows) CompleteSave(result SaveResult) bool {
	return w.Editing != nil && w.Editing.CompleteSave(result)
}

// Open reads and validates a note before the presentation replaces its active
// editing session.
func (w SessionWorkflows) Open(path string) (domain.Note, error) {
	return OpenDocument(w.Repository, path)
}

// SaveCurrent commits the current editor snapshot through the document
// session. Metadata ordering and recovery cleanup belong to this workflow.
func (w SessionWorkflows) SaveCurrent() error {
	if w.Editing == nil || w.Save == nil {
		return ErrWorkflowUnavailable
	}
	if err := w.FlushManagedTimestamp(); err != nil {
		return err
	}
	snapshot := w.Editing.Snapshot()
	if snapshot.Path == "" {
		return ErrSaveAsRequired
	}
	if err := w.Save(snapshot.Path, snapshot.DiskSource, snapshot.Revision); err != nil {
		return err
	}
	if !w.Editing.Rebase(snapshot.Path, domain.RevisionOf(snapshot.DiskSource), snapshot.DiskSource) {
		return ErrStaleSavePlan
	}
	return nil
}

// PrepareSaveAs owns document creation metadata and destination revision
// policy. It does not mutate the editor so the caller can marshal the planned
// source replacement to the UI thread before CommitSaveAs writes it.
func (w SessionWorkflows) PrepareSaveAs(path string, now time.Time) (SaveAsPlan, error) {
	if w.Editing == nil {
		return SaveAsPlan{}, ErrWorkflowUnavailable
	}
	if !doctypes.IsEditablePath(path) {
		return SaveAsPlan{}, ErrUnsupportedPath
	}
	if err := w.FlushManagedTimestampAt(now); err != nil {
		return SaveAsPlan{}, err
	}
	snapshot := w.Editing.Snapshot()
	state := w.Editing.PersistenceState()
	source := snapshot.DiskSource
	if snapshot.Path == "" && doctypes.IsMarkdownPath(path) {
		var err error
		source, _, err = EnsureCreatedAt(source, now)
		if err != nil {
			return SaveAsPlan{}, err
		}
		// Frontmatter serialization uses LF. Keep the new document's original
		// newline convention across the metadata insertion instead of creating
		// a mixed-ending file.
		if strings.Contains(snapshot.DiskSource, "\r\n") {
			source = strings.ReplaceAll(strings.ReplaceAll(source, "\r\n", "\n"), "\n", "\r\n")
		}
	}
	expected, err := w.Editing.DestinationRevision(path, snapshot.Revision)
	if err != nil {
		return SaveAsPlan{}, err
	}
	return SaveAsPlan{
		Path: path, Source: source, OriginalPath: snapshot.Path,
		OriginalSource: snapshot.DiskSource, Expected: expected,
		Generation: state.Generation,
	}, nil
}

// ApplySaveAsSource applies any planned metadata through the editor's system
// edit capability. Call on the editor's UI-owner thread.
func (w SessionWorkflows) ApplySaveAsSource(plan SaveAsPlan) error {
	if w.Editing == nil {
		return ErrWorkflowUnavailable
	}
	if !w.matchesSaveAsPlan(plan, false) {
		return ErrStaleSavePlan
	}
	if plan.Source != plan.OriginalSource {
		return w.Editing.ReplaceSystem(strings.ReplaceAll(plan.Source, "\r\n", "\n"))
	}
	return nil
}

// CommitSaveAs writes only the plan that was prepared for the still-active
// document generation. A stale plan never overwrites a newer editor buffer.
func (w SessionWorkflows) CommitSaveAs(plan SaveAsPlan) error {
	if w.Editing == nil || w.Save == nil {
		return ErrWorkflowUnavailable
	}
	if !w.matchesSaveAsPlan(plan, true) {
		return ErrStaleSavePlan
	}
	if err := w.Save(plan.Path, plan.Source, plan.Expected); err != nil {
		return err
	}
	if err := w.Editing.Relocate(plan.Path, domain.RevisionOf(plan.Source), plan.Source); err != nil {
		return err
	}
	key := plan.OriginalPath
	if key == "" {
		key = w.Editing.PersistenceState().DraftKey
	}
	if key != "" {
		_ = w.Editing.DeleteDraft(key)
	}
	return nil
}

func (w SessionWorkflows) matchesSaveAsPlan(plan SaveAsPlan, sourceApplied bool) bool {
	snapshot := w.Editing.Snapshot()
	state := w.Editing.PersistenceState()
	if state.Generation != plan.Generation || snapshot.Path != plan.OriginalPath {
		return false
	}
	if sourceApplied {
		return snapshot.DiskSource == plan.Source
	}
	return snapshot.DiskSource == plan.OriginalSource
}

type MergeOutcome uint8

const (
	MergeNoop MergeOutcome = iota
	MergeLoadedExternal
	MergeSaved
)

// ExternalChangeOutcome describes how the active document session reacted to
// a newer repository revision. The UI adapter may use it for notices and
// cursor restoration, but the merge and recovery decisions stay here.
type ExternalChangeOutcome uint8

const (
	ExternalChangeNoop ExternalChangeOutcome = iota
	ExternalChangeLoaded
	ExternalChangeMergedSaving
	ExternalChangeConflict
	ExternalChangeAcknowledged
	ExternalChangeDeferred
)

func (w SessionWorkflows) ListDrafts() ([]domain.Draft, error) {
	if w.Recovery == nil {
		return nil, errors.New("document recovery repository is not configured")
	}
	return w.Recovery.ListDrafts()
}

// OpenDocument reads a persisted document and rejects non-text sources before
// they reach an editor session. File-type presentation decisions remain with
// the caller.
func OpenDocument(repository domain.Repository, path string) (domain.Note, error) {
	note, err := Read(repository, path)
	if err != nil {
		return domain.Note{}, err
	}
	if !isTextSource(note.Source) {
		return domain.Note{}, ErrNotText
	}
	return note, nil
}

func (w SessionWorkflows) ListHistory(path string) ([]domain.HistoryEntry, error) {
	if path == "" {
		return nil, errors.New("document has not been saved")
	}
	if w.History == nil {
		return nil, errors.New("document history repository is not configured")
	}
	return w.History.ListHistory(path)
}

// ResolveDirtyTransition applies the confirmed user choice before replacing
// the active document. Prompt wording and save-path selection stay in the UI;
// dirty-state checks, destination revision checks, and the save itself remain
// in the document workflow.
func (w SessionWorkflows) ResolveDirtyTransition(decision DirtyTransitionDecision, savePath string) (DirtyTransitionOutcome, error) {
	if w.Editing == nil {
		return DirtyTransitionOutcome{}, ErrWorkflowUnavailable
	}
	if decision == DirtyTransitionSave {
		if err := w.FlushManagedTimestamp(); err != nil {
			return DirtyTransitionOutcome{}, err
		}
	}
	snapshot := w.Editing.Snapshot()
	if !snapshot.Dirty {
		return DirtyTransitionOutcome{Allowed: true, Path: snapshot.Path}, nil
	}
	switch decision {
	case DirtyTransitionCancel:
		return DirtyTransitionOutcome{}, nil
	case DirtyTransitionDiscard:
		key := snapshot.Path
		if key == "" {
			key = w.Editing.PersistenceState().DraftKey
		}
		completed, committed, err := w.Editing.InvalidatePendingSave(key)
		if err != nil {
			return DirtyTransitionOutcome{}, err
		}
		saved := committed && completed.Err == nil &&
			completed.Request.Path == snapshot.Path && completed.Request.Source == snapshot.DiskSource
		return DirtyTransitionOutcome{Allowed: true, Saved: saved, Path: snapshot.Path}, nil
	case DirtyTransitionSave:
		if savePath == "" {
			savePath = snapshot.Path
		}
		if savePath == "" || w.Save == nil {
			return DirtyTransitionOutcome{}, ErrWorkflowUnavailable
		}
		expected, err := w.Editing.DestinationRevision(savePath, snapshot.Revision)
		if err != nil {
			return DirtyTransitionOutcome{}, err
		}
		if err := w.Save(savePath, snapshot.DiskSource, expected); err != nil {
			return DirtyTransitionOutcome{}, err
		}
		if savePath == snapshot.Path {
			if !w.Editing.Rebase(savePath, domain.RevisionOf(snapshot.DiskSource), snapshot.DiskSource) {
				return DirtyTransitionOutcome{}, ErrStaleSavePlan
			}
		} else if err := w.Editing.Relocate(savePath, domain.RevisionOf(snapshot.DiskSource), snapshot.DiskSource); err != nil {
			return DirtyTransitionOutcome{}, err
		}
		key := snapshot.Path
		if key == "" {
			key = w.Editing.PersistenceState().DraftKey
		}
		if key != "" {
			_ = w.Editing.DeleteDraft(key)
		}
		return DirtyTransitionOutcome{Allowed: true, Saved: true, Path: savePath}, nil
	default:
		return DirtyTransitionOutcome{}, ErrWorkflowUnavailable
	}
}

func (w SessionWorkflows) KeepCopy() error {
	if w.Editing == nil || w.Replace == nil {
		return ErrWorkflowUnavailable
	}
	snapshot := w.Editing.Snapshot()
	if err := w.preserveCurrentDraft(snapshot); err != nil {
		return err
	}
	return w.Replace("", snapshot.DiskSource, true)
}

func (w SessionWorkflows) ReloadDisk() error {
	if w.Editing == nil || w.Repository == nil || w.Replace == nil {
		return ErrWorkflowUnavailable
	}
	snapshot := w.Editing.Snapshot()
	if snapshot.Path == "" {
		return errors.New("document has not been saved")
	}
	note, err := w.Repository.Read(snapshot.Path)
	if err != nil {
		return err
	}
	if !isTextSource(note.Source) {
		return ErrNotText
	}
	if err := w.preserveCurrentDraft(snapshot); err != nil {
		return err
	}
	return w.Replace(snapshot.Path, note.Source, false)
}

func (w SessionWorkflows) RestoreDraft(key string) error {
	if w.Editing == nil || w.Recovery == nil || w.Replace == nil {
		return ErrWorkflowUnavailable
	}
	draft, err := w.Recovery.ReadDraft(key)
	if err != nil {
		return err
	}
	if !isTextSource(draft.Text) {
		return ErrNotText
	}
	if err := w.preserveCurrentDraft(w.Editing.Snapshot()); err != nil {
		return err
	}
	return w.Replace("", draft.Text, true)
}

func (w SessionWorkflows) RestoreHistory(path, id string) error {
	if w.Editing == nil || w.History == nil || w.Replace == nil {
		return ErrWorkflowUnavailable
	}
	note, err := w.History.ReadHistory(path, id)
	if err != nil {
		return err
	}
	if !isTextSource(note.Source) {
		return ErrNotText
	}
	if err := w.preserveCurrentDraft(w.Editing.Snapshot()); err != nil {
		return err
	}
	return w.Replace("", note.Source, true)
}

func (w SessionWorkflows) TryMergeExternalChanges() (MergeOutcome, error) {
	if w.Editing == nil || w.Repository == nil || w.Save == nil {
		return MergeNoop, ErrWorkflowUnavailable
	}
	snapshot := w.Editing.Snapshot()
	if snapshot.Path == "" {
		return MergeNoop, errors.New("document has not been saved")
	}
	external, err := w.Repository.Read(snapshot.Path)
	if err != nil {
		return MergeNoop, err
	}
	if !isTextSource(external.Source) {
		return MergeNoop, ErrNotText
	}
	merged, ok := MergeDisjointEdits(snapshot.BaseSource, snapshot.DiskSource, external.Source)
	if !ok {
		return MergeNoop, ErrMergeConflict
	}
	if !w.Editing.Rebase(external.Path, external.Revision, external.Source) {
		return MergeNoop, errors.New("active document changed during merge")
	}
	if err := w.Editing.ReplaceUser(strings.ReplaceAll(merged, "\r\n", "\n")); err != nil {
		return MergeNoop, err
	}
	if !w.Editing.Snapshot().Dirty {
		w.Editing.ClearAutosave()
		if err := w.FlushManagedTimestamp(); err != nil {
			return MergeNoop, err
		}
		if !w.Editing.Snapshot().Dirty {
			return MergeLoadedExternal, nil
		}
	}
	if err := w.FlushManagedTimestamp(); err != nil {
		return MergeNoop, err
	}
	updated := w.Editing.Snapshot()
	if err := w.Save(updated.Path, updated.Source, updated.Revision); err != nil {
		w.Editing.SetConflict(updated.Path)
		return MergeNoop, err
	}
	_ = w.Editing.DeleteDraft(updated.Path)
	return MergeSaved, nil
}

// ReconcileExternal applies an indexed external revision to the active note.
// Dirty buffers are merged only when edits are disjoint and no save is in
// flight. Conflicts preserve a recovery draft before returning to the caller.
func (w SessionWorkflows) ReconcileExternal(external domain.Note) (ExternalChangeOutcome, error) {
	if w.Editing == nil || w.Replace == nil {
		return ExternalChangeNoop, ErrWorkflowUnavailable
	}
	snapshot := w.Editing.Snapshot()
	if external.Path == "" || external.Revision == snapshot.Revision {
		return ExternalChangeNoop, nil
	}
	if snapshot.Dirty || snapshot.Saving {
		if snapshot.Saving {
			if snapshot.HasInFlight && external.Source == snapshot.InFlight.Source {
				return ExternalChangeAcknowledged, nil
			}
			if w.Editing.DeferExternal(external) {
				return ExternalChangeDeferred, nil
			}
			return ExternalChangeNoop, errors.New("active document changed while deferring external revision")
		}
		if external.Source == snapshot.DiskSource {
			if !w.Editing.Rebase(external.Path, external.Revision, external.Source) {
				return ExternalChangeNoop, errors.New("active document changed while acknowledging external revision")
			}
			w.Editing.ClearAutosave()
			return ExternalChangeAcknowledged, nil
		}
		merged, ok := "", false
		if !snapshot.Saving {
			merged, ok = MergeDisjointEdits(snapshot.BaseSource, snapshot.DiskSource, external.Source)
		}
		if ok {
			if !w.Editing.Rebase(external.Path, external.Revision, external.Source) {
				return ExternalChangeNoop, errors.New("active document changed during external merge")
			}
			if err := w.Editing.ReplaceUser(strings.ReplaceAll(merged, "\r\n", "\n")); err != nil {
				return ExternalChangeNoop, err
			}
			if w.Editing.Snapshot().Dirty {
				return ExternalChangeMergedSaving, nil
			}
			w.Editing.ClearAutosave()
			if err := w.FlushManagedTimestamp(); err != nil {
				return ExternalChangeNoop, err
			}
			if !w.Editing.Snapshot().Dirty {
				return ExternalChangeLoaded, nil
			}
			return ExternalChangeMergedSaving, nil
		}
		w.Editing.SetConflict(snapshot.Path)
		if _, err := w.Editing.PreserveDraft(); err != nil {
			return ExternalChangeConflict, err
		}
		return ExternalChangeConflict, nil
	}
	if err := w.Replace(external.Path, external.Source, false); err != nil {
		return ExternalChangeNoop, err
	}
	return ExternalChangeLoaded, nil
}

// ReconcileCurrent rereads the selected Markdown note from the repository and
// applies external-change policy. The watcher/catalog remains an invalidation
// source; its stale payload is never treated as the bytes to reconcile.
func (w SessionWorkflows) ReconcileCurrent(path, vaultRoot string) (ReconcileCurrentOutcome, error) {
	if w.Editing == nil || w.Repository == nil {
		return ReconcileCurrentOutcome{}, ErrWorkflowUnavailable
	}
	snapshot := w.Editing.Snapshot()
	if path == "" || snapshot.Path != path || !doctypes.IsMarkdownPath(path) || vaultRoot == "" {
		return ReconcileCurrentOutcome{}, nil
	}
	rel, err := filepath.Rel(vaultRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ReconcileCurrentOutcome{}, nil
	}
	external, err := w.Repository.Read(path)
	if errors.Is(err, fs.ErrNotExist) {
		if markErr := w.MarkExternalMissing(); markErr != nil {
			return ReconcileCurrentOutcome{Missing: true}, markErr
		}
		return ReconcileCurrentOutcome{Missing: true}, nil
	}
	if err != nil {
		return ReconcileCurrentOutcome{}, err
	}
	change, err := w.ReconcileExternal(external)
	return ReconcileCurrentOutcome{Change: change}, err
}

// MarkExternalMissing records the conflict state when the active file was
// removed from the workspace catalog.
func (w SessionWorkflows) MarkExternalMissing() error {
	if w.Editing == nil {
		return ErrWorkflowUnavailable
	}
	snapshot := w.Editing.Snapshot()
	if snapshot.Path == "" {
		return nil
	}
	w.Editing.SetConflict(snapshot.Path)
	return nil
}

func (w SessionWorkflows) preserveCurrentDraft(snapshot EditingSnapshot) error {
	if !snapshot.Dirty {
		return nil
	}
	if err := w.FlushManagedTimestamp(); err != nil {
		return err
	}
	if w.Editing == nil {
		return ErrWorkflowUnavailable
	}
	_, err := w.Editing.PreserveDraft()
	return err
}

func isTextSource(source string) bool {
	return !strings.ContainsRune(source, '\x00') && utf8.ValidString(source)
}
