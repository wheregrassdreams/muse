// Package nativeui contains shell integration coverage for Explorer's typed
// intents after they are composed with the vault and editor workflows. Pure
// selection, projection, drag rendering, and key-binding assertions live in
// workbench/features/explorer/filetree; this suite keeps only checks that
// cross the shell, filesystem, or editor boundary.
package nativeui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	applicationvault "muse/application/vault"
	vaultfilesapp "muse/application/vault/files"
	"muse/document/frontmatter"
	"muse/platform/filesystem/documentstore"
	"muse/platform/filesystem/imagefiles"
	"muse/platform/filesystem/vaultfiles"
	vaultfilessource "muse/platform/filesystem/vaultfilessource"
	vaultindexsource "muse/platform/filesystem/vaultindexsource"
	vaultcatalog "muse/vault/files"
	vaultindex "muse/vault/index"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func configureFileTreeTestApp(t testing.TB, a *App, root string) {
	t.Helper()
	a.root = root
	fileOps := vaultfilesapp.NewFileOperations(vaultfiles.New(func(path string) error {
		return trashItem(path)
	}))
	store := documentstore.New(filepath.Join(t.TempDir(), "recovery"))
	a.SetDependencies(Dependencies{
		Documents: store, Recovery: store, History: store,
		FileOperations: fileOps, ImageLocalFiles: imagefiles.New(),
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
			return vaultcatalog.New(source), nil
		},
	})
	session, err := applicationvault.NewSession(root, applicationvault.SessionDependencies{
		NewIndex: a.newVaultIndex, NewCatalog: a.newVaultCatalog,
		NewWatcher: func(string) (applicationvault.WatcherPort, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !session.Snapshot().Ready && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !session.Snapshot().Ready {
		_ = session.Close()
		t.Fatal("vault test session did not initialize")
	}
	a.vault = session
	a.vaultSnapshot = session.Snapshot()
	t.Cleanup(func() { _ = session.Close() })
}

func newFileTreeTestApp(t testing.TB, root string) *App {
	t.Helper()
	a := New()
	configureFileTreeTestApp(t, a, root)
	return a
}

func TestNewFileToolbarCreatesAndOpens(t *testing.T) {
	dir := t.TempDir()
	a := New()
	configureFileTreeTestApp(t, a, dir)

	tt := ui.NewTester(a.View, 1200, 800)
	if err := tt.Click("新建文档"); err != nil {
		t.Fatalf("点击新建文件失败: %v", err)
	}

	created := filepath.Join(dir, "未命名.md")
	if _, err := os.Stat(created); err != nil {
		t.Fatalf("文件未创建: %v", err)
	}
	if a.documentPath() != created {
		t.Fatalf("新文件未自动打开: path=%q", a.documentPath())
	}
	doc := frontmatter.Parse(a.documentSource())
	fields := map[string]frontmatter.Field{}
	for _, f := range doc.Fields {
		fields[f.Key] = f
	}
	for _, key := range []string{"type", "tags", "aliases", "created_at", "updated_at"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("新文件缺少核心字段 %q: %q", key, a.documentSource())
		}
	}
	if fields["type"].Value != "note" || fields["created_at"].Value == "" || fields["updated_at"].Value != "" {
		t.Fatalf("新文件核心字段初始值错误: %+v", fields)
	}

}

func clickTreeRow(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("tree item %q missing", label)
	}
	tt.ClickAt(r.X+4, r.Y+r.H/2)
}

func keyTestWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"alpha.md", "beta.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("# "+name+"\nline two\nline three"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "dest"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func rightClickTreeRow(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("tree item %q missing", label)
	}
	tt.RightClickAt(r.X+4, r.Y+r.H/2)
}
