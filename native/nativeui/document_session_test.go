package nativeui

import (
	"context"
	"github.com/egoist/mygo/ui"
	"image/png"
	documentsave "muse/application/document"
	applicationvault "muse/application/vault"
	"muse/document"
	"muse/document/frontmatter"
	"muse/nativeui/workbench/features/dialogs/document"
	searchui "muse/nativeui/workbench/features/search"
	"muse/platform/filesystem/documentstore"
	"muse/platform/filesystem/filemetadata"
	"muse/platform/filesystem/imagefiles"
	vaultfilessource "muse/platform/filesystem/vaultfilessource"
	vaultindexsource "muse/platform/filesystem/vaultindexsource"
	vaultsearchstore "muse/platform/filesystem/vaultsearchstore"
	filesystemwatcher "muse/platform/filesystem/watcher"
	"muse/search"
	vaultfiles "muse/vault/files"
	vaultindex "muse/vault/index"
	vaultwatch "muse/vault/watch"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func waitDocument(t *testing.T, tt *ui.Tester, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		tt.Frame()
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("document service did not reach expected state")
}

// setDocumentPathForTest opens the current test buffer under a different file
// identity without reaching into App's former document mirror fields.
func setDocumentPathForTest(a *App, path string) {
	a.loadDocumentAt(path, a.documentSource())
}

func TestManagedUpdatedAtIsDebouncedAndKeepsCoreKeys(t *testing.T) {
	a, tt, _ := runtimeFixture(t)
	if err := a.noteEditor().ReplaceUser("a changed note"); err != nil {
		t.Fatal(err)
	}
	if a.noteEditor().PersistenceState().MetadataDue.IsZero() {
		t.Fatal("content edit did not schedule updated_at")
	}
	time.Sleep(documentsave.MetadataUpdateDelay + 10*time.Millisecond)
	tt.Frame()
	doc := frontmatter.Parse(a.documentSource())
	fields := map[string]frontmatter.Field{}
	for _, f := range doc.Fields {
		fields[f.Key] = f
	}
	for _, key := range []string{"type", "tags", "aliases", "created_at", "updated_at"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("managed metadata update omitted %q: %q", key, a.documentSource())
		}
	}
	if fields["updated_at"].Value == "" {
		t.Fatalf("updated_at was not set after debounce: %q", a.documentSource())
	}
	if !a.noteEditor().Undo() || a.noteEditor().Source() != "original" {
		t.Fatalf("Undo should restore the content and fold away managed metadata, got %q", a.noteEditor().Source())
	}
}

func TestTryMergeExternalChangesSavesNonOverlappingEdits(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	path := filepath.Join(t.TempDir(), "merge.md")
	base := "line one\nline two\nline three\n"
	if err := os.WriteFile(path, []byte(base), 0o640); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.navIconStore, _ = applicationvault.NewIconPreferences(&memoryNavIconRepository{values: make(map[string]map[string]string)})
	withDocumentPorts(t, a)
	setDocumentPathForTest(a, path)
	a.enableDocumentServices()
	a.loadDoc(base)
	if err := a.noteEditor().ReplaceUser("local line one\nline two\nline three\n"); err != nil {
		t.Fatal(err)
	}
	external := "line one\nline two\nexternal line three\n"
	if err := os.WriteFile(path, []byte(external), 0o640); err != nil {
		t.Fatal(err)
	}
	a.documents.Apply(documentdialogs.Action{Kind: documentdialogs.OpenConflict})
	outcome, err := a.documentWorkflows().TryMergeExternalChanges()
	if err != nil || outcome != documentsave.MergeSaved {
		t.Fatal("non-overlapping edits were not merged")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "local line one") || !strings.Contains(string(data), "external line three") || a.documentDirty() {
		t.Fatalf("merged file is incomplete or still dirty: dirty=%v source=%q", a.documentDirty(), data)
	}
	if state := a.noteEditor().PersistenceState(); state.Revision != document.RevisionOf(string(data)) || state.BaseSource != string(data) {
		t.Fatalf("successful merge did not adopt the saved revision as its new baseline: %#v", state)
	}
}

