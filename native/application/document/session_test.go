package document

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	domain "muse/document"
	"muse/platform/filesystem/documentstore"
)

func TestSessionDebouncesAutosaveAndTracksGeneration(t *testing.T) {
	session := NewSession(nil, nil, nil, nil)
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	session.Open("notes/a.md", "before")
	first := session.Snapshot()
	session.Changed(now)
	if _, ok := session.BeginSave("notes/a.md", "after", true, now.Add(AutosaveDelay-time.Nanosecond)); ok {
		t.Fatal("save started before debounce deadline")
	}
	if _, ok := session.BeginSave("notes/a.md", "after", true, now.Add(AutosaveDelay)); ok {
		t.Fatal("autosave started before its managed metadata patch settled")
	}
	session.ClearMetadataDeadline()
	session.MetadataChanged(now.Add(MetadataUpdateDelay))
	request, ok := session.BeginSave("notes/a.md", "after with metadata", true, now.Add(MetadataUpdateDelay+time.Nanosecond))
	if !ok {
		t.Fatal("save did not start at debounce deadline")
	}
	if request.Generation != first.Generation || request.Expected != domain.RevisionOf("before") {
		t.Fatalf("save request = %+v, session = %+v", request, first)
	}
	if _, ok := session.BeginSave("notes/a.md", "after", true, now.Add(time.Second)); ok {
		t.Fatal("a second save started while the first was in flight")
	}
	session.Open("notes/b.md", "new session")
	if session.CompleteSave(SaveResult{Request: request}, "notes/a.md") {
		t.Fatal("stale save completion was accepted after switching documents")
	}
	if got := session.Snapshot(); got.Saving {
		t.Fatalf("new session remained marked saving after switch: %+v", got)
	}
}

func TestSessionMetadataPatchPreservesPendingAutosaveDeadline(t *testing.T) {
	session := NewSession(nil, nil, nil, nil)
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	session.Changed(now)
	due := session.Snapshot().AutosaveDue
	session.ClearMetadataDeadline()
	session.MetadataChanged(now.Add(MetadataUpdateDelay))
	state := session.Snapshot()
	if !state.MetadataDue.IsZero() {
		t.Fatalf("metadata patch left another timestamp update scheduled: %v", state.MetadataDue)
	}
	if !state.AutosaveDue.Equal(due) {
		t.Fatalf("metadata patch moved pending autosave from %v to %v", due, state.AutosaveDue)
	}
}

func TestSessionMetadataPatchDuringSaveQueuesOneFollowup(t *testing.T) {
	session := NewSession(nil, nil, nil, nil)
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	session.Open("notes/a.md", "before")
	session.Changed(now)
	if _, ok := session.BeginSave("notes/a.md", "user edit", true, now.Add(AutosaveDelay)); ok {
		t.Fatal("initial autosave started before metadata update")
	}
	session.ClearMetadataDeadline()
	request, ok := session.BeginSave("notes/a.md", "user edit with metadata", true, now.Add(MetadataUpdateDelay))
	if !ok {
		t.Fatal("autosave did not start after the metadata deadline cleared")
	}
	session.Changed(now.Add(MetadataUpdateDelay + time.Second))
	session.ClearMetadataDeadline()
	session.MetadataChanged(now.Add(2 * time.Second))
	state := session.Snapshot()
	want := now.Add(MetadataUpdateDelay + time.Second + AutosaveDelay)
	if !state.AutosaveDue.Equal(want) || !state.Saving || !state.HasInFlight || state.InFlight != request {
		t.Fatalf("system patch during save state = %+v, want one follow-up at %v", state, want)
	}
}

