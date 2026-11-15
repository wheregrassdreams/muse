package nativeui

import (
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"muse/locales"
	"os"
	"path/filepath"
)

type editorSettings struct {
	Keymap              string `json:"keymap"`
	SmartTypography     *bool  `json:"smartTypography,omitempty"`
	HeadingMarkerReveal *bool  `json:"headingMarkerReveal,omitempty"`
	VimVirtualImageRows *bool  `json:"vimVirtualImageRows,omitempty"`
	CursorFade          *bool  `json:"cursorFade,omitempty"`
	SmoothCursor        *bool  `json:"smoothCursor,omitempty"`
	HideMouseCursor     *bool  `json:"hideMouseCursor,omitempty"`
	SearchMatchPulse    *bool  `json:"searchMatchPulse,omitempty"`
	VimYankFlash        *bool  `json:"vimYankFlash,omitempty"`
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
	if err := a.saveEditorSettings(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) setSmartTypography(enabled bool) {
	a.smartTypography = enabled
	opts := a.ed.Options()
	opts.DisableSmartTypography = !enabled
	a.ed.SetOptions(opts)
	if err := a.saveEditorSettings(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) setHeadingMarkerReveal(enabled bool) {
	a.headingMarkerReveal = enabled
	a.ed.SetOptions(a.editorOptions())
	if err := a.saveEditorSettings(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) setVimVirtualImageRows(enabled bool) {
	a.vimVirtualImageRows = enabled
	a.ed.SetOptions(a.editorOptions())
	if err := a.saveEditorSettings(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) setCursorFade(enabled bool) {
	a.cursorFade = enabled
	a.ed.SetOptions(a.editorOptions())
	if err := a.saveEditorSettings(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) setSmoothCursor(enabled bool) {
	a.smoothCursor = enabled
	a.ed.SetOptions(a.editorOptions())
	if err := a.saveEditorSettings(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) setHideMouseCursor(enabled bool) {
	a.hideMouseCursor = enabled
	a.mouseCursorHidden = false
	if err := a.saveEditorSettings(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) setSearchMatchPulse(enabled bool) {
	a.searchMatchPulse = enabled
	a.ed.SetOptions(a.editorOptions())
	if err := a.saveEditorSettings(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) setVimYankFlash(enabled bool) {
	a.vimYankFlash = enabled
	a.ed.SetOptions(a.editorOptions())
	if err := a.saveEditorSettings(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) saveEditorSettings() error {
	s := editorSettings{
		Keymap:              a.ed.Keymap(),
		SmartTypography:     &a.smartTypography,
		HeadingMarkerReveal: &a.headingMarkerReveal,
		VimVirtualImageRows: &a.vimVirtualImageRows,
		CursorFade:          &a.cursorFade,
		SmoothCursor:        &a.smoothCursor,
		HideMouseCursor:     &a.hideMouseCursor,
		SearchMatchPulse:    &a.searchMatchPulse,
		VimYankFlash:        &a.vimYankFlash,
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(editorConfigPath()), 0700)
	}
	if err == nil {
		err = os.WriteFile(editorConfigPath(), b, 0600)
	}
	return err
}
func (a *App) keymapSettings(c *ui.Context) {
	settingsRow(c, locales.SettingsKeymap(), locales.SettingsKeymapDetail(), func() {
		for _, v := range []struct{ name, mode string }{{locales.KeymapDefault(), "default"}, {locales.KeymapHelix(), "helix"}, {locales.KeymapVim(), "vim"}} {
			v := v
			b := ui.ButtonBase(c).Height(28).Padding(0, 8).Radius(6).Label(locales.KeymapNamed(v.name))
			if a.ed.Keymap() == v.mode {
				b.Background(c.Theme().SurfaceHover)
			}
			b.OnClick(func() { a.setEditorKeymap(v.mode) }).Children(func() { ui.Text(c, v.name) })
		}
	})
}
