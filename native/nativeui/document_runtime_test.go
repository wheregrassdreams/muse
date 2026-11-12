package nativeui

import (
	"github.com/egoist/mygo/ui"
	"image/png"
	"muse/notestore"
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
	a.enableDocumentServices()
	a.setWorkspace(root)
	a.openPathNow(path)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	t.Cleanup(func() {
		v := a.vault
		stopVault(v)
		if v != nil && v.closeDone != nil {
			select {
			case <-v.closeDone:
			case <-time.After(10 * time.Second):
				t.Error("search index did not close before test cache cleanup")
			}
		}
	})
	waitDocument(t, tt, func() bool {
		return a.vault != nil && a.vault.watch != nil && a.vault.index != nil && (a.vault.search != nil || a.vault.searchErr != nil)
	})
	return a, tt, path
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
	a.ed.SetSource("edited 中文")
	a.docChanged()
	a.documents.due = time.Now().Add(-time.Second)
	waitDocument(t, tt, func() bool { return !a.dirty })
	data, _ := os.ReadFile(path)
	if string(data) != "edited 中文" {
		t.Fatalf("saved %q", data)
	}
	history, err := a.documents.store.ListHistory(path)
	if err != nil || len(history) == 0 {
		t.Fatalf("history %v %v", history, err)
	}
	d, err := a.documents.store.ReadHistory(path, history[0].ID)
	if err != nil || d.Text != "original" {
		t.Fatalf("history content %v %v", d, err)
	}
}
func TestAutosaveDoesNotOverwriteExternalChange(t *testing.T) {
	a, tt, path := runtimeFixture(t)
	a.ed.SetSource("my unsaved edits")
	a.docChanged()
	if err := os.WriteFile(path, []byte("external content"), 0640); err != nil {
		t.Fatal(err)
	}
	a.documents.due = time.Now().Add(-time.Second)
	waitDocument(t, tt, func() bool { return a.documents.conflict != "" })
	runtimeShot(t, tt, "document-conflict")
	data, _ := os.ReadFile(path)
	if string(data) != "external content" {
		t.Fatalf("overwritten %q", data)
	}
	if a.source != "my unsaved edits" || !a.dirty {
		t.Fatal("lost current edits")
	}
	draft, err := a.documents.store.ReadDraft(path)
	if err != nil || draft.Text != "my unsaved edits" {
		t.Fatalf("recovery %v %v", draft, err)
	}
	a.documents.conflictOpen = false
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
	waitDocument(t, tt, func() bool { return a.source == "external UNIQUE123" })
	if a.dirty {
		t.Fatal("external reload marked dirty")
	}
	if len(a.vault.index.Query("UNIQUE123")) != 1 {
		t.Fatal("index did not refresh")
	}
	if a.documents.revision != notestore.Revision(a.diskSource()) {
		t.Fatal("baseline stale")
	}
}
func TestUntitledDraftSurvivesRecreatingApp(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.enableDocumentServices()
	a.ed.SetSource("unsaved untitled")
	a.docChanged()
	a.documents.due = time.Now().Add(-time.Second)
	tt := ui.NewTester(a.View, 1200, 800)
	waitDocument(t, tt, func() bool { return !a.documents.saving && a.documents.due.IsZero() })
	b := New()
	b.enableDocumentServices()
	b.openRecovery()
	if len(b.documents.drafts) != 1 || b.documents.drafts[0].Text != "unsaved untitled" {
		t.Fatal("draft not recovered")
	}
}

func TestIndexedWorkspaceSearchAndAliasNavigation(t *testing.T) {
	a, tt, path := runtimeFixture(t)
	a.ed.SetSource("unsaved-buffer-only")
	a.dirty = true
	if doc, ok := a.vault.index.Document("note.md"); !ok || doc.Source != "original" {
		t.Fatalf("editor buffer leaked into persistent metadata snapshot: %#v", doc)
	}
	a.ed.SetSource("original")
	a.dirty = false
	target := filepath.Join(filepath.Dir(path), "target.md")
	if err := os.WriteFile(target, []byte("---\naliases: [Second]\n---\n# Section\n\nneedle-unique-abc\n"), 0644); err != nil {
		t.Fatal(err)
	}
	waitDocument(t, tt, func() bool { _, ok := a.vault.index.Resolve("note.md", "Second#Section"); return ok })
	a.openWorkspaceSearch()
	a.workspaceQuery = "needle-unique-abc"
	waitDocument(t, tt, func() bool { return len(a.vault.hits) == 1 })
	if err := tt.Click("字面量"); err != nil {
		t.Fatalf("switch to literal mode: %v", err)
	}
	waitDocument(t, tt, func() bool { return a.vault.searchLiteral && len(a.vault.hits) == 1 })
	if err := tt.Click("全文"); err != nil {
		t.Fatalf("switch to full-text mode: %v", err)
	}
	waitDocument(t, tt, func() bool { return !a.vault.searchLiteral && len(a.vault.hits) == 1 })
	runtimeShot(t, tt, "vault-search-modal")
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	waitDocument(t, tt, func() bool { return a.path == target })
	if a.path != target {
		t.Fatalf("search opened %q", a.path)
	}
	a.openPathNow(path)
	if !a.indexedLink("Second#Section") || a.path != target {
		t.Fatal("alias/heading link did not navigate")
	}
}

