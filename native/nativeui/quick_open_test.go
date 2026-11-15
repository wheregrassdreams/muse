package nativeui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	vaultfilessource "muse/platform/filesystem/vaultfilessource"
	vaultindexsource "muse/platform/filesystem/vaultindexsource"
	vaultfiles "muse/vault/files"
	vaultindex "muse/vault/index"
)

// ⌘O 模糊跳转：打开面板 → 输入过滤 → Enter/点击打开目标文件。
func TestQuickOpenFuzzyJump(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "子"), 0o755)
	os.WriteFile(filepath.Join(dir, "笔记A.md"), []byte("# A"), 0o644)
	os.WriteFile(filepath.Join(dir, "子", "笔记B.md"), []byte("# B"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "aardvark.txt"), []byte("x"), 0o644)

	a := New()
	a.setWorkspace(dir)

	// 无查询时列出全部文件，Enter 打开选中项
	a.openQuickOpen()
	if !a.quickOpenOpen() {
		t.Fatal("面板未打开")
	}
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.quickOpenOpen() {
		t.Fatal("打开后面板未关闭")
	}
	// 无查询按路径排序，aardvark.txt 排最前
	if a.path != filepath.Join(dir, "aardvark.txt") {
		t.Fatalf("Enter 应打开第一个候选: path=%q", a.path)
	}

	// 展开子目录后 "B" 模糊命中 子/笔记B.md（相对路径评分，
	// 绝对路径里的字符不算命中——aardvark.txt 不应混进来）
	a.openQuickOpen()
	a.expanded[filepath.Join(dir, "子")] = true
	a.fileRowsDirty = true
	tt.Frame()
	tt.Type("B")
	tt.Frame()
	cands := a.quickOpenCandidates("B")
	if len(cands) != 1 || filepath.Base(cands[0].path) != "笔记B.md" {
		t.Fatalf("过滤结果错误: %+v", cands)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.path != filepath.Join(dir, "子", "笔记B.md") {
		t.Fatalf("应打开模糊命中的笔记B: path=%q", a.path)
	}

	// ↑↓ 选中候选后 Enter：无查询按路径字节序，index 1 是 子/笔记B
	a.openQuickOpen()
	tt.Frame()
	tt.Key(ui.Ctrl, ui.KeyN)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.quickOpenOpen() || a.path != filepath.Join(dir, "子", "笔记B.md") {
		t.Fatalf("选中项应打开并关闭面板: path=%q open=%v", a.path, a.quickOpenOpen())
	}
}

func TestQuickOpenUsesVaultCatalogForCollapsedFiles(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "closed", "nested-note.md")
	if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte("# nested"), 0o644); err != nil {
		t.Fatal(err)
	}
	source, err := vaultfilessource.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	catalog := vaultfiles.New(source)
	if err := catalog.Rebuild(); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.root = dir
	a.vault = &vaultRuntime{root: dir, files: catalog}
	candidates := a.quickOpenCandidates("nested")
	if len(candidates) != 1 || candidates[0].path != nested {
		t.Fatalf("collapsed nested file missing from quick open: %+v", candidates)
	}
}

func TestQuickOpenMatchesIndexedTitleAndAliasAndKeepsPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Projects", "Atlas", "Note.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\naliases: [Launch plan]\n---\n# Project Overview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	filesSource, err := vaultfilessource.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := vaultfiles.New(filesSource)
	if err := files.Rebuild(); err != nil {
		t.Fatal(err)
	}
	indexSource, err := vaultindexsource.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	index := vaultindex.New(indexSource)
	if err := index.Rebuild(); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.root = dir
	a.vault = &vaultRuntime{root: dir, files: files, index: index}
	candidates := a.quickOpenCandidates("launch")
	if len(candidates) != 1 || candidates[0].path != path {
		t.Fatalf("alias did not find the canonical path: %+v", candidates)
	}
	if title := candidates[0].title; title != "Project Overview" {
		t.Fatalf("display title = %q", title)
	}
	if got := filepath.ToSlash(filepath.Join(a.root, "Projects", "Atlas", "Note.md")); got != filepath.ToSlash(path) {
		t.Fatalf("canonical target changed: %q", got)
	}
}

