package nativeui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// A dialog call must return at once from the UI thread; these fakes stand in
// for the system pickers, whose real implementations block until chosen.
func TestOpenFileDialogReturnsImmediately(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.md")
	if err := os.WriteFile(file, []byte("# One\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := openDialog
	openDialog = func(mygo.OpenDialogOptions) ([]string, error) {
		return []string{file}, nil
	}
	defer func() { openDialog = old }()

	a := New()
	done := make(chan struct{})
	go func() {
		a.openFileDialog()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("openFileDialog blocked")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && a.path != file {
		time.Sleep(5 * time.Millisecond)
	}
	if a.path != file {
		t.Fatalf("path = %q, want %q", a.path, file)
	}
	if a.busy {
		t.Fatal("busy stayed set")
	}
}

func TestOpenFolderDialogReturnsImmediately(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "one.md"), []byte("# One\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := openDialog
	openDialog = func(mygo.OpenDialogOptions) ([]string, error) {
		return []string{dir}, nil
	}
	defer func() { openDialog = old }()

	a := New()
	done := make(chan struct{})
	go func() {
		a.openFolderDialog()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("openFolderDialog blocked")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && (a.root != dir || a.busy) {
		time.Sleep(5 * time.Millisecond)
	}
	if a.root != dir {
		t.Fatalf("root = %q, want %q", a.root, dir)
	}
	if a.busy {
		t.Fatal("busy stayed set")
	}
}

// The empty files panel used to stack a folder button and a "或打开单个文件"
// link; both now live in one 打开… menu button, one row high.
func TestFilesEmptyStateOpensFileOrFolderFromOneButton(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "one.md"), []byte("# One\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := openDialog
	openDialog = func(opts mygo.OpenDialogOptions) ([]string, error) {
		if !opts.Directory {
			t.Error("the folder item should ask for a directory")
		}
		return []string{dir}, nil
	}
	defer func() { openDialog = old }()

	a := New()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !tt.HasText("打开文件或文件夹") {
		t.Fatalf("empty state copy missing; texts=%q", tt.Texts())
	}
	if tt.HasText("或打开单个文件") {
		t.Fatal("the separate open-a-file line is still rendered")
	}
	if err := tt.Click("打开文件或文件夹"); err != nil {
		t.Fatalf("open menu: %v", err)
	}
	items := strings.Join(tt.Menu(), ",")
	if !strings.Contains(items, "打开文件…") || !strings.Contains(items, "打开文件夹…") {
		t.Fatalf("menu items = %q", items)
	}
	if err := tt.ChooseMenuItem("打开文件夹…"); err != nil {
		t.Fatalf("choose folder: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && (a.root != dir || a.busy) {
		time.Sleep(5 * time.Millisecond)
	}
	if a.root != dir || a.busy {
		t.Fatalf("root = %q busy=%v, want %q", a.root, a.busy, dir)
	}
}
