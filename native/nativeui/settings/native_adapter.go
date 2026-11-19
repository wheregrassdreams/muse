package settings

import (
	"errors"

	"github.com/egoist/mygo"
	applicationagent "muse/application/agent"
	applicationconfig "muse/application/config"
	"muse/config"
	"muse/locales"
	appearance "muse/theme"
)

// AssistantSettingsPort is the narrow feature API used by settings to edit
// provider configuration and request model discovery.
type AssistantSettingsPort interface {
	Settings() config.AssistantSettings
	SetSettings(config.AssistantSettings)
	SaveSettings() error
	FetchModels()
	ModelCatalogSnapshot() applicationagent.ModelCatalogSnapshot
	SelectProvider(magpie bool) config.AssistantSettings
}

// NativePorts are capabilities supplied by the desktop composition root.
// Persisted preferences and editor settings are owned by NativeAdapter;
// toolkit-specific effects are kept behind these narrow callbacks.
type NativePorts struct {
	Configuration applicationconfig.Repository
	Assistant     AssistantSettingsPort
	Keymap        func() string
	ApplyEditor   func(Editor)
	ApplyTheme    func(ThemeMode)
	Notify        func(string)
	Invalidate    func()
	SettingsPath  string
}

// RuntimeSettings is the immutable appearance/editor configuration used by
// the shell and note editor while rendering.
type RuntimeSettings struct {
	Appearance Appearance
	Editor     Editor
	AIEnabled  bool
}

// NativeAdapter owns the persisted settings workflow and its MyGo effects.
// It has no reference to nativeui.App or feature-private UI state.
type NativeAdapter struct {
	ports         NativePorts
	preferences   config.Preferences
	editor        config.EditorSettings
	loaded        bool
	headerFade    bool
	fontSize      float64
	lineHeight    float64
	hasFontSize   bool
	hasLineHeight bool
}

func NewNativeAdapter(ports NativePorts) *NativeAdapter {
	return &NativeAdapter{
		ports: ports, preferences: config.DefaultPreferences(), editor: config.DefaultEditorSettings(),
		headerFade: true, fontSize: 16, lineHeight: 1.7,
	}
}

func (a *NativeAdapter) Load() error {
	if a == nil {
		return nil
	}
	service := applicationconfig.NewService(a.ports.Configuration)
	preferences, preferencesErr := service.LoadPreferences()
	editor, editorErr := service.LoadEditorSettings()
	a.preferences, a.editor, a.loaded = preferences, editor, true
	if preferences.LocaleConfigured {
		mygo.App.SetLocale(preferences.Locale)
	}
	if a.ports.ApplyTheme != nil {
		a.ports.ApplyTheme(themeFromConfig(preferences.Theme))
	}
	if a.ports.ApplyEditor != nil {
		a.ports.ApplyEditor(editorFromConfig(editor))
	}
	return errors.Join(preferencesErr, editorErr)
}

func (a *NativeAdapter) Runtime() RuntimeSettings {
	if a == nil {
		return RuntimeSettings{}
	}
	if !a.loaded {
		_ = a.Load()
	}
	keymap := a.editor.Keymap
	if a.ports.Keymap != nil {
		if current := a.ports.Keymap(); current != "" {
			keymap = current
		}
	}
	editorSettings := editorFromConfig(a.editor, keymap)
	return RuntimeSettings{
		Appearance: Appearance{
			Theme: themeFromConfig(a.preferences.Theme), Accents: accents(),
			AccentID:   appearance.Normalize(a.preferences.Accent),
			HeaderFade: a.headerFade, FontSize: a.fontSize, LineHeight: a.lineHeight,
			SmartTypography: boolValue(a.editor.SmartTypography),
			Locale:          a.preferences.Locale, LocaleConfigured: a.preferences.LocaleConfigured,
			SmoothPageJumps: a.preferences.SmoothPageJumps,
		},
		Editor: editorSettings, AIEnabled: a.preferences.AIEnabled,
	}
}

