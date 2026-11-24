// Package nativeui contains shell integration coverage for Explorer's typed
// intents after they are composed with the vault and editor workflows. Pure
// selection, projection, drag rendering, and key-binding assertions live in
// workbench/features/explorer/filetree; this suite keeps only checks that
// cross the shell, filesystem, or editor boundary.
package nativeui

import (
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
		return os.RemoveAll(path)
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

func TestExplorerFolderToggleIconsAndDropMove(t *testing.T) {
	root := t.TempDir()
	projects := filepath.Join(root, "Projects")
	dest := filepath.Join(projects, "Archive")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "note.md")
	if err := os.WriteFile(source, []byte("# preserved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newFileTreeTestApp(t, root)
	tt := ui.NewTester(a.renderFilesSurface, 520, 360)

	if !tt.HasText("Projects") {
		t.Fatalf("collapsed folder missing from app Explorer: %v", tt.Texts())
	}
	projectLabel, _ := tt.Find("Projects")
	img := tt.Image()
	iconPixels := 0
	for y := maxInt(0, int(projectLabel.Y+projectLabel.H/2)-9); y < minInt(img.Bounds().Dy(), int(projectLabel.Y+projectLabel.H/2)+9); y++ {
		for x := maxInt(0, int(projectLabel.X)+24); x < minInt(img.Bounds().Dx(), int(projectLabel.X)+44); x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r+g+b > 0 {
				iconPixels++
			}
		}
	}
	if iconPixels == 0 {
		t.Fatalf("folder row did not render its fallback icon: label=%+v bounds=%v texts=%v", projectLabel, img.Bounds(), tt.Texts())
	}
	clickTreeRow(t, tt, "Projects")
	tt.Frame()
	if !tt.HasText("Archive") {
		t.Fatalf("clicking folder label did not expand its children: %v", tt.Texts())
	}
	clickTreeRow(t, tt, "Projects")
	tt.Frame()
	if tt.HasText("Archive") {
		t.Fatalf("clicking folder label did not collapse its children: %v", tt.Texts())
	}
	clickTreeRow(t, tt, "Projects")
	tt.Frame()
	sourceRow, ok := tt.Find("note.md")
	if !ok {
		t.Fatalf("draggable source row missing: %v", tt.Texts())
	}
	destRow, ok := tt.Find("Archive")
	if !ok {
		t.Fatalf("drop target row missing: %v", tt.Texts())
	}
	tt.Press(sourceRow.X+sourceRow.W/2, sourceRow.Y+sourceRow.H/2)
	tt.Move(destRow.X+destRow.W/2, destRow.Y+destRow.H/2)
	tt.Frame()
	tt.Release(destRow.X+destRow.W/2, destRow.Y+destRow.H/2)
	tt.Frame()
	moved := filepath.Join(dest, filepath.Base(source))
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("drop left source in place: stat err=%v", err)
	}
	content, err := os.ReadFile(moved)
	if err != nil {
		t.Fatalf("drop did not move file into target folder: %v", err)
	}
	if string(content) != "# preserved\n" {
		t.Fatalf("moved file contents = %q", content)
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
