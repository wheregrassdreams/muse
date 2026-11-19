package settings

import (
	"testing"

	applicationconfig "muse/application/config"
	"muse/config"
)

type memorySettingsRepository struct {
	preferences config.Preferences
	editor      config.EditorSettings
}

func (r *memorySettingsRepository) LoadPreferences() (config.Preferences, error) {
	return r.preferences, nil
}
func (r *memorySettingsRepository) SavePreferences(value config.Preferences) error {
	r.preferences = value
	return nil
}
func (r *memorySettingsRepository) LoadEditorSettings() (config.EditorSettings, error) {
	return r.editor, nil
}
func (r *memorySettingsRepository) SaveEditorSettings(value config.EditorSettings) error {
	r.editor = value
	return nil
}
func (*memorySettingsRepository) LoadAssistantSettings() (config.AssistantSettings, error) {
	return config.DefaultAssistantSettings(), nil
}
func (*memorySettingsRepository) SaveAssistantSettings(config.AssistantSettings) error { return nil }
func (*memorySettingsRepository) LoadDocumentPropertyPreferences(string) (config.DocumentPropertyPreferences, error) {
	return config.DocumentPropertyPreferences{}, nil
}
func (*memorySettingsRepository) SaveDocumentPropertyPreferences(string, config.DocumentPropertyPreferences) error {
	return nil
}
func (*memorySettingsRepository) AssistantSettingsLocation() string { return "" }

var _ applicationconfig.Repository = (*memorySettingsRepository)(nil)

func TestNativeHostOwnsPreferencePersistenceAndRuntimeProjection(t *testing.T) {
	repository := &memorySettingsRepository{preferences: config.DefaultPreferences(), editor: config.DefaultEditorSettings()}
	var appliedTheme ThemeMode
	host := NewNativeHost(NativePorts{
		Configuration: repository,
		ApplyTheme:    func(theme ThemeMode) { appliedTheme = theme },
	})
	if err := host.Load(); err != nil {
		t.Fatal(err)
	}
	host.Dispatch(Action{Kind: ThemeChanged, Theme: ThemeDark})
	host.Dispatch(Action{Kind: AccentChanged, AccentID: "blue"})
	host.Dispatch(Action{Kind: HideMouseCursorChanged, Enabled: true})

	runtime := host.Runtime()
	if runtime.Appearance.Theme != ThemeDark || runtime.Appearance.AccentID != "blue" {
		t.Fatalf("appearance runtime=%+v", runtime.Appearance)
	}
	if !runtime.Editor.HideMouseCursor {
		t.Fatalf("editor runtime=%+v", runtime.Editor)
	}
	if appliedTheme != ThemeDark || repository.preferences.Theme != config.ThemeDark || repository.preferences.Accent != "blue" {
		t.Fatalf("theme callback=%v preferences=%+v", appliedTheme, repository.preferences)
	}
	if repository.editor.HideMouseCursor == nil || !*repository.editor.HideMouseCursor {
		t.Fatalf("persisted editor settings=%+v", repository.editor)
	}
}
