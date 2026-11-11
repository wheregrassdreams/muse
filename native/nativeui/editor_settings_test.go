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

func TestSmartTypographyDefaultsAndReload(t *testing.T) {
	for _, tc := range []struct {
		name, config string
		want         bool
	}{
		{name: "missing file", want: true},
		{name: "legacy keymap", config: `{"keymap":"vim"}`, want: true},
		{name: "enabled", config: `{"keymap":"vim","smartTypography":true}`, want: true},
		{name: "disabled", config: `{"keymap":"vim","smartTypography":false}`, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
			if tc.config != "" {
				if err := os.WriteFile(editorConfigPath(), []byte(tc.config), 0600); err != nil {
					t.Fatal(err)
				}
			}
			a := New()
			if a.smartTypography != tc.want || a.ed.Options().DisableSmartTypography != !tc.want {
				t.Fatalf("smartTypography = %v, editor disabled = %v; want enabled %v", a.smartTypography, a.ed.Options().DisableSmartTypography, tc.want)
			}
		})
	}
}

func TestSmartTypographyAndKeymapSaveTogether(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.setEditorKeymap("vim")
	a.setSmartTypography(false)
	b := New()
	if b.smartTypography || b.ed.Keymap() != "vim" {
		t.Fatalf("typography save restored enabled=%v keymap=%q", b.smartTypography, b.ed.Keymap())
	}
	b.setEditorKeymap("helix")
	c := New()
	if c.smartTypography || !c.ed.Options().DisableSmartTypography || c.ed.Keymap() != "helix" {
		t.Fatalf("keymap save restored enabled=%v disabled=%v keymap=%q", c.smartTypography, c.ed.Options().DisableSmartTypography, c.ed.Keymap())
	}
	c.setSmartTypography(true)
	d := New()
	if !d.smartTypography || d.ed.Options().DisableSmartTypography || d.ed.Keymap() != "helix" {
		t.Fatalf("re-enable restored enabled=%v disabled=%v keymap=%q", d.smartTypography, d.ed.Options().DisableSmartTypography, d.ed.Keymap())
	}
}

func TestSmartTypographySettingsToggle(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.openSettings()
	a.settingsTab = 1
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !tt.HasText("智能排版") {
		t.Fatal("missing smart typography setting")
	}
	for _, enabled := range []bool{false, true} {
		if err := tt.Click("智能排版开关"); err != nil {
			t.Fatal(err)
		}
		// The click updates the active editor without waiting for another frame.
		if a.smartTypography != enabled || a.editorOptions().DisableSmartTypography != !enabled || a.ed.Options().DisableSmartTypography != !enabled {
			t.Fatalf("toggle: enabled=%v options disabled=%v editor disabled=%v; want enabled %v", a.smartTypography, a.editorOptions().DisableSmartTypography, a.ed.Options().DisableSmartTypography, enabled)
		}
		saved := loadEditorSettings()
		if saved.SmartTypography == nil || *saved.SmartTypography != enabled {
			t.Fatalf("saved smartTypography = %v, want %v", saved.SmartTypography, enabled)
		}
		tt.Frame()
	}
}
