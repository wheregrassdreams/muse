package services

import (
	"os"
	"path/filepath"
	"testing"
)

// RenameDocument 用于「标题即文件名」：文档可以从工作区之外打开，所以这里不设工作区限制。
func TestRenameDocumentOutsideWorkspace(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "Untitled.md")
	if err := os.WriteFile(old, []byte("# 标题\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	f := &Files{} // 零值足够：Rename 路径只用到 mu / recent / recentFile
	want := filepath.Join(dir, "标题.md")
	if got := f.RenameDocument(old, "标题.md"); got != want {
		t.Fatalf("RenameDocument = %q, want %q", got, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("改名后的文件不存在: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("旧文件仍然存在: %v", err)
	}
}

// 目标已被占用时不能静默覆盖，直接失败。
func TestRenameDocumentRefusesExistingTarget(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "Untitled.md")
	occupied := filepath.Join(dir, "占位.md")
	for _, p := range []string{old, occupied} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	f := &Files{}
	if got := f.RenameDocument(old, "占位.md"); got != "" {
		t.Fatalf("目标已存在时应返回空串，得到 %q", got)
	}
	data, err := os.ReadFile(occupied)
	if err != nil || string(data) != "x" {
		t.Fatalf("已存在的文件被改动了: %q %v", data, err)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("改名失败后原文件不应消失: %v", err)
	}
}

// 只接受纯文件名，且只对文本类文件生效。
func TestRenameDocumentRejectsBadNames(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "a.md")
	if err := os.WriteFile(md, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "b.bin")
	if err := os.WriteFile(bin, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	f := &Files{}
	for _, name := range []string{"", "  ", "../逃逸.md", "子目录/文件.md", ".", ".."} {
		if got := f.RenameDocument(md, name); got != "" {
			t.Fatalf("非法文件名 %q 应被拒绝，得到 %q", name, got)
		}
	}
	if got := f.RenameDocument(bin, "c.bin"); got != "" {
		t.Fatalf("非文本文件不应改名，得到 %q", got)
	}
	if got := f.RenameDocument(filepath.Join(dir, "不存在.md"), "x.md"); got != "" {
		t.Fatalf("源文件不存在应返回空串，得到 %q", got)
	}
}