func (a *NativeAdapter) ViewModel() ViewModel {
	runtime := a.Runtime()
	assistant := Assistant{}
	if a.ports.Assistant != nil {
		settings := a.ports.Assistant.Settings()
		catalog := a.ports.Assistant.ModelCatalogSnapshot()
		models := make([]string, 0, len(catalog.Models))
		for _, model := range catalog.Models {
			models = append(models, model.ID)
		}
		assistant = Assistant{
			Enabled: runtime.AIEnabled, Provider: settings.Provider, BaseURL: settings.BaseURL,
			APIKey: settings.APIKey, Model: settings.Model, SystemPrompt: settings.System,
			Capabilities:    Capabilities{Browse: settings.Tools.Browse, Edit: settings.Tools.Edit, Manage: settings.Tools.Manage},
			AvailableModels: models, ModelsLoading: catalog.Loading, ModelsError: catalog.Error,
			SettingsPath: a.ports.SettingsPath, CapabilitiesDetail: locales.AiCapabilitiesDetail(),
		}
	}
	return ViewModel{Appearance: runtime.Appearance, Editor: runtime.Editor, Assistant: assistant}
}

func (a *NativeAdapter) Dispatch(action Action) {
	if a == nil {
		return
	}
	if !a.loaded {
		_ = a.Load()
	}
	switch action.Kind {
	case ThemeChanged:
		a.preferences.Theme = themeToConfig(action.Theme)
		a.persistPreferences()
		if a.ports.ApplyTheme != nil {
			a.ports.ApplyTheme(action.Theme)
		}
	case AccentChanged:
		a.preferences.Accent = appearance.Normalize(action.AccentID)
		a.persistPreferences()
		if a.ports.Invalidate != nil {
			a.ports.Invalidate()
		}
	case LocaleChanged:
		a.preferences.Locale, a.preferences.LocaleConfigured = action.Locale, true
		a.persistPreferences()
		mygo.App.SetLocale(action.Locale)
	case SmoothPageJumpsChanged:
		a.preferences.SmoothPageJumps = action.Enabled
		a.persistPreferences()
	case HeaderFadeChanged:
		// Header fade is a window presentation preference. It is persisted in
		// the settings feature and read by the shell at render time.
		a.headerFade = action.Enabled
		if a.ports.Invalidate != nil {
			a.ports.Invalidate()
		}
	case SmartTypographyChanged:
		a.editor.SmartTypography = boolPtr(action.Enabled)
		a.persistEditor()
	case KeymapChanged:
		a.editor.Keymap = action.Text
		a.persistEditor()
	case HideMouseCursorChanged:
		a.editor.HideMouseCursor = boolPtr(action.Enabled)
		a.persistEditor()
	case CursorFadeChanged:
		a.editor.CursorFade = boolPtr(action.Enabled)
		a.persistEditor()
	case SmoothCursorChanged:
		a.editor.SmoothCursor = boolPtr(action.Enabled)
		a.persistEditor()
	case HeadingMarkerRevealChanged:
		a.editor.HeadingMarkerReveal = boolPtr(action.Enabled)
		a.persistEditor()
	case VimVirtualImageRowsChanged:
		a.editor.VimVirtualImageRows = boolPtr(action.Enabled)
		a.persistEditor()
	case VimYankFlashChanged:
		a.editor.VimYankFlash = boolPtr(action.Enabled)
		a.persistEditor()
	case SearchMatchPulseChanged:
		a.editor.SearchMatchPulse = boolPtr(action.Enabled)
		a.persistEditor()
	case FontSizeChanged:
		a.fontSize, a.hasFontSize = action.Number, true
		a.applyEditor()
	case LineHeightChanged:
		a.lineHeight, a.hasLineHeight = action.Number, true
		a.applyEditor()
	case AssistantEnabledChanged:
		a.preferences.AIEnabled = action.Enabled
		a.persistPreferences()
	case ProviderChanged, BaseURLChanged, APIKeyChanged, ModelChanged, SystemPromptChanged, CapabilityChanged:
		a.dispatchAssistant(action)
	case FetchModels:
		if a.ports.Assistant != nil {
			a.ports.Assistant.FetchModels()
		}
	case SaveAssistant:
		if a.ports.Assistant == nil {
			a.notify(locales.CommonSaveFailed("assistant settings are unavailable"))
			return
		}
		if err := a.ports.Assistant.SaveSettings(); err != nil {
			a.notify(locales.CommonSaveFailed(err.Error()))
		} else {
			a.notify(locales.AiModelSettingsSaved())
		}
	}
	if a.ports.Invalidate != nil {
		a.ports.Invalidate()
	}
}

