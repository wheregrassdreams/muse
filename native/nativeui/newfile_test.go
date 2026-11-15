package nativeui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/document/frontmatter"
)

// 新建文件（面板工具栏）→ 文件落盘 → 自动打开：整条链路一次走通。
func TestNewFileToolbarCreatesAndOpens(t *testing.T) {
	dir := t.TempDir()
	a := New()
	a.root = dir

	tt := ui.NewTester(a.View, 1200, 800)
	if err := tt.Click("新建文档"); err != nil {
		t.Fatalf("点击新建文件失败: %v", err)
	}

	created := filepath.Join(dir, "未命名.md")
	if _, err := os.Stat(created); err != nil {
		t.Fatalf("文件未创建: %v", err)
	}
	if a.path != created {
		t.Fatalf("新文件未自动打开: path=%q", a.path)
	}
	doc := frontmatter.Parse(a.source)
	fields := map[string]frontmatter.Field{}
	for _, f := range doc.Fields {
		fields[f.Key] = f
	}
	for _, key := range []string{"type", "tags", "aliases", "created_at", "updated_at"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("新文件缺少核心字段 %q: %q", key, a.source)
		}
	}
	if fields["type"].Value != "note" || fields["created_at"].Value == "" || fields["updated_at"].Value != "" {
		t.Fatalf("新文件核心字段初始值错误: %+v", fields)
	}

}
