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

	editorui "muse/nativeui/workbench/features/editor"
	findreplaceui "muse/nativeui/workbench/features/search/findreplace"
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

	a := newFileTreeTestApp(t, dir)
	a.setWorkspace(dir)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()

	clickTreeRow(t, tt, "周报.md")
	tt.Frame()
	if a.documentPath() != one {
		t.Fatalf("a.documentPath() = %q, want %q", a.documentPath(), one)
	}
	if !strings.Contains(a.documentSource(), "原始行") {
		t.Fatalf("file not loaded: %q", a.documentSource())
	}

	// Click the rendered list item, go to its end and type.
	clickSource(t, a, tt, "原始行")
	tt.Key(0, ui.KeyEnd)
	tt.Type(" E2E 追加")
	tt.Frame()
	if !a.documentDirty() {
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
	if a.documentDirty() {
		t.Fatalf("dirty stayed true after a successful save")
	}
}

func TestE2ESaveUntitledViaDialog(t *testing.T) {
	a := New()
	withDocumentPorts(t, a)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()

	// A new window is an empty document with the caret in it: type away.
	a.noteEditor().FocusCommands().Focus()
	tt.Frame()
	tt.Type("# E2E 已验证")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Type("正文")
	tt.Frame()
	if a.documentSource() != "# E2E 已验证\n正文" {
		t.Fatalf("source = %q", a.documentSource())
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
	for a.documentPath() != target && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if a.documentPath() != target {
		t.Fatalf("a.documentPath() = %q, want %q", a.documentPath(), target)
	}
}

func TestE2ESwitchFilesKeepsSavedState(t *testing.T) {
	dir, one := e2eWorkspace(t)
	two := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(two, []byte("# 笔记\n\n第二份文档。\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := newFileTreeTestApp(t, dir)
	a.setWorkspace(dir)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()

	clickTreeRow(t, tt, "周报.md")
	tt.Frame()
	clickSource(t, a, tt, "原始行")
	tt.Key(0, ui.KeyEnd)
	tt.Type(" 已保存再切走")
	tt.Frame()
	a.save()
	waitFileContains(t, one, "已保存再切走")
	if a.documentDirty() {
		t.Fatal("dirty after save")
	}

	clickTreeRow(t, tt, "notes.md")
	tt.Frame()
	if a.documentPath() != two || !strings.Contains(a.documentSource(), "第二份文档。") {
		t.Fatalf("notes.md not opened: path=%q source=%q", a.documentPath(), a.documentSource())
	}
	clickTreeRow(t, tt, "周报.md")
	tt.Frame()
	if !strings.Contains(a.documentSource(), "已保存再切走") {
		t.Fatalf("saved edit not loaded after switching back: %q", a.documentSource())
	}
	if a.documentPath() != one {
		t.Fatalf("a.documentPath() = %q, want %q", a.documentPath(), one)
	}
}

func TestE2ETableCmdEnterAddsRow(t *testing.T) {
	dir := t.TempDir()
	one := filepath.Join(dir, "表.md")
	if err := os.WriteFile(one, []byte("| a | b |\n| --- | --- |\n| 1 | 2 |\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newFileTreeTestApp(t, dir)
	a.setWorkspace(dir)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	clickTreeRow(t, tt, "表.md")
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
	body := got
	if _, after, ok := strings.Cut(body, "---\n\n"); ok {
		body = after
	}
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
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
	a := newFileTreeTestApp(t, dir)
	a.setWorkspace(dir)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	clickTreeRow(t, tt, "crlf.md")
	tt.Frame()
	if strings.Contains(a.documentSource(), "\r") {
		t.Fatalf("the editor sees carriage returns: %q", a.documentSource())
	}
	clickSource(t, a, tt, "正文")
	tt.Key(0, ui.KeyEnd)
	tt.Type("。")
	tt.Frame()
	a.save()
	got := waitFileContains(t, one, "正文。")
	if !strings.HasSuffix(got, "# 标题\r\n\r\n正文。\r\n") {
		t.Fatalf("saved body did not preserve Windows line endings: %q", got)
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
	if want := strings.Index(a.documentSource(), "二"); a.noteEditor().NavigationCommands().Caret() != want {
		t.Fatalf("caret = %d, want %d", a.noteEditor().NavigationCommands().Caret(), want)
	}
}

func TestSearchSelectsMatch(t *testing.T) {
	a := New()
	a.loadDoc("第一段 Muse\n\n第二段 muse 结束\n")
	a.showPanel(panelSearch)
	a.findReplace.Apply(findreplaceui.Action{Kind: findreplaceui.QueryChanged, Query: "muse"})
	a.refreshSearch()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	if count := len(a.findReplace.Snapshot().Matches); count != 2 {
		t.Fatalf("matches = %d, want 2", count)
	}
	a.goToHit(1)
	tt.Frame()
	if got := a.noteEditor().ClipboardCommands().SelectedText(); got != "muse" {
		t.Fatalf("selected %q", got)
	}
}

func TestAIEditIsUndoable(t *testing.T) {
	a := New()
	a.loadDoc("原文。\n")
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	a.noteEditor().ReplaceUser("原文。\n\nAI 新增。\n")
	tt.Frame()
	if !a.documentDirty() || a.documentSource() != "原文。\n\nAI 新增。\n" {
		t.Fatalf("tool edit not applied: dirty=%v source=%q", a.documentDirty(), a.documentSource())
	}
	a.noteEditor().Undo()
	tt.Frame()
	if a.documentSource() != "原文。\n" {
		t.Fatalf("undo left %q", a.documentSource())
	}
}

func TestCodeFilesOpenAsCode(t *testing.T) {
	if editorui.CodeLanguageFor("a.md") != "" || editorui.CodeLanguageFor("README") != "" {
		t.Fatal("Markdown files open as code")
	}
	if editorui.CodeLanguageFor("main.go") == "" || editorui.CodeLanguageFor("notes.txt") != "text" {
		t.Fatalf("code files: go=%q txt=%q", editorui.CodeLanguageFor("main.go"), editorui.CodeLanguageFor("notes.txt"))
	}
}

func TestSearchReplace(t *testing.T) {
	a := New()
	a.loadDoc("cat and cat and Cat\n")
	a.showPanel(panelSearch)
	a.findReplace.Apply(findreplaceui.Action{Kind: findreplaceui.QueryChanged, Query: "cat"})
	a.findReplace.Apply(findreplaceui.Action{Kind: findreplaceui.ReplacementChanged, Text: "dog"})
	a.refreshSearch()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	a.goToHit(1)
	a.replaceHit()
	if a.documentSource() != "cat and dog and Cat\n" {
		t.Fatalf("replace one: %q", a.documentSource())
	}
	a.replaceAllHits()
	if a.documentSource() != "dog and dog and dog\n" {
		t.Fatalf("replace all: %q", a.documentSource())
	}
	a.noteEditor().Undo()
	if a.documentSource() != "cat and dog and Cat\n" {
		t.Fatalf("undo replace all: %q", a.documentSource())
	}
}
