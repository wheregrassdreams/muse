// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

func (a *App) settingsDialog(c *ui.Context) {
	a.settingsPanel.RenderDialog(c, a.settingsViewModel(), a.dispatchSettingsAction)
}

func (a *App) settingsViewModel() settingsui.ViewModel {
	mode := settingsui.ThemeSystem
	switch a.themeMode {
	case themeLight:
		mode = settingsui.ThemeLight
	case themeDark:
		mode = settingsui.ThemeDark
	}
	accents := make([]settingsui.Accent, 0, len(appearance.Presets))
	for _, preset := range appearance.Presets {
		accents = append(accents, settingsui.Accent{ID: preset.ID, Label: settingsui.LocalizedAccentName(preset.ID), Color: preset.Color})
	}
	modelCatalog := a.assistant.ModelCatalogSnapshot()
	assistantModels := make([]string, 0, len(modelCatalog.Models))
	for _, model := range modelCatalog.Models {
		assistantModels = append(assistantModels, model.ID)
	}
	return settingsui.ViewModel{
		Appearance: settingsui.Appearance{
			Theme: mode, Accents: accents, AccentID: a.accentID,
			Locale: a.localeSetting, LocaleConfigured: a.localeConfigured,
			HeaderFade: a.headerFade, SmoothPageJumps: a.smoothPageJumps,
			SmartTypography: a.smartTypography, FontSize: a.fontSize, LineHeight: a.lineHeight,
		},
		Editor: settingsui.Editor{
			Keymap: a.noteEditor().ModalCommands().Keymap(), HideMouseCursor: a.hideMouseCursor,
			CursorFade: a.cursorFade, SmoothCursor: a.smoothCursor,
			HeadingMarkerReveal: a.headingMarkerReveal, VimVirtualImageRows: a.vimVirtualImageRows,
			VimYankFlash: a.vimYankFlash, SearchMatchPulse: a.searchMatchPulse,
		},
		Assistant: settingsui.Assistant{
			Enabled: a.aiEnabled, Provider: a.ai.Provider, BaseURL: a.ai.BaseURL, APIKey: a.ai.APIKey,
			Model: a.ai.Model, SystemPrompt: a.ai.System, AvailableModels: assistantModels,
			ModelsLoading: modelCatalog.Loading, ModelsError: modelCatalog.Error,
			Capabilities: settingsui.Capabilities{Browse: a.ai.Tools.Browse, Edit: a.ai.Tools.Edit, Manage: a.ai.Tools.Manage},
			SettingsPath: a.assistantSettingsLocation, CapabilitiesDetail: locales.AiCapabilitiesDetail(),
		},
	}
}

func (a *App) dispatchSettingsAction(action settingsui.Action) {
	a.settingsPanel.Apply(action)
	switch action.Kind {
	case settingsui.SelectPage:
	case settingsui.ThemeChanged:
		mode := themeSystem
		switch action.Theme {
		case settingsui.ThemeLight:
			mode = themeLight
		case settingsui.ThemeDark:
			mode = themeDark
		}
		a.setThemeMode(mode)
	case settingsui.AccentChanged:
		a.setAccent(action.AccentID)
	case settingsui.LocaleChanged:
		a.setLocale(action.Locale)
	case settingsui.HeaderFadeChanged:
		a.headerFade = action.Enabled
		if err := a.saveNativePreferences(); err != nil {
			a.notice(locales.CommonSettingSaveError(err.Error()))
		}
		a.invalidate()
	case settingsui.SmoothPageJumpsChanged:
		a.setSmoothPageJumps(action.Enabled)
	case settingsui.KeymapChanged:
		a.setEditorKeymap(action.Text)
	case settingsui.SmartTypographyChanged:
		a.setSmartTypography(action.Enabled)
	case settingsui.FontSizeChanged:
		a.fontSize = action.Number
		a.configureActiveNote(0)
		a.invalidate()
	case settingsui.LineHeightChanged:
		a.lineHeight = action.Number
		a.configureActiveNote(0)
		a.invalidate()
	case settingsui.HideMouseCursorChanged:
		a.setHideMouseCursor(action.Enabled)
	case settingsui.CursorFadeChanged:
		a.setCursorFade(action.Enabled)
	case settingsui.SmoothCursorChanged:
		a.setSmoothCursor(action.Enabled)
	case settingsui.HeadingMarkerRevealChanged:
		a.setHeadingMarkerReveal(action.Enabled)
	case settingsui.VimVirtualImageRowsChanged:
		a.setVimVirtualImageRows(action.Enabled)
	case settingsui.VimYankFlashChanged:
		a.setVimYankFlash(action.Enabled)
	case settingsui.SearchMatchPulseChanged:
		a.setSearchMatchPulse(action.Enabled)
	case settingsui.AssistantEnabledChanged:
		a.setAIEnabled(action.Enabled)
	case settingsui.ProviderChanged:
		a.setAIService(action.Text == config.AssistantProviderMagpie)
	case settingsui.BaseURLChanged:
		a.ai.BaseURL = action.Text
	case settingsui.APIKeyChanged:
		a.ai.APIKey = action.Text
	case settingsui.ModelChanged:
		a.ai.Model = action.Text
	case settingsui.SystemPromptChanged:
		a.ai.System = action.Text
	case settingsui.CapabilityChanged:
		switch action.Capability {
		case settingsui.BrowseCapability:
			a.ai.Tools.Browse = action.Enabled
		case settingsui.EditCapability:
			a.ai.Tools.Edit = action.Enabled
		case settingsui.ManageCapability:
			a.ai.Tools.Manage = action.Enabled
		}
		a.saveAITools()
	case settingsui.FetchModels:
		a.fetchAIModels()
	case settingsui.SaveAssistant:
		if err := a.saveAssistantSettings(); err != nil {
			a.notice(locales.CommonSaveFailed(err.Error()))
		} else {
			a.notice(locales.AiModelSettingsSaved())
		}
	case settingsui.Close:
	}
}

func (a *App) openSettings() {
	a.settingsPanel.OpenAt(settingsui.AppearancePage)
}