func TestWorkspaceSearchCoversNestedNotesAndTracksUpdatesAndDeletion(t *testing.T) {
	a, tt, rootNote := runtimeFixtureCopy(t, filepath.Join("..", "testdata", "vault-fixture"))
	root := filepath.Dir(rootNote)
	nested := filepath.Join(root, "Projects", "Atlas", "Overview.md")
	waitDocument(t, tt, func() bool {
		_, ok := a.vault.index.Document("Projects/Atlas/Overview.md")
		return ok
	})
	a.openWorkspaceSearch()
	a.workspaceQuery = "vaultfixturedeepsearchunique"
	waitDocument(t, tt, func() bool { return !a.vault.searchBusy && len(a.vault.hits) == 1 })
	if got := a.vault.hits[0].Path; got != "Projects/Atlas/Overview.md" {
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
		doc, ok := a.vault.index.Document("Projects/Atlas/Overview.md")
		return ok && strings.Contains(doc.Source, "vaultfixturedeepsearchupdated")
	})
	a.workspaceQuery = "vaultfixturedeepsearchupdated"
	waitDocument(t, tt, func() bool { return !a.vault.searchBusy && len(a.vault.hits) == 1 })
	a.workspaceQuery = "vaultfixturedeepsearchunique"
	waitDocument(t, tt, func() bool { return !a.vault.searchBusy && len(a.vault.hits) == 0 })

	if err := os.Remove(nested); err != nil {
		t.Fatal(err)
	}
	waitDocument(t, tt, func() bool {
		_, ok := a.vault.index.Document("Projects/Atlas/Overview.md")
		return !ok
	})
	a.workspaceQuery = "vaultfixturedeepsearchupdated"
	waitDocument(t, tt, func() bool { return !a.vault.searchBusy && len(a.vault.hits) == 0 })
}

func TestHistoryRestoreDoesNotOverwriteOriginal(t *testing.T) {
	a, tt, path := runtimeFixture(t)
	a.ed.SetSource("updated")
	a.docChanged()
	a.save()
	a.openHistory()
	tt.Frame()
	runtimeShot(t, tt, "document-history")
	h := a.documents.history[0]
	label := h.CreatedAt.Local().Format("2006-01-02 15:04:05") + " · " + h.Revision[:8]
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.path != "" || a.source != "original" || !a.dirty {
		t.Fatalf("restore path=%q source=%q dirty=%v", a.path, a.source, a.dirty)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "updated" {
		t.Fatal("history restore overwrote original file")
	}
}
func TestPausedAutosaveKeepsBaseline(t *testing.T) {
	a, tt, path := runtimeFixture(t)
	revision := a.documents.revision
	a.documents.conflict = path
	a.ed.SetSource("local edits")
	a.docChanged()
	a.documents.due = time.Now().Add(-time.Second)
	waitDocument(t, tt, func() bool { return !a.documents.saving && a.documents.due.IsZero() })
	if a.documents.revision != revision || !a.dirty {
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
	waitDocument(t, tt, func() bool { return len(a.vault.index.Backlinks("target.md")) == 1 })
	a.openPathNow(target)
	a.openLinks()
	tt.Frame()
	waitDocument(t, tt, func() bool { return !a.vault.linksBusy })
	if len(a.vault.incoming) != 1 || a.vault.incoming[0].Source != "note.md" {
		t.Fatal("wrong backlink results")
	}
	runtimeShot(t, tt, "backlinks")
	if err := tt.Click("反向链接：note.md"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.path != path {
		t.Fatal("backlink did not navigate")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) && a.documents.conflict != path {
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
	if a.documents.conflict != path {
		t.Fatalf("external deletion was not surfaced: conflict=%q current=%q refreshing=%v watch=%v queued=%d", a.documents.conflict, a.path, a.vault.refreshing, a.vault.watch != nil, len(a.vault.watch.Events()))
	}
	a.documents.conflictOpen = false
	a.ed.SetSource("keep after deletion")
	a.docChanged()
	a.documents.due = time.Now().Add(-time.Second)
	waitDocument(t, tt, func() bool { return !a.documents.saving && a.documents.due.IsZero() })
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("autosave recreated externally deleted file")
	}
	d, err := a.documents.store.ReadDraft(path)
	if err != nil || d.Text != "keep after deletion" {
		t.Fatal("deleted document draft missing")
	}
}

func TestIndexedCRLFHeadingOffsets(t *testing.T) {
	a, tt, path := runtimeFixture(t)
	target := filepath.Join(filepath.Dir(path), "crlf.md")
	raw := "# Top\r\n\r\nfirst\r\n\r\n## Second\r\n\r\nbody\r\n"
	if err := os.WriteFile(target, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	waitDocument(t, tt, func() bool { _, ok := a.vault.index.Resolve("note.md", "crlf#Second"); return ok })
	if !a.indexedLink("crlf#Second") {
		t.Fatal("link did not resolve")
	}
	tt.Frame()
	want := strings.Index(a.source, "## Second")
	if a.ed.Caret() != want {
		t.Fatalf("caret=%d want=%d", a.ed.Caret(), want)
	}
	// The same target in the active CRLF buffer must use the same conversion.
	a.ed.Select(0, 0)
	a.indexedLink("#Second")
	tt.Frame()
	if a.ed.Caret() != want {
		t.Fatal("same-file CRLF offset drift")
	}
}