func TestQuickOpenRefreshesAfterCatalogRevision(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "alpha-note.md")
	if err := os.WriteFile(firstPath, []byte("# Alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	filesSource, err := vaultfilessource.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := vaultfiles.New(filesSource)
	if err := files.Rebuild(); err != nil {
		t.Fatal(err)
	}
	indexSource, err := vaultindexsource.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	index := vaultindex.New(indexSource)
	if err := index.Rebuild(); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.root = dir
	a.vault = &vaultRuntime{root: dir, files: files, index: index}
	first := a.quickOpenCandidates("alpha")
	if len(first) != 1 || first[0].path != firstPath {
		t.Fatalf("initial candidates = %+v", first)
	}
	changedQuery := a.quickOpenCandidates("note")
	if len(changedQuery) != 1 || changedQuery[0].path != firstPath {
		t.Fatalf("query change returned unexpected candidates: %+v", changedQuery)
	}

	secondPath := filepath.Join(dir, "beta.md")
	if err := os.WriteFile(secondPath, []byte("# Beta"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := files.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if err := index.Rebuild(); err != nil {
		t.Fatal(err)
	}
	changedRevision := a.quickOpenCandidates("")
	if len(changedRevision) != 2 {
		t.Fatalf("revision change kept stale candidates: %+v", changedRevision)
	}
}

func TestCloseQuickOpenClosesDialog(t *testing.T) {
	a := New()
	a.closeQuickOpen()

	if a.quickOpenOpen() {
		t.Fatal("closing Quick Open left the dialog open")
	}
}

func TestSetWorkspaceReleasesQuickOpenCachesFromPreviousVault(t *testing.T) {
	firstRoot, nextRoot := t.TempDir(), t.TempDir()
	firstPath := filepath.Join(firstRoot, "old-note.md")
	if err := os.WriteFile(firstPath, []byte("# Old"), 0o644); err != nil {
		t.Fatal(err)
	}
	firstSource, err := vaultfilessource.New(firstRoot)
	if err != nil {
		t.Fatal(err)
	}
	firstFiles := vaultfiles.New(firstSource)
	if err := firstFiles.Rebuild(); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.root = firstRoot
	a.vault = &vaultRuntime{root: firstRoot, files: firstFiles}
	oldCandidates := a.quickOpenCandidates("old")
	if len(oldCandidates) != 1 || oldCandidates[0].path != firstPath {
		t.Fatalf("old candidates = %+v", oldCandidates)
	}

	a.setWorkspace(nextRoot)
	nextPath := filepath.Join(nextRoot, "next-note.md")
	if err := os.WriteFile(nextPath, []byte("# Next"), 0o644); err != nil {
		t.Fatal(err)
	}
	nextSource, err := vaultfilessource.New(nextRoot)
	if err != nil {
		t.Fatal(err)
	}
	nextFiles := vaultfiles.New(nextSource)
	if err := nextFiles.Rebuild(); err != nil {
		t.Fatal(err)
	}
	nextVault := &vaultRuntime{root: nextRoot, files: nextFiles}
	a.vault = nextVault
	newCandidates := a.quickOpenCandidates("next")
	if len(newCandidates) != 1 || newCandidates[0].path != nextPath || newCandidates[0].path == oldCandidates[0].path {
		t.Fatalf("new vault reused old candidate results: %+v", newCandidates)
	}

	replacementVault := &vaultRuntime{root: nextRoot, files: nextFiles}
	a.vault = replacementVault
	a.vault = replacementVault
	if got := a.quickOpenCandidates("next"); len(got) != 1 {
		t.Fatalf("replacement vault candidates = %+v", got)
	}
	a.vault = nil
	if got := a.quickOpenCandidates(); len(got) != 0 {
		t.Fatalf("nil vault returned stale candidates: %+v", got)
	}
}
