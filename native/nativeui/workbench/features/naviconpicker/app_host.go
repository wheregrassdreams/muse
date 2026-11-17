// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

func (p navIconPreferences) Get(itemPath string) string {
	if p.app == nil || p.app.navIconStore == nil {
		return ""
	}
	return p.app.navIconStore.Get(p.app.root, itemPath)
}

func (p navIconPreferences) Set(itemPath, iconID string) error {
	if p.app == nil || p.app.navIconStore == nil {
		return errors.New("navigation icon storage is not configured")
	}
	return p.app.navIconStore.Set(p.app.root, itemPath, iconID)
}

func (p navIconPreferences) Delete(itemPath string) error {
	if p.app == nil || p.app.navIconStore == nil {
		return errors.New("navigation icon storage is not configured")
	}
	return p.app.navIconStore.Delete(p.app.root, itemPath)
}

func (a *App) handleNavIconNotice(notice naviconpicker.Notice) {
	if a == nil {
		return
	}
	switch notice.Kind {
	case naviconpicker.PreferencesUnavailable:
		a.notice(locales.CommonOpenIconSettingsError("navigation icon storage is not configured"))
	case naviconpicker.PreferenceSaveFailed:
		if notice.Err != nil {
			a.notice(locales.CommonSaveIconError(notice.Err.Error()))
		}
	case naviconpicker.PreferenceUpdated:
		a.notice(locales.NavigationIconUpdated())
	}
}

func (a *App) openNavIconPicker(path string) {
	if a == nil || a.navIconPicker == nil {
		return
	}
	if a.navIconStore == nil {
		a.handleNavIconNotice(naviconpicker.Notice{Kind: naviconpicker.PreferencesUnavailable})
		return
	}
	a.navIconPicker.Open(path)
}

func (a *App) navIconPickerPopover(c *ui.Context, anchor ui.Element) {
	if a != nil && a.navIconPicker != nil {
		a.navIconPicker.RenderPopover(c, anchor)
	}
}

type navIconPreferences struct{ app *App }