func TestSessionPublishesAsyncSaveResults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := documentstore.New(filepath.Join(t.TempDir(), "recovery"))
	session := NewSession(store, store, store, nil)
	session.Open(path, "before")
	now := time.Now()
	session.Changed(now)
	session.ClearMetadataDeadline()
	session.MetadataChanged(now.Add(MetadataUpdateDelay))
	request, ok := session.BeginSave(path, "after", true, now.Add(AutosaveDelay))
	if !ok {
		t.Fatal("save request was not claimed")
	}
	session.StartSave(request)
	select {
	case result := <-session.Results():
		if result.Err != nil || result.Note.Source != "after" || result.Note.Path != path {
			t.Fatalf("save result = %+v", result)
		}
		if !session.CompleteSave(result, path) {
			t.Fatal("current save result was rejected")
		}
		state := session.Snapshot()
		if state.Saving || state.Revision != result.Note.Revision || state.BaseSource != "after" {
			t.Fatalf("completed state = %+v", state)
		}
	case <-time.After(time.Second):
		t.Fatal("session did not publish the asynchronous save result")
	}
}

func TestSessionAcceptsSaveCompletionWhenBufferAdvancedInFlight(t *testing.T) {
	session := NewSession(nil, nil, nil, nil)
	path := "notes/a.md"
	session.Open(path, "before")
	now := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	session.Changed(now)
	session.ClearMetadataDeadline()
	request, ok := session.BeginSave(path, "saved revision", true, now.Add(AutosaveDelay))
	if !ok {
		t.Fatal("save request was not claimed")
	}
	// This is the source currently in the editor after the write was issued.
	session.Changed(now.Add(AutosaveDelay + time.Millisecond))

	result := SaveResult{
		Request: request,
		Note:    domain.NewNote(path, request.Source, domain.RevisionOf(request.Source)),
	}
	if !session.CompleteSave(result, path) {
		t.Fatal("matching save completion was rejected after the buffer advanced")
	}
	state := session.Snapshot()
	if state.Saving || state.Revision != result.Note.Revision || state.BaseSource != request.Source {
		t.Fatalf("completed state did not advance the durable baseline: %+v", state)
	}
	if state.AutosaveDue.IsZero() {
		t.Fatal("completion discarded the follow-up autosave deadline for the newer edit")
	}
}

func TestSessionOwnsDraftHistoryAndDestinationOperations(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := documentstore.New(filepath.Join(root, "recovery"))
	session := NewSession(store, store, store, nil)
	session.Open("", "untitled")
	key, err := session.PreserveDraft("", "untitled draft")
	if err != nil || key == "" {
		t.Fatalf("preserve untitled draft key=%q error=%v", key, err)
	}
	drafts, err := session.ListDrafts()
	if err != nil || len(drafts) != 1 || drafts[0].Key != key {
		t.Fatalf("drafts=%+v error=%v", drafts, err)
	}
	loaded, err := session.ReadDraft(key)
	if err != nil || loaded.Text != "untitled draft" {
		t.Fatalf("draft=%+v error=%v", loaded, err)
	}

	if _, err := store.Save(path, "saved", domain.RevisionOf("before")); err != nil {
		t.Fatal(err)
	}
	history, err := session.ListHistory(path)
	if err != nil || len(history) != 1 {
		t.Fatalf("history=%+v error=%v", history, err)
	}
	previous, err := session.ReadHistory(path, history[0].ID)
	if err != nil || previous.Source != "before" {
		t.Fatalf("history note=%+v error=%v", previous, err)
	}

	if revision, err := session.DestinationRevision(path, path, domain.RevisionOf("open")); err != nil || revision != domain.RevisionOf("open") {
		t.Fatalf("current path revision=%q error=%v", revision, err)
	}
	if revision, err := session.DestinationRevision(filepath.Join(root, "new.md"), path, "initial"); err != nil || revision != "" {
		t.Fatalf("new destination revision=%q error=%v", revision, err)
	}
	if revision, err := session.DestinationRevision(path, filepath.Join(root, "other.md"), "initial"); err != nil || revision != domain.RevisionOf("saved") {
		t.Fatalf("existing destination revision=%q error=%v", revision, err)
	}
}

func TestSessionLoadReadsCurrentRepositoryRevisionWithoutChangingSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("current on disk"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := documentstore.New(filepath.Join(t.TempDir(), "recovery"))
	session := NewSession(store, store, store, nil)
	session.Open(path, "older buffer")
	before := session.Snapshot()

	note, err := session.Load(path)
	if err != nil || note.Source != "current on disk" || note.Revision != domain.RevisionOf(note.Source) {
		t.Fatalf("loaded note=%+v error=%v", note, err)
	}
	if after := session.Snapshot(); !reflect.DeepEqual(after, before) {
		t.Fatalf("read-only load changed persistence session: before=%+v after=%+v", before, after)
	}
}

