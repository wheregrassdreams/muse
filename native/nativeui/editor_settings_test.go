package nativeui

import (
	"github.com/egoist/mygo/ui"
	"os"
	"testing"
)

func TestEditorKeymapPersists(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	if a.ed.Keymap() != "default" {
		t.Fatal(a.ed.Keymap())
	}
	a.setEditorKeymap("vim")
	b := New()
	if b.ed.Keymap() != "vim" {
		t.Fatalf("restored %q", b.ed.Keymap())
	}
	if err := os.WriteFile(editorConfigPath(), []byte(`{"keymap":"unknown"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := New().ed.Keymap(); got != "default" {
		t.Fatal(got)
	}
}

func TestEditorKeymapSettingsUI(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.openSettings()
	a.settingsTab = 1
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if err := tt.Click("按键模式：Vim"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.ed.Keymap() != "vim" {
		t.Fatal(a.ed.Keymap())
	}
	if err := tt.Click("按键模式：默认"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.ed.Keymap() != "default" {
		t.Fatal(a.ed.Keymap())
	}
}
