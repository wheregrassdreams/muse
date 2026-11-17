package nativeui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	workspaceapp "muse/application/workspace"
	"muse/platform/filesystem/recentstore"
)

func setRecentTestStore(a *App, dir string) {
	a.SetDependencies(Dependencies{
		RecentItems: workspaceapp.NewRecentItems(recentstore.New(filepath.Join(dir, "recent.json"))),
	})
}

func TestEmptyStateMenuListsOnlyLiveRecents(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "project")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(proj, "notes.md")
	if err := os.WriteFile(notes, []byte("# Notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(dir, "deleted", "gone.md")

	a := New()
	setRecentTestStore(a, dir)
	a.addRecent(proj, true)
	a.addRecent(notes, false)
	a.addRecent(gone, false)

	tester := ui.NewTester(a.View, 1200, 800)
	tester.Frame()
	if !tester.HasText("打开文件或文件夹") {
		t.Fatalf("open menu action missing; texts=%q", tester.Texts())
	}
	if err := tester.Click("打开文件或文件夹"); err != nil {
		t.Fatalf("open menu: %v", err)
	}
	items := strings.Join(tester.Menu(), ",")
	if !strings.Contains(items, "project") || !strings.Contains(items, "notes.md") {
		t.Fatalf("menu missing recent items: %q", items)
	}
	if strings.Contains(items, "gone.md") {
		t.Fatalf("missing path shown in recents: %q", items)
	}
	if err := tester.ChooseMenuItem("project"); err != nil {
		t.Fatalf("choose project: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && a.root != proj {
		time.Sleep(5 * time.Millisecond)
	}
	if a.root != proj {
		t.Fatalf("root = %q, want %q", a.root, proj)
	}
}

func TestEmptyStateMenuOpensPersistedRecentFile(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "project")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(proj, "notes.md")
	if err := os.WriteFile(notes, []byte("# Notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	first := New()
	setRecentTestStore(first, dir)
	first.addRecent(notes, false)

	second := New()
	setRecentTestStore(second, dir)
	entries := second.recentItems.Entries()
	if len(entries) != 1 || entries[0].Path != notes {
		t.Fatalf("recents did not load: %+v", entries)
	}
	tester := ui.NewTester(second.View, 1200, 800)
	tester.Frame()
	if err := tester.Click("打开文件或文件夹"); err != nil {
		t.Fatalf("open menu: %v", err)
	}
	tester.Frame()
	if err := tester.ChooseMenuItem("notes.md"); err != nil {
		t.Fatalf("choose recent file: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && second.documentPath() != notes {
		time.Sleep(5 * time.Millisecond)
	}
	if second.documentPath() != notes {
		t.Fatalf("path = %q, want %q", second.documentPath(), notes)
	}
}