func (a *NativeAdapter) dispatchAssistant(action Action) {
	if a.ports.Assistant == nil {
		return
	}
	settings := a.ports.Assistant.Settings()
	switch action.Kind {
	case ProviderChanged:
		updated := a.ports.Assistant.SelectProvider(action.Text == config.AssistantProviderMagpie)
		if updated != settings {
			settings = updated
		}
	case BaseURLChanged:
		settings.BaseURL = action.Text
	case APIKeyChanged:
		settings.APIKey = action.Text
	case ModelChanged:
		settings.Model = action.Text
	case SystemPromptChanged:
		settings.System = action.Text
	case CapabilityChanged:
		switch action.Capability {
		case BrowseCapability:
			settings.Tools.Browse = action.Enabled
		case EditCapability:
			settings.Tools.Edit = action.Enabled
		case ManageCapability:
			settings.Tools.Manage = action.Enabled
		}
	}
	a.ports.Assistant.SetSettings(settings)
	if action.Kind == ProviderChanged || action.Kind == CapabilityChanged {
		if err := a.ports.Assistant.SaveSettings(); err != nil {
			a.notify(locales.CommonSaveFailed(err.Error()))
		}
	}
}

func (a *NativeAdapter) persistPreferences() {
	if err := applicationconfig.NewService(a.ports.Configuration).SavePreferences(a.preferences); err != nil {
		a.notify(locales.CommonSettingSaveError(err.Error()))
	}
}

func (a *NativeAdapter) persistEditor() {
	if err := applicationconfig.NewService(a.ports.Configuration).SaveEditorSettings(a.editor); err != nil {
		a.notify(locales.CommonSettingSaveError(err.Error()))
	}
	if a.ports.ApplyEditor != nil {
		a.ports.ApplyEditor(editorFromConfig(a.editor, a.Runtime().Editor.Keymap))
	}
}

func (a *NativeAdapter) applyEditor() {
	if a.ports.ApplyEditor != nil {
		a.ports.ApplyEditor(a.Runtime().Editor)
	}
}

func (a *NativeAdapter) notify(message string) {
	if a.ports.Notify != nil && message != "" {
		a.ports.Notify(message)
	}
}

func accents() []Accent {
	items := make([]Accent, 0, len(appearance.Presets))
	for _, preset := range appearance.Presets {
		items = append(items, Accent{ID: preset.ID, Label: LocalizedAccentName(preset.ID), Color: preset.Color})
	}
	return items
}

func themeFromConfig(mode config.ThemeMode) ThemeMode {
	switch mode {
	case config.ThemeLight:
		return ThemeLight
	case config.ThemeDark:
		return ThemeDark
	default:
		return ThemeSystem
	}
}

func themeToConfig(mode ThemeMode) config.ThemeMode {
	switch mode {
	case ThemeLight:
		return config.ThemeLight
	case ThemeDark:
		return config.ThemeDark
	default:
		return config.ThemeSystem
	}
}

func editorFromConfig(settings config.EditorSettings, keymap ...string) Editor {
	value := settings.Keymap
	if len(keymap) > 0 && keymap[0] != "" {
		value = keymap[0]
	}
	return Editor{
		Keymap:              value,
		HeadingMarkerReveal: boolValue(settings.HeadingMarkerReveal),
		VimVirtualImageRows: boolValue(settings.VimVirtualImageRows),
		CursorFade:          boolValue(settings.CursorFade), SmoothCursor: boolValue(settings.SmoothCursor),
		HideMouseCursor: boolValue(settings.HideMouseCursor), SearchMatchPulse: boolValue(settings.SearchMatchPulse),
		VimYankFlash: boolValue(settings.VimYankFlash),
	}
}

func boolValue(value *bool) bool { return value != nil && *value }
func boolPtr(value bool) *bool   { return &value }
