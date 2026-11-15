package nativeui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// clickSource clicks the editor at the start of the first occurrence of
// needle in the document, as the user clicks the rendered text there.
func clickSource(t *testing.T, a *App, tt *ui.Tester, needle string) {
	t.Helper()
	off := strings.Index(a.source, needle)
	if off < 0 {
		t.Fatalf("%q not in the document %q", needle, a.source)
	}
	tt.Frame()
	x, y, ok := a.ed.WindowPoint(off)
	if !ok {
		t.Fatalf("no position for %q", needle)
	}
	tt.ClickAt(x+2, y)
	tt.Frame()
}

func TestClickingTextDoesNotDirty(t *testing.T) {
	a := New()
	a.loadDoc("# 标题\n\n正文\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n")
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	clickSource(t, a, tt, "正文")
	clickSource(t, a, tt, "标题")
	clickSource(t, a, tt, "1 |")
	if a.dirty {
		t.Fatal("clicking the document marked it dirty")
	}
}

func TestTypingDirties(t *testing.T) {
	a := New()
	a.loadDoc("# 标题\n\n正文\n")
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	clickSource(t, a, tt, "正文")
	tt.Key(0, ui.KeyEnd)
	tt.Type("！")
	tt.Frame()
	if !a.dirty {
		t.Fatal("typing did not mark the document dirty")
	}
	if a.source != "# 标题\n\n正文！\n" {
		t.Fatalf("source = %q", a.source)
	}
}