func TestReadRejectsMissingRepository(t *testing.T) {
	if _, err := Read(nil, "note.md"); err == nil {
		t.Fatal("read without a repository unexpectedly succeeded")
	}
}

func TestSessionSaveNowForOtherDocumentDoesNotReplaceActiveRevision(t *testing.T) {
	root := t.TempDir()
	activePath := filepath.Join(root, "active.md")
	otherPath := filepath.Join(root, "other.md")
	for path, source := range map[string]string{activePath: "active", otherPath: "other"} {
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store := documentstore.New(filepath.Join(root, "recovery"))
	session := NewSession(store, store, store, nil)
	session.Open(activePath, "active")
	activeRevision := session.Snapshot().Revision
	if _, err := session.SaveNow(otherPath, otherPath, "other updated", domain.RevisionOf("other")); err != nil {
		t.Fatal(err)
	}
	state := session.Snapshot()
	if state.Path != activePath || state.Revision != activeRevision || state.BaseSource != "active" {
		t.Fatalf("saving another note changed active session state: %+v", state)
	}
}

func TestSessionCloseInvalidatesClaimedSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := documentstore.New(filepath.Join(t.TempDir(), "recovery"))
	session := NewSession(store, store, store, nil)
	session.Open(path, "before")
	now := time.Now()
	session.Changed(now)
	session.ClearMetadataDeadline()
	session.MetadataChanged(now.Add(MetadataUpdateDelay))
	request, ok := session.BeginSave(path, "after", true, now.Add(AutosaveDelay))
	if !ok {
		t.Fatal("save request was not claimed")
	}
	session.Close()
	result := session.Save(request)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Note.Path != "" || string(data) != "before" {
		t.Fatalf("closed session save result=%+v disk=%q", result, data)
	}
	state := session.Snapshot()
	if state.Saving || !state.AutosaveDue.IsZero() || !state.MetadataDue.IsZero() {
		t.Fatalf("closed session retained pending work: %+v", state)
	}
}

type blockingSaveRepository struct {
	domain.Repository
	entered chan struct{}
	release chan struct{}
	saves   int
}

func (r *blockingSaveRepository) Save(path, source string, expected domain.Revision) (domain.Revision, error) {
	r.saves++
	close(r.entered)
	<-r.release
	return r.Repository.Save(path, source, expected)
}

func TestInvalidatePendingSaveDrainsWriterAndRejectsQueuedGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := documentstore.New(filepath.Join(t.TempDir(), "recovery"))
	repository := &blockingSaveRepository{
		Repository: store,
		entered:    make(chan struct{}),
		release:    make(chan struct{}),
	}
	session := NewSession(repository, store, store, nil)
	session.Open(path, "before")
	now := time.Now()
	session.Changed(now)
	session.ClearMetadataDeadline()
	request, ok := session.BeginSave(path, "after", true, now.Add(AutosaveDelay))
	if !ok {
		t.Fatal("save request was not claimed")
	}
	writeDone := make(chan SaveResult, 1)
	go func() { writeDone <- session.Save(request) }()
	<-repository.entered // The writer passed its generation check and owns saveMu.

	transitionStarted := make(chan struct{})
	transitionDone := make(chan struct {
		result    SaveResult
		committed bool
		err       error
	}, 1)
	go func() {
		close(transitionStarted)
		result, committed, err := session.InvalidatePendingSave("")
		transitionDone <- struct {
			result    SaveResult
			committed bool
			err       error
		}{result, committed, err}
	}()
	<-transitionStarted
	select {
	case <-transitionDone:
		t.Fatal("discard returned while a durable writer still held the commit gate")
	case <-time.After(30 * time.Millisecond):
	}
	close(repository.release)
	if result := <-writeDone; result.Err != nil {
		t.Fatalf("writer failed: %v", result.Err)
	}
	transition := <-transitionDone
	if transition.err != nil {
		t.Fatalf("invalidate pending save: %v", transition.err)
	}
	if !transition.committed || transition.result.Request != request || transition.result.Note.Source != "after" {
		t.Fatalf("transition did not report the committed active buffer: %+v", transition)
	}
	if stale := session.Save(request); stale.Note.Path != "" {
		t.Fatalf("stale queued request wrote after invalidation: %+v", stale)
	}
	if repository.saves != 1 {
		t.Fatalf("durable writes = %d, want exactly one before discard returns", repository.saves)
	}
}

