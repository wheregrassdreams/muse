package vault

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	vaultfilessource "muse/platform/filesystem/vaultfilessource"
	vaultindexsource "muse/platform/filesystem/vaultindexsource"
	vaultsearchstore "muse/platform/filesystem/vaultsearchstore"
	"muse/search"
	vaultfiles "muse/vault/files"
	vaultindex "muse/vault/index"
	vaultwatch "muse/vault/watch"
)

type sessionTestWatcher struct {
	events chan vaultwatch.Event
	once   sync.Once
	mu     sync.Mutex
	closed int
}

type blockingCatalog struct {
	CatalogPort
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

type inProcessSearchReconciler struct{ store SearchIndexStore }

func (r inProcessSearchReconciler) OpenAndReconcile(ctx context.Context, root string, corpus search.Corpus) (*search.Index, error) {
	index, err := r.store.Open(root)
	if err != nil {
		return nil, err
	}
	if err := index.ReconcileCorpus(ctx, corpus); err != nil {
		_ = index.Close()
		return nil, err
	}
	return index, nil
}

func (r inProcessSearchReconciler) Reconcile(ctx context.Context, root string, current *search.Index, corpus search.Corpus) (*search.Index, error) {
	if current == nil {
		return r.OpenAndReconcile(ctx, root, corpus)
	}
	return current, current.ReconcileCorpus(ctx, corpus)
}

func (c *blockingCatalog) Paths() []string {
	c.once.Do(func() { close(c.entered) })
	<-c.release
	return c.CatalogPort.Paths()
}

func newSessionTestWatcher() *sessionTestWatcher {
	return &sessionTestWatcher{events: make(chan vaultwatch.Event, 16)}
}

func (w *sessionTestWatcher) Events() <-chan vaultwatch.Event { return w.events }
func (w *sessionTestWatcher) WatchFile(string) error          { return nil }
func (w *sessionTestWatcher) Close() error {
	w.once.Do(func() {
		w.mu.Lock()
		w.closed++
		w.mu.Unlock()
		close(w.events)
	})
	return nil
}

func (w *sessionTestWatcher) closeCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closed
}

func sessionTestDependencies(t *testing.T, watcher *sessionTestWatcher) SessionDependencies {
	t.Helper()
	cache, err := vaultsearchstore.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	return SessionDependencies{
		NewIndex: func(root string) (IndexPort, error) {
			source, err := vaultindexsource.New(root)
			if err != nil {
				return nil, err
			}
			return vaultindex.New(source), nil
		},
		NewCatalog: func(root string) (CatalogPort, error) {
			source, err := vaultfilessource.New(root)
			if err != nil {
				return nil, err
			}
			return vaultfiles.New(source), nil
		},
		NewWatcher:       func(string) (WatcherPort, error) { return watcher, nil },
		SearchReconciler: inProcessSearchReconciler{store: cache},
	}
}

func TestSessionExposesCatalogChildrenAndEntry(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "child.md"), []byte("# Child"), 0o600); err != nil {
		t.Fatal(err)
	}
	watcher := newSessionTestWatcher()
	session, err := NewSession(root, sessionTestDependencies(t, watcher))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	waitSession(t, session, func(snapshot Snapshot) bool { return snapshot.Ready })

	entries := session.Children("")
	want := []vaultfiles.Entry{{Path: "empty", Kind: vaultfiles.EntryDirectory}, {Path: "notes", Kind: vaultfiles.EntryDirectory}}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("root children = %+v, want %+v", entries, want)
	}
	if got := session.Children("notes"); !reflect.DeepEqual(got, []vaultfiles.Entry{{Path: "notes/child.md", Kind: vaultfiles.EntryRegular}}) {
		t.Fatalf("notes children = %+v", got)
	}
	if got, ok := session.Entry("notes"); !ok || got.Kind != vaultfiles.EntryDirectory {
		t.Fatalf("notes entry = %+v, %v", got, ok)
	}
}

