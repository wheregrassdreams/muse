// Package config defines application-owned persistence capabilities for
// user-authored configuration values.
package config

import "muse/config"

// Repository loads and saves typed configuration values. Implementations own
// storage paths, file permissions, and serialization I/O.
type Repository interface {
	LoadPreferences() (config.Preferences, error)
	SavePreferences(config.Preferences) error
	LoadEditorSettings() (config.EditorSettings, error)
	SaveEditorSettings(config.EditorSettings) error
	LoadAssistantSettings() (config.AssistantSettings, error)
	SaveAssistantSettings(config.AssistantSettings) error
	LoadDocumentPropertyPreferences(notePath string) (config.DocumentPropertyPreferences, error)
	SaveDocumentPropertyPreferences(notePath string, preferences config.DocumentPropertyPreferences) error
	AssistantSettingsLocation() string
}
