package quickopen

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	applicationsearch "muse/application/search"
	workspaceSearch "muse/nativeui/workbench/features/search/workspace"
	vaultfilessource "muse/platform/filesystem/vaultfilessource"
	"muse/platform/filesystem/vaultindexsource"
	"muse/platform/filesystem/vaultsearchstore"
	"muse/search"
	vaultfiles "muse/vault/files"
	vaultindex "muse/vault/index"
	vaultwatch "muse/vault/watch"

	applicationvault "muse/application/vault"
)

type testVaultQuickOpen struct {
	snapshot applicationvault.Snapshot
	matches  []applicationvault.QuickOpenMatch
	queries  []string
}

type quickOpenTestWatcher struct{ events chan vaultwatch.Event }

func (w *quickOpenTestWatcher) Events() <-chan vaultwatch.Event { return w.events }
func (*quickOpenTestWatcher) WatchFile(string) error            { return nil }
func (*quickOpenTestWatcher) Close() error                      { return nil }

type quickOpenTestReconciler struct{ store *vaultsearchstore.Store }

func (r quickOpenTestReconciler) OpenAndReconcile(ctx context.Context, root string, corpus search.Corpus) (*search.Index, error) {
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

func (r quickOpenTestReconciler) Reconcile(ctx context.Context, root string, current *search.Index, corpus search.Corpus) (*search.Index, error) {
	if current == nil {
		return r.OpenAndReconcile(ctx, root, corpus)
	}
	return current, current.ReconcileCorpus(ctx, corpus)
}

func TestVaultCandidatesReturnsFixtureResultsAfterSessionStartup(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "testdata", "vault-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "Projects", "Atlas", "Overview.md")
	cache, err := vaultsearchstore.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	watcher := &quickOpenTestWatcher{events: make(chan vaultwatch.Event)}
	started := time.Now()
	session, err := applicationvault.NewSession(root, applicationvault.SessionDependencies{
		NewIndex: func(root string) (applicationvault.IndexPort, error) {
			source, err := vaultindexsource.New(root)
			if err != nil {
				return nil, err
			}
			return vaultindex.New(source), nil
		},
		NewCatalog: func(root string) (applicationvault.CatalogPort, error) {
			source, err := vaultfilessource.New(root)
			if err != nil {
				return nil, err
			}
			return vaultfiles.New(source), nil
		},
		NewWatcher:       func(string) (applicationvault.WatcherPort, error) { return watcher, nil },
		SearchReconciler: quickOpenTestReconciler{store: cache},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("NewSession returned in %s", time.Since(started))
	t.Cleanup(func() { _ = session.Close() })
	source := NewVaultCandidates(session)
	deadline := time.Now().Add(5 * time.Second)
	for !session.Snapshot().Ready && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if snapshot := session.Snapshot(); !snapshot.Ready || !snapshot.SearchAvailable {
		t.Fatalf("session startup snapshot = %+v", snapshot)
	}
	t.Logf("fixture session became Ready in %s", time.Since(started))
	rows := source.Rows("overview")
	if rows.Len() != 1 {
		t.Fatalf("Quick Open rows = %d, want one populated fixture result", rows.Len())
	}
	row := rows.At(0)
	if row.Path != path || row.PathText != "Projects/Atlas/Overview.md" || row.Title != "Atlas Overview" {
		t.Fatalf("Quick Open fixture row = %+v", row)
	}
	searchPort := workspaceSearch.NewVaultSearchPort(session)
	token, err := searchPort.Query(applicationsearch.QueryInput{Text: "vaultfixturehomeunique"})
	if err != nil {
		t.Fatalf("workspace search query: %v", err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for searchPort.SearchState().Busy && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	state := searchPort.SearchState()
	if state.Token != token || len(state.Hits) != 1 || state.Hits[0].Path != "Home.md" {
		t.Fatalf("workspace search state = %+v", state)
	}
}

func (v *testVaultQuickOpen) QuickOpen(query string) []applicationvault.QuickOpenMatch {
	v.queries = append(v.queries, query)
	return v.matches
}

func (v *testVaultQuickOpen) Snapshot() applicationvault.Snapshot { return v.snapshot }

func TestVaultCandidatesAdaptsReadySessionAndRevision(t *testing.T) {
	vault := &testVaultQuickOpen{
		snapshot: applicationvault.Snapshot{Ready: true, FilesRevision: 3, IndexRevision: 7},
		matches: []applicationvault.QuickOpenMatch{{
			Path: "/vault/note.md", PathText: "note.md", Title: "Note",
			TitleMatches: []int{0}, PathMatches: []int{0, 1},
		}},
	}
	source := NewVaultCandidates(vault)
	if got := source.Revision(); got != (Revision{Catalog: 3, Index: 7}) {
		t.Fatalf("revision = %+v", got)
	}
	rows := source.Rows("no")
	if rows.Len() != 1 {
		t.Fatalf("row count = %d", rows.Len())
	}
	row := rows.At(0)
	if row.Path != "/vault/note.md" || row.Title != "Note" || row.PathText != "note.md" {
		t.Fatalf("row projection = %+v", row)
	}
	row.TitleMatches[0] = 99
	if vault.matches[0].TitleMatches[0] != 0 {
		t.Fatal("candidate source exposed mutable application match slices")
	}
	if len(vault.queries) != 1 || vault.queries[0] != "no" {
		t.Fatalf("queries = %v", vault.queries)
	}
}

func TestVaultCandidatesSwitchesSessionsAndIgnoresUnreadyVault(t *testing.T) {
	first := &testVaultQuickOpen{
		snapshot: applicationvault.Snapshot{Ready: false, FilesRevision: 4, IndexRevision: 9},
		matches:  []applicationvault.QuickOpenMatch{{Path: "/old.md"}},
	}
	source := NewVaultCandidates(first)
	if got := source.Revision(); got != (Revision{Catalog: 4, Index: 9}) {
		t.Fatalf("unready revision = %+v", got)
	}
	if got := source.Rows("old"); got.Len() != 0 {
		t.Fatalf("unready vault returned %d rows", got.Len())
	}

	second := &testVaultQuickOpen{
		snapshot: applicationvault.Snapshot{Ready: true, FilesRevision: 11, IndexRevision: 12},
		matches:  []applicationvault.QuickOpenMatch{{Path: "/new.md", Title: "New"}},
	}
	source.SetVault(second)
	if got := source.Revision(); got != (Revision{Catalog: 11, Index: 12}) {
		t.Fatalf("switched revision = %+v", got)
	}
	if rows := source.Rows(""); rows.Len() != 1 || rows.At(0).Path != "/new.md" {
		t.Fatalf("switched rows = %+v", rows)
	}
	source.SetVault(nil)
	if got := source.Revision(); got != (Revision{}) || source.Rows("").Len() != 0 {
		t.Fatalf("cleared source retained values: revision=%+v", got)
	}
}
