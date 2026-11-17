package nativeui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestEditorKeepsSingleSourceNewlineOnItsOwnVisualRow(t *testing.T) {
	const src = "这一行接着写，用来检查单换行在排版里是否成行。\n第二行。"
	a := New()
	a.loadDoc(src)
	if a.editorOptions().SoftBreaksAsSpaces {
		t.Fatal("editor options still fold single source newlines into spaces")
	}

	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	first := strings.Index(src, "这一行")
	second := strings.Index(src, "第二行")
	_, firstY, ok := a.noteEditor().NavigationCommands().WindowPoint(first)
	if !ok {
		t.Fatal("first line has no editor geometry")
	}
	_, secondY, ok := a.noteEditor().NavigationCommands().WindowPoint(second)
	if !ok {
		t.Fatal("second line has no editor geometry")
	}
	if secondY <= firstY {
		t.Fatalf("single source newline stayed on the first visual row: firstY=%.1f secondY=%.1f", firstY, secondY)
	}
}
