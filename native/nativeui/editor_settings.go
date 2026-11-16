package nativeui

import (
	"muse/config"
	"muse/locales"
)

type editorSettings = config.EditorSettings

func editorConfigPath() string { return config.EditorSettingsPath() }
func loadEditorSettings() editorSettings {
	s, _ := config.LoadEditorSettings()
	return s
}
func (a *App) setEditorKeymap(mode string) {
	a.activeNote.ModalCommands().SetKeymap(mode)
	if err := a.saveEditorSettings(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) setSmartTypography(enabled bool) {
	a.smartTypography = enabled
	a.configureActiveNote(0)
	if err := a.saveEditorSettings(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) setHeadingMarkerReveal(enabled bool) {
	a.headingMarkerReveal = enabled
	a.configureActiveNote(0)
	if err := a.saveEditorSettings(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) setVimVirtualImageRows(enabled bool) {
	a.vimVirtualImageRows = enabled
	a.configureActiveNote(0)
	if err := a.saveEditorSettings(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) setCursorFade(enabled bool) {
	a.cursorFade = enabled
	a.configureActiveNote(0)
	if err := a.saveEditorSettings(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) setSmoothCursor(enabled bool) {
	a.smoothCursor = enabled
	a.configureActiveNote(0)
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
	a.configureActiveNote(0)
	if err := a.saveEditorSettings(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) setVimYankFlash(enabled bool) {
	a.vimYankFlash = enabled
	a.configureActiveNote(0)
	if err := a.saveEditorSettings(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.invalidate()
}

func (a *App) saveEditorSettings() error {
	s := editorSettings{
		Keymap:              a.activeNote.ModalCommands().Keymap(),
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