func waitSession(t *testing.T, session *Session, predicate func(Snapshot) bool) Snapshot {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := session.Snapshot()
		if predicate(snapshot) {
			return snapshot
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for session state: %+v", session.Snapshot())
	return Snapshot{}
}

func TestSessionStartsRefreshesQueriesAndCloses(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "A.md"), []byte("# Alpha\n\nunique-needle [[B]]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "B.md"), []byte("# Beta\n\nBack to [[A]]"), 0o644); err != nil {
		t.Fatal(err)
	}
	watcher := newSessionTestWatcher()
	session, err := NewSession(root, sessionTestDependencies(t, watcher))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	ready := waitSession(t, session, func(snapshot Snapshot) bool { return snapshot.Ready })
	if ready.Error != "" || !ready.SearchAvailable || len(session.Paths()) != 2 {
		t.Fatalf("initial session snapshot = %+v paths=%v", ready, session.Paths())
	}
	if err := session.OpenLinks("B.md"); err != nil {
		t.Fatal(err)
	}
	links := session.Snapshot()
	if len(links.Incoming) != 1 || !links.LinksOpen {
		t.Fatalf("incoming links = %+v", links)
	}
	links.Incoming[0].Source = "mutated"
	if got := session.Snapshot().Incoming[0].Source; got == "mutated" {
		t.Fatal("Snapshot exposed mutable link storage")
	}

	token, err := session.Query("unique-needle", false, filepath.Join(root, "A.md"), "# Alpha\n\nunique-needle [[B]]")
	if err != nil {
		t.Fatal(err)
	}
	queried := waitSession(t, session, func(snapshot Snapshot) bool {
		return !snapshot.Search.Busy && snapshot.Search.Token == token
	})
	if queried.Search.Error != "" || len(queried.Search.Hits) != 1 || queried.Search.Hits[0].Path != "A.md" {
		t.Fatalf("query snapshot = %+v", queried.Search)
	}

	if err := session.ApplyActiveDocument(filepath.Join(root, "A.md"), "# Renamed\n\nlive-buffer-token [[B]]"); err != nil {
		t.Fatal(err)
	}
	if title, source, ok := session.NoteContent("A.md"); !ok || title != "Renamed" || source != "# Renamed\n\nlive-buffer-token [[B]]" {
		t.Fatalf("applied active document = %q %q %t", title, source, ok)
	}

	newPath := filepath.Join(root, "C.md")
	if err := os.WriteFile(newPath, []byte("# Gamma"), 0o644); err != nil {
		t.Fatal(err)
	}
	watcher.events <- vaultwatch.Event{Path: newPath, Kind: vaultwatch.Changed}
	waitSession(t, session, func(Snapshot) bool {
		for _, path := range session.Paths() {
			if path == "C.md" {
				return true
			}
		}
		return false
	})
	if err := session.Refresh("C.md"); err != nil {
		t.Fatalf("manual refresh: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "D.md"), []byte("# Delta"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := session.Refresh("D.md"); err != nil {
		t.Fatalf("manual relative refresh: %v", err)
	}
	waitSession(t, session, func(Snapshot) bool {
		for _, path := range session.Paths() {
			if path == "D.md" {
				return true
			}
		}
		return false
	})
	if err := os.Remove(filepath.Join(root, "B.md")); err != nil {
		t.Fatal(err)
	}
	watcher.events <- vaultwatch.Event{Path: filepath.Join(root, "B.md"), Kind: vaultwatch.Removed, Deleted: true}
	waitSession(t, session, func(Snapshot) bool {
		_, ok := session.Document("B.md")
		return !ok
	})

	nestedPath := filepath.Join(root, "nested", "child.md")
	if err := os.MkdirAll(filepath.Dir(nestedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nestedPath, []byte("# Child\n\nchild-only-token"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := session.Refresh(nestedPath); err != nil {
		t.Fatal(err)
	}
	waitSession(t, session, func(Snapshot) bool {
		_, ok := session.Document("nested/child.md")
		return ok
	})
	if err := os.RemoveAll(filepath.Dir(nestedPath)); err != nil {
		t.Fatal(err)
	}
	watcher.events <- vaultwatch.Event{Path: filepath.Join(root, "nested"), Kind: vaultwatch.Removed, Deleted: true}
	waitFilesRefreshed(t, session, "nested")
	removed, err := session.Query("child-only-token", false, "", "")
	if err != nil {
		t.Fatal(err)
	}
	removedSnapshot := waitSession(t, session, func(snapshot Snapshot) bool {
		return snapshot.Search.Token == removed && !snapshot.Search.Busy
	})
	if len(removedSnapshot.Search.Hits) != 0 {
		t.Fatalf("deleted subtree remained in search index: %+v", removedSnapshot.Search.Hits)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if watcher.closeCount() != 1 {
		t.Fatalf("watcher closed %d times", watcher.closeCount())
	}
	if _, err := session.Query("after-close", false, "", ""); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("query after close error = %v", err)
	}
	if err := session.CloseLinks(); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("CloseLinks after close error = %v", err)
	}
	drainDeadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-session.Results():
			if !ok {
				return
			}
		case <-drainDeadline:
			t.Fatal("results channel did not close after Close")
		}
	}
}

func TestSessionQueryCapsAndOrdersMatchingNotes(t *testing.T) {
	root := t.TempDir()
	notesDir := filepath.Join(root, "Notes")
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 512; n++ {
		path := filepath.Join(notesDir, fmt.Sprintf("%04d.md", n))
		if err := os.WriteFile(path, []byte("oldqueueprobe latestqueueprobe"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	watcher := newSessionTestWatcher()
	session, err := NewSession(root, sessionTestDependencies(t, watcher))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	waitSession(t, session, func(snapshot Snapshot) bool { return snapshot.Ready })

	token, err := session.Query("latestqueueprobe", true, "", "")
	if err != nil {
		t.Fatal(err)
	}
	result := waitSession(t, session, func(snapshot Snapshot) bool {
		return snapshot.Search.Token == token && !snapshot.Search.Busy
	})
	if result.Search.Error != "" {
		t.Fatalf("query failed: %s", result.Search.Error)
	}
	if got := len(result.Search.Hits); got != searchLimit {
		t.Fatalf("query returned %d hits, want capped result count %d", got, searchLimit)
	}
	for n, hit := range result.Search.Hits {
		want := fmt.Sprintf("Notes/%04d.md", n)
		if hit.Path != want {
			t.Fatalf("hit %d path = %q, want deterministic path order %q", n, hit.Path, want)
		}
	}
}

func waitFilesRefreshed(t *testing.T, session *Session, changedPath string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case result, ok := <-session.Results():
			if !ok {
				t.Fatal("session closed before file refresh result")
			}
			if result.Kind == FilesRefreshed {
				for _, path := range result.Changed {
					if filepath.Base(path) == changedPath || strings.HasSuffix(filepath.ToSlash(path), "/"+changedPath) {
						return
					}
				}
			}
		case <-deadline:
			t.Fatalf("timed out waiting for FilesRefreshed(%q); snapshot=%+v", changedPath, session.Snapshot())
		}
	}
}

func TestSessionQueryTokensAndLatestResultChannel(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("# Searchable\n\nalpha beta gamma"), 0o644); err != nil {
		t.Fatal(err)
	}
	watcher := newSessionTestWatcher()
	session, err := NewSession(root, sessionTestDependencies(t, watcher))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	waitSession(t, session, func(snapshot Snapshot) bool { return snapshot.Ready })
	first, err := session.Query("alpha", false, "", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.Query("gamma", true, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if second <= first {
		t.Fatalf("query token did not advance: first=%d second=%d", first, second)
	}
	result := waitSession(t, session, func(snapshot Snapshot) bool {
		return snapshot.Search.Token == second && !snapshot.Search.Busy
	})
	if result.Search.Query != "gamma" || !result.Search.Literal || len(result.Search.Hits) != 1 {
		t.Fatalf("latest query state = %+v", result.Search)
	}
	if len(session.Results()) > 1 {
		t.Fatalf("result channel exceeded its latest-only capacity: %d", len(session.Results()))
	}
}

func TestSessionCloseWaitsForInFlightPublicOperation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("# Note"), 0o644); err != nil {
		t.Fatal(err)
	}
	watcher := newSessionTestWatcher()
	session, err := NewSession(root, sessionTestDependencies(t, watcher))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	waitSession(t, session, func(snapshot Snapshot) bool { return snapshot.Ready })
	blocked := &blockingCatalog{CatalogPort: session.catalog, entered: make(chan struct{}), release: make(chan struct{})}
	session.mu.Lock()
	session.catalog = blocked
	session.mu.Unlock()
	operationDone := make(chan struct{})
	go func() {
		defer close(operationDone)
		_ = session.Paths()
	}()
	select {
	case <-blocked.entered:
	case <-time.After(time.Second):
		t.Fatal("public operation did not enter catalog")
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- session.Close() }()
	select {
	case err := <-closeDone:
		t.Fatalf("Close returned before public operation finished: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(blocked.release)
	select {
	case <-operationDone:
	case <-time.After(time.Second):
		t.Fatal("public operation did not finish")
	}
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not finish after public operation")
	}
}
