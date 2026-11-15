package nativeui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestRecentPersistence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MUSE_CONFIG_DIR", dir)

	a := New()
	a.addRecent(filepath.Join(dir, "a", "x.md"), false)
	a.addRecent(filepath.Join(dir, "proj"), true)
	// 重复加入：去重并挪到最前
	a.addRecent(filepath.Join(dir, "a", "x.md"), false)

	if len(a.recents) != 2 || a.recents[0].Path != filepath.Join(dir, "a", "x.md") {
		t.Fatalf("recents = %+v", a.recents)
	}
	if !a.recents[1].IsDir {
		t.Fatalf("folder flag lost: %+v", a.recents[1])
	}
	loaded := loadRecents()
	if len(loaded) != 2 || loaded[0].Path != filepath.Join(dir, "a", "x.md") {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestRecentLabelsDedupAndClip(t *testing.T) {
	rs := []recentEntry{
		{Path: "/w/proj/notes.md"},
		{Path: "/w/other/notes.md"},
		{Path: "/w/一个特别特别特别特别长长长长长长长长长长长的文件夹名字啊"},
	}
	labels := recentLabels(rs)
	if labels[0] != "proj/notes.md" || labels[1] != "other/notes.md" {
		t.Fatalf("重名没有带父目录: %q", labels)
	}
	if got := []rune(labels[2]); len(got) > 24 || !strings.HasSuffix(labels[2], "…") {
		t.Fatalf("超长没有截断: %q (%d runes)", labels[2], len(got))
	}
}

// TestEmptyStateMenuListsRecents 空态按钮的下拉里有最近打开，
// 点条目能回到对应文件夹/文件；已失效的路径不再出现。
func TestEmptyStateMenuListsRecents(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MUSE_CONFIG_DIR", dir)
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
	a.addRecent(proj, true)
	a.addRecent(notes, false)
	a.addRecent(gone, false)

	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	// 文案已经搬到按钮上，不再单独占一行
	if !tt.HasText("打开文件或文件夹") {
		t.Fatalf("按钮文案缺失; texts=%q", tt.Texts())
	}
	if err := tt.Click("打开文件或文件夹"); err != nil {
		t.Fatalf("open menu: %v", err)
	}
	items := strings.Join(tt.Menu(), ",")
	if !strings.Contains(items, "project") || !strings.Contains(items, "notes.md") {
		t.Fatalf("menu missing recents: %q", items)
	}
	if strings.Contains(items, "gone.md") {
		t.Fatalf("失效路径不应出现: %q", items)
	}

	if err := tt.ChooseMenuItem("project"); err != nil {
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

// TestEmptyStateMenuOpensRecentFile 从空态的最近列表打开一个文件。
// 走新应用实例，顺带验证记录是持久化过的。
func TestEmptyStateMenuOpensRecentFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MUSE_CONFIG_DIR", dir)
	proj := filepath.Join(dir, "project")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(proj, "notes.md")
	if err := os.WriteFile(notes, []byte("# Notes"+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	first := New()
	first.addRecent(notes, false)
	if err := saveRecents(first.recents); err != nil {
		t.Fatal(err)
	}

	// 新实例从盘上读回记录
	a := New()
	if len(a.recents) != 1 || a.recents[0].Path != notes {
		t.Fatalf("recents not persisted: %+v", a.recents)
	}
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if err := tt.Click("打开文件或文件夹"); err != nil {
		t.Fatalf("open menu: %v", err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("notes.md"); err != nil {
		t.Fatalf("choose notes: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && a.path != notes {
		time.Sleep(5 * time.Millisecond)
	}
	if a.path != notes {
		t.Fatalf("path = %q, want %q", a.path, notes)
	}
}
