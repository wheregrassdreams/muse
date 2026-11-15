package nativeui

import (
	"muse/config"
	"muse/locales"

	"github.com/egoist/mygo/ui"
)

type editorSettings = config.EditorSettings

func editorConfigPath() string { return config.EditorSettingsPath() }
func loadEditorSettings() editorSettings {
	s, _ := config.LoadEditorSettings()
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
	return config.SaveEditorSettings(s)
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
