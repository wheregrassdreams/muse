package nativeui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"muse/vaultfiles"
	"muse/vaultindex"
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
	if !a.quickOpen {
		t.Fatal("面板未打开")
	}
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if a.quickOpenSel != 0 {
		t.Fatalf("默认选中应为 0, got %d", a.quickOpenSel)
	}
	a.applyQuickOpen()
	if a.quickOpen {
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
	a.quickOpenValue = "B"
	cands := a.quickOpenCandidates()
	if len(cands) != 1 || filepath.Base(cands[0].path) != "笔记B.md" {
		t.Fatalf("过滤结果错误: %+v", cands)
	}
	a.quickOpenSel = 0
	a.applyQuickOpen()
	if a.path != filepath.Join(dir, "子", "笔记B.md") {
		t.Fatalf("应打开模糊命中的笔记B: path=%q", a.path)
	}

	// ↑↓ 选中候选后 Enter：无查询按路径字节序，index 1 是 子/笔记B
	a.openQuickOpen()
	tt.Frame()
	a.quickOpenSel = 1
	a.applyQuickOpen()
	if a.quickOpen || a.path != filepath.Join(dir, "子", "笔记B.md") {
		t.Fatalf("选中项应打开并关闭面板: path=%q open=%v", a.path, a.quickOpen)
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
	catalog := vaultfiles.New(dir)
	if err := catalog.Rebuild(); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.root = dir
	a.vault = &vaultRuntime{root: dir, files: catalog}
	a.quickOpenValue = "nested"
	candidates := a.quickOpenCandidates()
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
	files := vaultfiles.New(dir)
	if err := files.Rebuild(); err != nil {
		t.Fatal(err)
	}
	index := vaultindex.New()
	if err := index.Rebuild(dir); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.root = dir
	a.vault = &vaultRuntime{root: dir, files: files, index: index}
	a.quickOpenValue = "launch"
	candidates := a.quickOpenCandidates()
	if len(candidates) != 1 || candidates[0].path != path {
		t.Fatalf("alias did not find the canonical path: %+v", candidates)
	}
	if title := a.quickOpenTitle(path); title != "Project Overview" {
		t.Fatalf("display title = %q", title)
	}
	if got := filepath.ToSlash(filepath.Join(a.root, "Projects", "Atlas", "Note.md")); got != filepath.ToSlash(path) {
		t.Fatalf("canonical target changed: %q", got)
	}
}

func TestQuickOpenCandidatesCacheByQueryAndCatalogRevision(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "alpha-note.md")
	if err := os.WriteFile(firstPath, []byte("# Alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := vaultfiles.New(dir)
	if err := files.Rebuild(); err != nil {
		t.Fatal(err)
	}
	index := vaultindex.New()
	if err := index.Rebuild(dir); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.root = dir
	a.vault = &vaultRuntime{root: dir, files: files, index: index}
	a.quickOpenValue = "alpha"

	first := a.quickOpenCandidates()
	if len(first) != 1 || first[0].path != firstPath {
		t.Fatalf("initial candidates = %+v", first)
	}
	firstAgain := a.quickOpenCandidates()
	if len(firstAgain) != 1 || &firstAgain[0] != &first[0] {
		t.Fatal("same query and catalog revision did not reuse candidate storage")
	}

	a.quickOpenValue = "note"
	changedQuery := a.quickOpenCandidates()
	if len(changedQuery) != 1 || &changedQuery[0] == &first[0] {
		t.Fatal("query change did not recompute candidates")
	}

	secondPath := filepath.Join(dir, "beta.md")
	if err := os.WriteFile(secondPath, []byte("# Beta"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := files.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if err := index.Rebuild(dir); err != nil {
		t.Fatal(err)
	}
	a.quickOpenValue = ""
	changedRevision := a.quickOpenCandidates()
	if len(changedRevision) != 2 {
		t.Fatalf("revision change kept stale candidates: %+v", changedRevision)
	}
}

func TestCloseQuickOpenReleasesCandidateResults(t *testing.T) {
	a := New()
	a.quickOpen = true
	a.quickOpenCandidatesReady = true
	a.quickOpenCandidateQuery = "needle"
	a.quickOpenCandidateRows = []quickOpenCandidate{{fileRow: fileRow{path: "/tmp/note.md"}, pathMatches: []int{0, 1}}}

	a.closeQuickOpen()

	if a.quickOpen || a.quickOpenCandidatesReady || a.quickOpenCandidateRows != nil || a.quickOpenCandidateQuery != "" {
		t.Fatal("closing Quick Open retained candidate results")
	}
}

func TestSetWorkspaceReleasesQuickOpenCachesFromPreviousVault(t *testing.T) {
	firstRoot, nextRoot := t.TempDir(), t.TempDir()
	firstPath := filepath.Join(firstRoot, "old-note.md")
	if err := os.WriteFile(firstPath, []byte("# Old"), 0o644); err != nil {
		t.Fatal(err)
	}
	firstFiles := vaultfiles.New(firstRoot)
	if err := firstFiles.Rebuild(); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.root = firstRoot
	a.vault = &vaultRuntime{root: firstRoot, files: firstFiles}
	a.quickOpenValue = "old"
	oldCandidates := a.quickOpenCandidates()
	if len(oldCandidates) != 1 || oldCandidates[0].path != firstPath {
		t.Fatalf("old candidates = %+v", oldCandidates)
	}

	a.setWorkspace(nextRoot)
	if a.quickOpenCatalogVault != nil || a.quickOpenRows != nil || a.quickOpenTitles != nil || a.quickOpenAliases != nil || a.quickOpenCatalogReady || a.quickOpenCandidateRows != nil || a.quickOpenCandidatesReady {
		t.Fatal("workspace switch retained the previous vault's Quick Open caches")
	}

	nextPath := filepath.Join(nextRoot, "next-note.md")
	if err := os.WriteFile(nextPath, []byte("# Next"), 0o644); err != nil {
		t.Fatal(err)
	}
	nextFiles := vaultfiles.New(nextRoot)
	if err := nextFiles.Rebuild(); err != nil {
		t.Fatal(err)
	}
	nextVault := &vaultRuntime{root: nextRoot, files: nextFiles}
	a.vault = nextVault
	a.quickOpenValue = "next"
	newCandidates := a.quickOpenCandidates()
	if len(newCandidates) != 1 || newCandidates[0].path != nextPath || &newCandidates[0] == &oldCandidates[0] {
		t.Fatalf("new vault reused old candidate results: %+v", newCandidates)
	}
	if a.quickOpenCatalogVault != nextVault {
		t.Fatal("Quick Open catalog retained the old vault runtime")
	}

	replacementVault := &vaultRuntime{root: nextRoot, files: nextFiles}
	a.vault = replacementVault
	a.clearQuickOpenCatalogIfVaultChanged()
	if a.quickOpenCatalogVault != nil || a.quickOpenRows != nil || a.quickOpenCandidateRows != nil || a.quickOpenCatalogReady || a.quickOpenCandidatesReady {
		t.Fatal("vault runtime replacement retained prior catalog or candidates")
	}
	a.vault = replacementVault
	a.quickOpenValue = "next"
	if got := a.quickOpenCandidates(); len(got) != 1 {
		t.Fatalf("replacement vault candidates = %+v", got)
	}
	a.vault = nil
	a.quickOpenCatalog()
	if a.quickOpenCatalogVault != nil || a.quickOpenRows != nil || a.quickOpenTitles != nil || a.quickOpenAliases != nil || a.quickOpenCatalogReady || a.quickOpenCandidateRows != nil || a.quickOpenCandidatesReady {
		t.Fatal("nil-vault catalog fallback retained previous runtime or candidates")
	}
}