type blockingDraftRecovery struct {
	domain.RecoveryRepository
	entered chan struct{}
	release chan struct{}
	writes  int
}

func (r *blockingDraftRecovery) WriteDraft(key, path, source string) error {
	r.writes++
	close(r.entered)
	<-r.release
	return r.RecoveryRepository.WriteDraft(key, path, source)
}

func TestInvalidatePendingSaveDeletesDraftAfterBlockedConflictWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	store := documentstore.New(filepath.Join(t.TempDir(), "recovery"))
	recovery := &blockingDraftRecovery{
		RecoveryRepository: store,
		entered:            make(chan struct{}),
		release:            make(chan struct{}),
	}
	session := NewSession(store, recovery, store, nil)
	session.Open(path, "before")
	session.SetConflict(path)
	now := time.Now()
	session.Changed(now)
	session.ClearMetadataDeadline()
	request, ok := session.BeginSave(path, "local draft", true, now.Add(AutosaveDelay))
	if !ok || !request.Conflict {
		t.Fatalf("conflict save request = %+v, ok=%t", request, ok)
	}
	writeDone := make(chan SaveResult, 1)
	go func() { writeDone <- session.Save(request) }()
	<-recovery.entered
	transitionDone := make(chan error, 1)
	go func() {
		_, _, err := session.InvalidatePendingSave(path)
		transitionDone <- err
	}()
	select {
	case err := <-transitionDone:
		t.Fatalf("discard returned before conflict draft write drained: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(recovery.release)
	if result := <-writeDone; result.Err != nil {
		t.Fatalf("conflict draft write failed: %v", result.Err)
	}
	if err := <-transitionDone; err != nil {
		t.Fatalf("invalidate and delete draft: %v", err)
	}
	if _, err := recovery.ReadDraft(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("discard left a recovery draft: %v", err)
	}
	if stale := session.Save(request); stale.Err != nil || stale.Request != request {
		t.Fatalf("stale request result = %+v", stale)
	}
	if recovery.writes != 1 {
		t.Fatalf("recovery writes = %d, want exactly one before deletion", recovery.writes)
	}
}

func TestUpdateManagedTimestampPreservesCoreMetadata(t *testing.T) {
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	updated, err := UpdateManagedTimestamp("# note", now)
	if err != nil {
		t.Fatal(err)
	}
	metadata := domain.NewNote("note.md", updated, domain.RevisionOf(updated)).Metadata
	if metadata.Type == "" || metadata.UpdatedAt == "" || metadata.UpdatedAt != now.Format(time.RFC3339Nano) {
		t.Fatalf("updated metadata = %+v", metadata)
	}
}

func TestEnsureCreatedAtPreservesAuthoredTimestampOrAddsFallback(t *testing.T) {
	fallback := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	updated, createdAt, err := EnsureCreatedAt("---\ncreated_at: 2020-01-02T03:04:05Z\n---\nbody", fallback)
	if err != nil || createdAt != "2020-01-02T03:04:05Z" || !strings.Contains(updated, "created_at: 2020-01-02T03:04:05Z") {
		t.Fatalf("authored created_at result=%q value=%q error=%v", updated, createdAt, err)
	}
	updated, createdAt, err = EnsureCreatedAt("# body", fallback)
	if err != nil || createdAt != fallback.Format(time.RFC3339Nano) || !strings.Contains(updated, createdAt) {
		t.Fatalf("fallback created_at result=%q value=%q error=%v", updated, createdAt, err)
	}
}
