package nativeui

import (
	applicationconfig "muse/application/config"
	"muse/config"
	"muse/locales"
)

type editorSettings = config.EditorSettings

func (a *App) loadEditorSettings() editorSettings {
	s, _ := applicationconfig.NewService(a.configRepository).LoadEditorSettings()
	return s
}

func (a *App) applyEditorSettings(s editorSettings) {
	if s.SmartTypography != nil {
		a.smartTypography = *s.SmartTypography
	}
	if s.HeadingMarkerReveal != nil {
		a.headingMarkerReveal = *s.HeadingMarkerReveal
	}
	if s.VimVirtualImageRows != nil {
		a.vimVirtualImageRows = *s.VimVirtualImageRows
	}
	if s.CursorFade != nil {
		a.cursorFade = *s.CursorFade
	}
	if s.SmoothCursor != nil {
		a.smoothCursor = *s.SmoothCursor
	}
	if s.HideMouseCursor != nil {
		a.hideMouseCursor = *s.HideMouseCursor
	}
	if s.SearchMatchPulse != nil {
		a.searchMatchPulse = *s.SearchMatchPulse
	}
	if s.VimYankFlash != nil {
		a.vimYankFlash = *s.VimYankFlash
	}
	if a.noteEditor() != nil {
		a.noteEditor().ModalCommands().SetKeymap(s.Keymap)
		a.configureActiveNote(0)
	}
}
func (a *App) setEditorKeymap(mode string) {
	a.noteEditor().ModalCommands().SetKeymap(mode)
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
		Keymap:              a.noteEditor().ModalCommands().Keymap(),
		SmartTypography:     &a.smartTypography,
		HeadingMarkerReveal: &a.headingMarkerReveal,
		VimVirtualImageRows: &a.vimVirtualImageRows,
		CursorFade:          &a.cursorFade,
		SmoothCursor:        &a.smoothCursor,
		HideMouseCursor:     &a.hideMouseCursor,
		SearchMatchPulse:    &a.searchMatchPulse,
		VimYankFlash:        &a.vimYankFlash,
	}
	return applicationconfig.NewService(a.configRepository).SaveEditorSettings(s)
}
