package nativeui

import (
	"strings"
	"sync"
	"testing"
	"time"

	"muse/document"
	"muse/events"
)

type blockingInternalSaveRepository struct {
	document.Repository
	path    string
	entered chan string
	release chan struct{}
	once    sync.Once
	unlock  sync.Once
}

func (r *blockingInternalSaveRepository) unblock() { r.unlock.Do(func() { close(r.release) }) }

func (r *blockingInternalSaveRepository) Save(path, source string, expected document.Revision) (document.Revision, error) {
	revision, err := r.Repository.Save(path, source, expected)
	if err == nil && path == r.path {
		r.once.Do(func() {
			r.entered <- source
			<-r.release
		})
	}
	return revision, err
}

func TestInternalSaveWatcherAcknowledgesInFlightRevisionWhileBufferContinues(t *testing.T) {
	a, tester, path := runtimeFixture(t)
	dispatcher := events.New[document.DocumentSaved]()
	saved := make(chan document.DocumentSaved, 2)
	dispatcher.Subscribe(func(event document.DocumentSaved) { saved <- event })
	a.documentEvents = dispatcher
	a.setEventDispatchers(nil, dispatcher)
	if err := a.createActiveNote(document.NewNote(path, a.documentSource(), a.noteEditor().PersistenceState().Revision), a.noteEditor().Snapshot().CursorOffset); err != nil {
		t.Fatal(err)
	}
	baseRepository := a.documentRepository
	blocking := &blockingInternalSaveRepository{
		Repository: baseRepository,
		path:       path,
		entered:    make(chan string, 1),
		release:    make(chan struct{}),
	}
	a.SetDocumentRepositories(blocking, a.documentRecovery, a.documentHistory)
	t.Cleanup(blocking.unblock)

	beforeIndexRevision := a.vaultSnapshot.IndexRevision
	if err := a.noteEditor().ReplaceUser("first internal edit"); err != nil {
		t.Fatal(err)
	}
	expireAutosave(a)
	tester.Frame() // Applies the managed timestamp system edit.
	tester.Frame() // Claims and starts the autosave.

	var inFlight string
	select {
	case inFlight = <-blocking.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("autosave did not reach the durable repository")
	}
	if !strings.Contains(inFlight, "updated_at:") || !strings.HasSuffix(inFlight, "first internal edit") {
		t.Fatalf("in-flight save omitted system metadata or user edit: %q", inFlight)
	}
	if !a.noteEditor().PersistenceState().Saving {
		t.Fatal("save was not held in flight")
	}

	// Continue typing after the durable write has landed but before the save
	// result is delivered. The filesystem invalidation now describes Muse's own
	// in-flight revision, not an external edit to the newer buffer.
	if err := a.noteEditor().ReplaceUser("second internal edit"); err != nil {
		t.Fatal(err)
	}
	// A watcher snapshot can be stale by the time its event is reconciled.
	// Hold one such observation behind the active save and verify completion
	// accepts the write, drains the invalidation, then rereads the repository.
	a.reconcileExternalNote(document.NewNote(path, "stale watcher snapshot", document.RevisionOf("stale watcher snapshot")))
	if !a.noteEditor().PersistenceState().HasDeferred {
		t.Fatal("non-matching watcher snapshot was not deferred during the in-flight save")
	}
	a.notice("sentinel")
	waitDocument(t, tester, func() bool {
		indexed, ok := a.vault.Document("note.md")
		return ok && indexed.Source == inFlight && a.vaultSnapshot.IndexRevision != beforeIndexRevision
	})
	if a.noteEditor().PersistenceState().ConflictPath != "" || a.documents.IsConflictOpen() {
		t.Fatalf("Muse's in-flight save was surfaced as an external conflict: state=%+v", a.noteEditor().PersistenceState())
	}
	if a.status != "sentinel" {
		t.Fatalf("Muse's in-flight save emitted an external-change notice: %q", a.status)
	}

	blocking.unblock()
	var event document.DocumentSaved
	select {
	case event = <-saved:
	case <-time.After(3 * time.Second):
		t.Fatal("internal save did not publish DocumentSaved")
	}
	if event.Path != path || event.Revision != document.RevisionOf(inFlight) {
		t.Fatalf("DocumentSaved event=%+v, want path %q revision %q", event, path, document.RevisionOf(inFlight))
	}
	waitDocument(t, tester, func() bool {
		state := a.noteEditor().PersistenceState()
		return !state.Saving && !state.HasDeferred && state.Revision == event.Revision && a.noteEditor().Source() == "second internal edit" && a.documentDirty()
	})
	if a.noteEditor().PersistenceState().ConflictPath != "" || a.documents.IsConflictOpen() {
		t.Fatalf("completion of internal save opened a conflict: state=%+v", a.noteEditor().PersistenceState())
	}
}
