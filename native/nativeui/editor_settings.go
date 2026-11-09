package nativeui

import (
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"os"
	"path/filepath"
)

type editorSettings struct {
	Keymap string `json:"keymap"`
}

func editorConfigPath() string { return filepath.Join(filepath.Dir(aiConfigPath()), "editor.json") }
func loadEditorSettings() editorSettings {
	var s editorSettings
	b, err := os.ReadFile(editorConfigPath())
	if err == nil {
		_ = json.Unmarshal(b, &s)
	}
	if s.Keymap != "vim" && s.Keymap != "helix" {
		s.Keymap = "default"
	}
	return s
}
func (a *App) setEditorKeymap(mode string) {
	a.ed.SetKeymap(mode)
	s := editorSettings{Keymap: a.ed.Keymap()}
	b, err := json.MarshalIndent(s, "", "  ")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(editorConfigPath()), 0700)
	}
	if err == nil {
		err = os.WriteFile(editorConfigPath(), b, 0600)
	}
	if err != nil {
		a.notice("保存按键模式失败：" + err.Error())
	}
	a.invalidate()
}
func (a *App) keymapSettings(c *ui.Context) {
	settingsRow(c, "按键模式", "Helix / Vim 基础键位；Esc 返回普通模式，i 开始输入。", func() {
		for _, v := range []struct{ name, mode string }{{"默认", "default"}, {"Helix", "helix"}, {"Vim", "vim"}} {
			v := v
			b := ui.ButtonBase(c).Height(28).Padding(0, 8).Radius(6).Label("按键模式：" + v.name)
			if a.ed.Keymap() == v.mode {
				b.Background(c.Theme().SurfaceHover)
			}
			b.OnClick(func() { a.setEditorKeymap(v.mode) }).Children(func() { ui.Text(c, v.name) })
		}
	})
}