func TestTryMergeExternalChangesPreservesDirtyBufferWhenDiskChangesAgain(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	path := filepath.Join(t.TempDir(), "merge-race.md")
	base := "line one\nline two\nline three\n"
	if err := os.WriteFile(path, []byte(base), 0o640); err != nil {
		t.Fatal(err)
	}
	store := documentstore.New(filepath.Join(t.TempDir(), "recovery"))
	repository := &changeBeforeSaveRepository{
		Repository: store,
		path:       path,
		source:     "line one\nline two\nconcurrent line three\n",
	}
	a := New()
	a.navIconStore, _ = applicationvault.NewIconPreferences(&memoryNavIconRepository{values: make(map[string]map[string]string)})
	a.SetDocumentRepositories(repository, store, store)
	setDocumentPathForTest(a, path)
	a.loadDoc(base)
	if err := a.noteEditor().ReplaceUser("local line one\nline two\nline three\n"); err != nil {
		t.Fatal(err)
	}
	external := "line one\nline two\nexternal line three\n"
	if err := os.WriteFile(path, []byte(external), 0o640); err != nil {
		t.Fatal(err)
	}
	a.documents.Apply(documentdialogs.Action{Kind: documentdialogs.OpenConflict})
	if outcome, err := a.documentWorkflows().TryMergeExternalChanges(); err == nil || outcome == documentsave.MergeSaved {
		t.Fatal("merge unexpectedly overwrote a concurrent disk change")
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	buffer := a.documentSource()
	if string(disk) != repository.source || !strings.Contains(buffer, "local line one") || !strings.Contains(buffer, "external line three") || !a.documentDirty() || !a.documents.IsConflictOpen() {
		t.Fatalf("concurrent conflict lost data: disk=%q buffer=%q dirty=%v conflict=%v", disk, buffer, a.documentDirty(), a.documents.IsConflictOpen())
	}
	if state := a.noteEditor().PersistenceState(); state.Revision != document.RevisionOf(external) || state.BaseSource != external {
		t.Fatalf("merge race did not retain the last observed disk baseline: %#v", state)
	}
}

type changeBeforeSaveRepository struct {
	document.Repository
	path   string
	source string
}

func (r *changeBeforeSaveRepository) Save(path, source string, expected document.Revision) (document.Revision, error) {
	if path == r.path {
		if err := os.WriteFile(path, []byte(r.source), 0o640); err != nil {
			return "", err
		}
	}
	return r.Repository.Save(path, source, expected)
}

type memoryNavIconRepository struct{ values map[string]map[string]string }

var _ applicationvault.NavigationIconRepository = (*memoryNavIconRepository)(nil)

func (r *memoryNavIconRepository) LoadVault(root string) (map[string]string, error) {
	items := make(map[string]string)
	for key, value := range r.values[root] {
		items[key] = value
	}
	return items, nil
}

func (r *memoryNavIconRepository) SaveVault(root string, icons map[string]string) error {
	items := make(map[string]string)
	for key, value := range icons {
		items[key] = value
	}
	r.values[root] = items
	return nil
}
func runtimeFixture(t *testing.T) (*App, *ui.Tester, string) {
	return runtimeFixtureWithNotes(t, nil)
}

func runtimeFixtureWithNotes(t *testing.T, notes map[string]string) (*App, *ui.Tester, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	if err := os.WriteFile(path, []byte("original"), 0640); err != nil {
		t.Fatal(err)
	}
	for rel, source := range notes {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return runtimeFixtureAt(t, root, path)
}

func runtimeFixtureAt(t *testing.T, root, path string) (*App, *ui.Tester, string) {
	t.Helper()
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	t.Setenv("MUSE_CACHE_DIR", t.TempDir())
	a := New()
	configureVaultPorts(t, a)
	withDocumentPorts(t, a)
	a.enableDocumentServices()
	a.setWorkspace(root)
	a.openPathNow(path)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	t.Cleanup(func() {
		v := a.vault
		if v != nil {
			if err := v.Close(); err != nil {
				t.Errorf("close vault session: %v", err)
			}
		}
	})
	waitDocument(t, tt, func() bool {
		return a.vault != nil && a.vaultSnapshot.Ready && (a.vaultSnapshot.SearchAvailable || a.vaultSnapshot.Search.Error != "")
	})
	return a, tt, path
}

type nativeUITestSearchReconciler struct {
	store applicationvault.SearchIndexStore
}

func (r nativeUITestSearchReconciler) OpenAndReconcile(ctx context.Context, root string, corpus search.Corpus) (*search.Index, error) {
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

func (r nativeUITestSearchReconciler) Reconcile(ctx context.Context, root string, current *search.Index, corpus search.Corpus) (*search.Index, error) {
	if current == nil {
		return r.OpenAndReconcile(ctx, root, corpus)
	}
	return current, current.ReconcileCorpus(ctx, corpus)
}

func configureVaultPorts(t *testing.T, a *App) {
	t.Helper()
	store, err := vaultsearchstore.New(filepath.Join(t.TempDir(), "search"))
	if err != nil {
		t.Fatal(err)
	}
	a.SetDependencies(Dependencies{
		DocumentMetadata:      filemetadata.New(),
		ImageLocalFiles:       imagefiles.New(),
		NavIcons:              &memoryNavIconRepository{values: make(map[string]map[string]string)},
		VaultSearchReconciler: nativeUITestSearchReconciler{store: store},
		NewVaultIndex: func(root string) (applicationvault.IndexPort, error) {
			source, err := vaultindexsource.New(root)
			if err != nil {
				return nil, err
			}
			return vaultindex.New(source), nil
		},
		NewVaultCatalog: func(root string) (applicationvault.CatalogPort, error) {
			source, err := vaultfilessource.New(root)
			if err != nil {
				return nil, err
			}
			return vaultfiles.New(source), nil
		},
		NewWatcher: func(root string) (applicationvault.WatcherPort, error) {
			backend, err := filesystemwatcher.New()
			if err != nil {
				return nil, err
			}
			return vaultwatch.New(root, backend, filesystemwatcher.NewDirectoryAccess())
		},
	})
}

func runtimeFixtureCopy(t *testing.T, sourceRoot string) (*App, *ui.Tester, string) {
	t.Helper()
	root := t.TempDir()
	err := filepath.WalkDir(sourceRoot, func(source string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(sourceRoot, source)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dest := filepath.Join(root, rel)
		if entry.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return runtimeFixtureAt(t, root, filepath.Join(root, "Home.md"))
}
func TestAutosaveAndRecoveryHistory(t *testing.T) {
	a, tt, path := runtimeFixture(t)
	if err := a.noteEditor().ReplaceUser("edited 中文"); err != nil {
		t.Fatal(err)
	}
	expireAutosave(a)
	waitDocument(t, tt, func() bool { return !a.documentDirty() })
	data, _ := os.ReadFile(path)
	if string(data) != "edited 中文" {
		t.Fatalf("saved %q", data)
	}
	history, err := a.noteEditor().ListHistory(path)
	if err != nil || len(history) == 0 {
		t.Fatalf("history %v %v", history, err)
	}
	d, err := a.noteEditor().ReadHistory(path, history[0].ID)
	if err != nil || d.Source != "original" {
		t.Fatalf("history content %v %v", d, err)
	}
}
func TestAutosaveDoesNotOverwriteExternalChange(t *testing.T) {
	a, tt, path := runtimeFixture(t)
	if err := a.noteEditor().ReplaceUser("my unsaved edits"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("external content"), 0640); err != nil {
		t.Fatal(err)
	}
	expireAutosave(a)
	waitDocument(t, tt, func() bool { return a.noteEditor().PersistenceState().ConflictPath != "" })
	runtimeShot(t, tt, "document-conflict")
	data, _ := os.ReadFile(path)
	if string(data) != "external content" {
		t.Fatalf("overwritten %q", data)
	}
	if a.documentSource() != "my unsaved edits" || !a.documentDirty() {
		t.Fatal("lost current edits")
	}
	draft, err := a.noteEditor().ReadDraft(path)
	if err != nil || draft.Text != "my unsaved edits" {
		t.Fatalf("recovery %v %v", draft, err)
	}
	a.documents.Apply(documentdialogs.Action{Kind: documentdialogs.CloseConflict})
	a.save()
	data, _ = os.ReadFile(path)
	if string(data) != "external content" {
		t.Fatal("manual save bypassed revision check")
	}
}
func TestCleanExternalEditReloadsAndIndexFindsIt(t *testing.T) {
	a, tt, path := runtimeFixture(t)
	if err := os.WriteFile(path, []byte("external UNIQUE123"), 0640); err != nil {
		t.Fatal(err)
	}
	waitDocument(t, tt, func() bool { return a.documentSource() == "external UNIQUE123" })
	if a.documentDirty() {
		t.Fatal("external reload marked dirty")
	}
	if _, err := a.vault.Query("UNIQUE123", false, a.documentPath(), a.diskSource()); err != nil {
		t.Fatal(err)
	}
	waitDocument(t, tt, func() bool { return !a.vaultSnapshot.Search.Busy && len(a.vaultSnapshot.Search.Hits) == 1 })
	if len(a.vaultSnapshot.Search.Hits) != 1 {
		t.Fatal("index did not refresh")
	}
	if a.noteEditor().PersistenceState().Revision != document.RevisionOf(a.diskSource()) {
		t.Fatal("baseline stale")
	}
}
func TestUntitledDraftSurvivesRecreatingApp(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	withDocumentPorts(t, a)
	a.enableDocumentServices()
	if err := a.noteEditor().ReplaceUser("unsaved untitled"); err != nil {
		t.Fatal(err)
	}
	expireAutosave(a)
	tt := ui.NewTester(a.View, 1200, 800)
	waitDocument(t, tt, func() bool {
		state := a.noteEditor().PersistenceState()
		return !state.Saving && state.AutosaveDue.IsZero()
	})
	b := New()
	withDocumentPorts(t, b)
	b.enableDocumentServices()
	b.openRecovery()
	drafts, err := b.documentRecovery.ListDrafts()
	if err != nil || len(drafts) != 1 || drafts[0].Text != "unsaved untitled" {
		t.Fatal("draft not recovered")
	}
}

func withDocumentPorts(t *testing.T, a *App) {
	t.Helper()
	store := documentstore.New(filepath.Join(t.TempDir(), "recovery"))
	a.SetDocumentRepositories(store, store, store)
}

func expireAutosave(a *App) {
	time.Sleep(documentsave.AutosaveDelay + 10*time.Millisecond)
}

func TestIndexedWorkspaceSearchAndAliasNavigation(t *testing.T) {
	a, tt, path := runtimeFixture(t)
	if err := a.noteEditor().ReplaceUser("unsaved-buffer-only"); err != nil {
		t.Fatal(err)
	}
	if doc, ok := a.vault.Document("note.md"); !ok || doc.Source != "original" {
		t.Fatalf("editor buffer leaked into persistent metadata snapshot: %#v", doc)
	}
	a.loadDoc("original")
	target := filepath.Join(filepath.Dir(path), "target.md")
	if err := os.WriteFile(target, []byte("---\naliases: [Second]\n---\n# Section\n\nneedle-unique-abc\n"), 0644); err != nil {
		t.Fatal(err)
	}
	waitDocument(t, tt, func() bool { _, ok := a.vault.ResolveLink("note.md", "Second#Section"); return ok })
	a.openWorkspaceSearch()
	a.workspaceSearch.Apply(searchui.Action{Kind: searchui.QueryChanged, Query: "needle-unique-abc"})
	waitDocument(t, tt, func() bool { return len(a.vaultSnapshot.Search.Hits) == 1 })
	if err := tt.Click("字面量"); err != nil {
		t.Fatalf("switch to literal mode: %v", err)
	}
	waitDocument(t, tt, func() bool { return a.vaultSnapshot.Search.Literal && len(a.vaultSnapshot.Search.Hits) == 1 })
	if err := tt.Click("全文"); err != nil {
		t.Fatalf("switch to full-text mode: %v", err)
	}
	waitDocument(t, tt, func() bool { return !a.vaultSnapshot.Search.Literal && len(a.vaultSnapshot.Search.Hits) == 1 })
	runtimeShot(t, tt, "vault-search-modal")
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	waitDocument(t, tt, func() bool { return a.documentPath() == target })
	if a.documentPath() != target {
		t.Fatalf("search opened %q", a.documentPath())
	}
	a.openPathNow(path)
	if !a.indexedLink("Second#Section") || a.documentPath() != target {
		t.Fatal("alias/heading link did not navigate")
	}
}

func TestWorkspaceSearchCoversNestedNotesAndTracksUpdatesAndDeletion(t *testing.T) {
	a, tt, rootNote := runtimeFixtureCopy(t, filepath.Join("..", "testdata", "vault-fixture"))
	root := filepath.Dir(rootNote)
	nested := filepath.Join(root, "Projects", "Atlas", "Overview.md")
	waitDocument(t, tt, func() bool {
		_, ok := a.vault.Document("Projects/Atlas/Overview.md")
		return ok
	})
	a.openWorkspaceSearch()
	a.workspaceSearch.Apply(searchui.Action{Kind: searchui.QueryChanged, Query: "vaultfixturedeepsearchunique"})
	waitDocument(t, tt, func() bool { return !a.vaultSnapshot.Search.Busy && len(a.vaultSnapshot.Search.Hits) == 1 })
	if got := a.vaultSnapshot.Search.Hits[0].Path; got != "Projects/Atlas/Overview.md" {
		t.Fatalf("deep note search result path = %q", got)
	}

	original, err := os.ReadFile(nested)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(original), "vaultfixturedeepsearchunique", "vaultfixturedeepsearchupdated", 1)
	if err := os.WriteFile(nested, []byte(updated), 0644); err != nil {
		t.Fatal(err)
	}
	waitDocument(t, tt, func() bool {
		doc, ok := a.vault.Document("Projects/Atlas/Overview.md")
		return ok && strings.Contains(doc.Source, "vaultfixturedeepsearchupdated")
	})
	a.workspaceSearch.Apply(searchui.Action{Kind: searchui.QueryChanged, Query: "vaultfixturedeepsearchupdated"})
	waitDocument(t, tt, func() bool { return !a.vaultSnapshot.Search.Busy && len(a.vaultSnapshot.Search.Hits) == 1 })
	a.workspaceSearch.Apply(searchui.Action{Kind: searchui.QueryChanged, Query: "vaultfixturedeepsearchunique"})
	waitDocument(t, tt, func() bool { return !a.vaultSnapshot.Search.Busy && len(a.vaultSnapshot.Search.Hits) == 0 })

	if err := os.Remove(nested); err != nil {
		t.Fatal(err)
	}
	waitDocument(t, tt, func() bool {
		_, ok := a.vault.Document("Projects/Atlas/Overview.md")
		return !ok
	})
	a.workspaceSearch.Apply(searchui.Action{Kind: searchui.QueryChanged, Query: "vaultfixturedeepsearchupdated"})
	waitDocument(t, tt, func() bool { return !a.vaultSnapshot.Search.Busy && len(a.vaultSnapshot.Search.Hits) == 0 })
}

func TestHistoryRestoreDoesNotOverwriteOriginal(t *testing.T) {
	a, tt, path := runtimeFixture(t)
	if err := a.noteEditor().ReplaceUser("updated"); err != nil {
		t.Fatal(err)
	}
	a.save()
	a.openHistory()
	tt.Frame()
	runtimeShot(t, tt, "document-history")
	history := a.documents.HistoryEntries()
	if len(history) == 0 {
		t.Fatal("history list is empty")
	}
	h := history[0]
	label := h.CreatedAt.Local().Format("2006-01-02 15:04:05") + " · " + string(h.Revision)[:8]
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.documentPath() != "" || a.documentSource() != "original" || !a.documentDirty() {
		t.Fatalf("restore path=%q source=%q dirty=%v", a.documentPath(), a.documentSource(), a.documentDirty())
	}
	data, _ := os.ReadFile(path)
	if string(data) != "updated" {
		t.Fatal("history restore overwrote original file")
	}
}
func TestPausedAutosaveKeepsBaseline(t *testing.T) {
	a, tt, path := runtimeFixture(t)
	revision := a.noteEditor().PersistenceState().Revision
	a.noteEditor().SetConflict(path)
	if err := a.noteEditor().ReplaceUser("local edits"); err != nil {
		t.Fatal(err)
	}
	expireAutosave(a)
	waitDocument(t, tt, func() bool {
		state := a.noteEditor().PersistenceState()
		return !state.Saving && state.AutosaveDue.IsZero()
	})
	if a.noteEditor().PersistenceState().Revision != revision || !a.documentDirty() {
		t.Fatal("paused save changed baseline/dirty state")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "original" {
		t.Fatal("paused save wrote disk")
	}
}

func runtimeShot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("MUSE_UI_SHOTS")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestBacklinkUIAndExternalDeletion(t *testing.T) {
	a, tt, path := runtimeFixture(t)
	target := filepath.Join(filepath.Dir(path), "target.md")
	if err := os.WriteFile(target, []byte("# Target\n\nbody"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[[Target]]"), 0640); err != nil {
		t.Fatal(err)
	}
	waitDocument(t, tt, func() bool { return len(a.vault.Backlinks("target.md")) == 1 })
	a.openPathNow(target)
	a.openLinks()
	tt.Frame()
	waitDocument(t, tt, func() bool { return !a.vaultSnapshot.LinksBusy })
	if len(a.vaultSnapshot.Incoming) != 1 || a.vaultSnapshot.Incoming[0].Source != "note.md" {
		t.Fatal("wrong backlink results")
	}
	runtimeShot(t, tt, "backlinks")
	if err := tt.Click("反向链接：note.md"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.documentPath() != path {
		t.Fatal("backlink did not navigate")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) && a.noteEditor().PersistenceState().ConflictPath != path {
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
	if a.noteEditor().PersistenceState().ConflictPath != path {
		t.Fatalf("external deletion was not surfaced: conflict=%q current=%q vault=%+v", a.noteEditor().PersistenceState().ConflictPath, a.documentPath(), a.vault.Snapshot())
	}
	a.documents.Apply(documentdialogs.Action{Kind: documentdialogs.CloseConflict})
	if err := a.noteEditor().ReplaceUser("keep after deletion"); err != nil {
		t.Fatal(err)
	}
	expireAutosave(a)
	waitDocument(t, tt, func() bool {
		state := a.noteEditor().PersistenceState()
		return !state.Saving && state.AutosaveDue.IsZero()
	})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("autosave recreated externally deleted file")
	}
	d, err := a.noteEditor().ReadDraft(path)
	if err != nil || d.Text != "keep after deletion" {
		t.Fatal("deleted document draft missing")
	}
}

func TestEditorChangesUpdateSharedBacklinkIndex(t *testing.T) {
	a, tt, _ := runtimeFixtureWithNotes(t, map[string]string{"Target.md": "# Target\n"})
	if err := a.noteEditor().ReplaceUser("[[Target]]"); err != nil {
		t.Fatal(err)
	}
	waitDocument(t, tt, func() bool {
		return len(a.vault.Backlinks("Target.md")) == 1
	})

	if err := a.noteEditor().ReplaceUser("No internal link"); err != nil {
		t.Fatal(err)
	}
	waitDocument(t, tt, func() bool {
		return len(a.vault.Backlinks("Target.md")) == 0
	})
	if got := a.vault.Outgoing("note.md"); len(got) != 0 {
		t.Fatalf("editor source snapshot left stale outgoing links: %+v", got)
	}
}

func TestIndexedCRLFHeadingOffsets(t *testing.T) {
	a, tt, path := runtimeFixture(t)
	target := filepath.Join(filepath.Dir(path), "crlf.md")
	raw := "# Top\r\n\r\nfirst\r\n\r\n## Second\r\n\r\nbody\r\n"
	if err := os.WriteFile(target, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	waitDocument(t, tt, func() bool { _, ok := a.vault.ResolveLink("note.md", "crlf#Second"); return ok })
	if !a.indexedLink("crlf#Second") {
		t.Fatal("link did not resolve")
	}
	tt.Frame()
	want := strings.Index(a.documentSource(), "## Second")
	if a.noteEditor().NavigationCommands().Caret() != want {
		t.Fatalf("caret=%d want=%d", a.noteEditor().NavigationCommands().Caret(), want)
	}
	// The same target in the active CRLF buffer must use the same conversion.
	a.noteEditor().NavigationCommands().Select(0, 0)
	a.indexedLink("#Second")
	tt.Frame()
	if a.noteEditor().NavigationCommands().Caret() != want {
		t.Fatal("same-file CRLF offset drift")
	}
}
