package nativeui

// End-to-end flows over the real App view driven by the headless UI tester:
// open a file from the workspace tree, edit it in the editor, save, and
// verify the bytes that reach disk. These are the same seams the shipped app
// uses (no test-only code paths beyond the dialog stubs).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// save() writes asynchronously on the UI queue; tests poll the file like a
// user's editor would observe it.
func waitFileContains(t *testing.T, path, needle string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var got string
	for {
		b, err := os.ReadFile(path)
		if err == nil {
			got = string(b)
			if strings.Contains(got, needle) {
				return got
			}
		}
		if time.Now().After(deadline) {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func e2eWorkspace(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	one := filepath.Join(dir, "周报.md")
	src := "# 缪斯周报\n\n## 本周进展\n\n- 原始行\n\n> 所写，即所见。\n"
	if err := os.WriteFile(one, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, one
}

func TestE2EOpenEditSaveRoundTrip(t *testing.T) {
	dir, one := e2eWorkspace(t)

	a := New()
	a.setWorkspace(dir)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()

	if err := tt.Click("周报.md"); err != nil {
		t.Fatalf("open file from tree: %v", err)
	}
	tt.Frame()
	if a.path != one {
		t.Fatalf("a.path = %q, want %q", a.path, one)
	}
	if !strings.Contains(a.source, "原始行") {
		t.Fatalf("file not loaded: %q", a.source)
	}

	// Click the rendered list item, go to its end and type.
	clickSource(t, a, tt, "原始行")
	tt.Key(0, ui.KeyEnd)
	tt.Type(" E2E 追加")
	tt.Frame()
	if !a.dirty {
		t.Fatal("typing did not mark the document dirty")
	}

	a.save()
	tt.Frame()

	got := waitFileContains(t, one, "E2E 追加")
	for _, want := range []string{"# 缪斯周报", "## 本周进展", "- 原始行 E2E 追加", "> 所写，即所见。"} {
		if !strings.Contains(got, want) {
			t.Fatalf("saved markdown missing %q; got:\n%s", want, got)
		}
	}
	if a.dirty {
		t.Fatalf("dirty stayed true after a successful save")
	}
}

func TestE2ESaveUntitledViaDialog(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()

	// A new window is an empty document with the caret in it: type away.
	a.ed.Focus()
	tt.Frame()
	tt.Type("# E2E 已验证")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Type("正文")
	tt.Frame()
	if a.source != "# E2E 已验证\n正文" {
		t.Fatalf("source = %q", a.source)
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "e2e-untitled.md")
	old := saveDialog
	saveDialog = func(mygo.SaveDialogOptions) (string, error) { return target, nil }
	defer func() { saveDialog = old }()

	a.save()
	tt.Frame()

	b := []byte(waitFileContains(t, target, "E2E 已验证"))
	if !strings.Contains(string(b), "# E2E 已验证\n正文") {
		t.Fatalf("saved doc missing typed text; got:\n%s", b)
	}
	deadline := time.Now().Add(2 * time.Second)
	for a.path != target && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if a.path != target {
		t.Fatalf("a.path = %q, want %q", a.path, target)
	}
}

func TestE2ESwitchFilesKeepsSavedState(t *testing.T) {
	dir, one := e2eWorkspace(t)
	two := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(two, []byte("# 笔记\n\n第二份文档。\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := New()
	a.setWorkspace(dir)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()

	if err := tt.Click("周报.md"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	clickSource(t, a, tt, "原始行")
	tt.Key(0, ui.KeyEnd)
	tt.Type(" 已保存再切走")
	tt.Frame()
	a.save()
	waitFileContains(t, one, "已保存再切走")
	if a.dirty {
		t.Fatal("dirty after save")
	}

	if err := tt.Click("notes.md"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.path != two || !strings.Contains(a.source, "第二份文档。") {
		t.Fatalf("notes.md not opened: path=%q source=%q", a.path, a.source)
	}
	if err := tt.Click("周报.md"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !strings.Contains(a.source, "已保存再切走") {
		t.Fatalf("saved edit not loaded after switching back: %q", a.source)
	}
	if a.path != one {
		t.Fatalf("a.path = %q, want %q", a.path, one)
	}
}

func TestE2ETableCmdEnterAddsRow(t *testing.T) {
	dir := t.TempDir()
	one := filepath.Join(dir, "表.md")
	if err := os.WriteFile(one, []byte("| a | b |\n| --- | --- |\n| 1 | 2 |\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.setWorkspace(dir)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if err := tt.Click("表.md"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	clickSource(t, a, tt, "1 |")
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Frame()
	tt.Type("新行")
	tt.Frame()
	a.save()
	got := waitFileContains(t, one, "新行")
	if !strings.Contains(got, "新行") {
		t.Fatalf("saved table missing the new row; got:\n%s", got)
	}
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected header+alignment+2 body rows, got %d lines:\n%s", len(lines), got)
	}
	if !strings.HasPrefix(lines[3], "| 新行") {
		t.Fatalf("new row = %q", lines[3])
	}
}

func TestE2EWindowsLineEndingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	one := filepath.Join(dir, "crlf.md")
	if err := os.WriteFile(one, []byte("# 标题\r\n\r\n正文\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.setWorkspace(dir)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if err := tt.Click("crlf.md"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if strings.Contains(a.source, "\r") {
		t.Fatalf("the editor sees carriage returns: %q", a.source)
	}
	clickSource(t, a, tt, "正文")
	tt.Key(0, ui.KeyEnd)
	tt.Type("。")
	tt.Frame()
	a.save()
	got := waitFileContains(t, one, "正文。")
	if got != "# 标题\r\n\r\n正文。\r\n" {
		t.Fatalf("saved %q", got)
	}
}

func TestOutlineClickMovesCaret(t *testing.T) {
	a := New()
	a.loadDoc("# 一\n\n正文\n\n## 二\n\n更多\n")
	a.showPanel(panelOutline)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if err := tt.Click("二"); err != nil {
		t.Fatalf("outline row: %v", err)
	}
	tt.Frame()
	if want := strings.Index(a.source, "二"); a.ed.Caret() != want {
		t.Fatalf("caret = %d, want %d", a.ed.Caret(), want)
	}
}

func TestSearchSelectsMatch(t *testing.T) {
	a := New()
	a.loadDoc("第一段 Muse\n\n第二段 muse 结束\n")
	a.showPanel(panelSearch)
	a.query = "muse"
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	if a.matches != 2 {
		t.Fatalf("matches = %d, want 2", a.matches)
	}
	a.goToHit(1)
	tt.Frame()
	if got := a.ed.SelectedText(); got != "muse" {
		t.Fatalf("selected %q", got)
	}
}

func TestAIEditIsUndoable(t *testing.T) {
	a := New()
	a.loadDoc("原文。\n")
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	a.applyToolSourceEdit("原文。\n\nAI 新增。\n")
	tt.Frame()
	if !a.dirty || a.source != "原文。\n\nAI 新增。\n" {
		t.Fatalf("tool edit not applied: dirty=%v source=%q", a.dirty, a.source)
	}
	a.ed.Undo()
	tt.Frame()
	if a.source != "原文。\n" {
		t.Fatalf("undo left %q", a.source)
	}
}

func TestCodeFilesOpenAsCode(t *testing.T) {
	if codeLanguage("a.md") != "" || codeLanguage("README") != "" {
		t.Fatal("Markdown files open as code")
	}
	if codeLanguage("main.go") == "" || codeLanguage("notes.txt") != "text" {
		t.Fatalf("code files: go=%q txt=%q", codeLanguage("main.go"), codeLanguage("notes.txt"))
	}
}

func TestSearchReplace(t *testing.T) {
	a := New()
	a.loadDoc("cat and cat and Cat\n")
	a.showPanel(panelSearch)
	a.query = "cat"
	a.replaceWith = "dog"
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	a.goToHit(1)
	a.replaceHit()
	if a.source != "cat and dog and Cat\n" {
		t.Fatalf("replace one: %q", a.source)
	}
	a.replaceAllHits()
	if a.source != "dog and dog and dog\n" {
		t.Fatalf("replace all: %q", a.source)
	}
	a.ed.Undo()
	a.docChanged()
	if a.source != "cat and dog and Cat\n" {
		t.Fatalf("undo replace all: %q", a.source)
	}
}
