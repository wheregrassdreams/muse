// Package config owns persisted user settings and their file format.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ThemeMode is the user's persisted theme selection.
type ThemeMode string

const (
	ThemeSystem ThemeMode = "system"
	ThemeLight  ThemeMode = "light"
	ThemeDark   ThemeMode = "dark"
)

// Preferences contains application-wide settings that are persisted together.
// Runtime state and UI callbacks belong to the application/presentation layers.
type Preferences struct {
	Theme            ThemeMode `json:"theme,omitempty"`
	Accent           string    `json:"accent,omitempty"`
	AIEnabled        bool      `json:"aiEnabled,omitempty"`
	SmoothPageJumps  bool      `json:"smoothPageJumps,omitempty"`
	Locale           string    `json:"locale,omitempty"`
	LocaleConfigured bool      `json:"localeConfigured,omitempty"`
}

// EditorSettings contains persisted editor options. Pointer booleans preserve
// the distinction between an omitted legacy setting and an explicit false.
type EditorSettings struct {
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

// AssistantTools controls which tool groups the assistant may invoke. These
// capabilities are persisted separately from transient chat/session state.
type AssistantTools struct {
	Browse bool `json:"browse"`
	Edit   bool `json:"edit"`
	Manage bool `json:"manage"`
}

// AssistantSettings is the user-owned provider and model configuration.
// Conversation messages, approvals, streaming and view state are runtime data.
type AssistantSettings struct {
	Provider string         `json:"provider,omitempty"`
	BaseURL  string         `json:"baseUrl"`
	APIKey   string         `json:"apiKey"`
	Model    string         `json:"model"`
	System   string         `json:"system,omitempty"`
	Tools    AssistantTools `json:"tools"`
}

const (
	AssistantProviderCustom = ""
	AssistantProviderMagpie = "magpie"
	AssistantMagpieBaseURL  = "http://127.0.0.1:3425/v1"
)

func DefaultAssistantTools() AssistantTools {
	return AssistantTools{Browse: true, Edit: true, Manage: true}
}

func (s AssistantSettings) UsesMagpie() bool { return s.Provider == AssistantProviderMagpie }

func AssistantSettingsPath() string {
	return filepath.Join(filepath.Dir(PreferencesPath()), "ai.json")
}

func LoadAssistantSettings() (AssistantSettings, error) {
	var settings AssistantSettings
	data, err := os.ReadFile(AssistantSettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			settings.Tools = DefaultAssistantTools()
			return settings, nil
		}
		return settings, err
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return AssistantSettings{Tools: DefaultAssistantTools()}, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		settings.Tools = DefaultAssistantTools()
		return settings, nil
	}
	if _, exists := raw["tools"]; !exists {
		settings.Tools = DefaultAssistantTools()
	}
	return settings, nil
}

func SaveAssistantSettings(settings AssistantSettings) error {
	return writeJSON(AssistantSettingsPath(), settings)
}

// PreferencesPath resolves the application settings path using the standard
// per-user configuration directory, with MUSE_CONFIG_DIR for development.
func PreferencesPath() string {
	if dir := os.Getenv("MUSE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "settings.json")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "Muse", "settings.json")
}

// EditorSettingsPath places editor options beside the application settings.
func EditorSettingsPath() string {
	return filepath.Join(filepath.Dir(PreferencesPath()), "editor.json")
}

func LoadPreferences() (Preferences, error) {
	p := Preferences{Theme: ThemeSystem}
	data, err := os.ReadFile(PreferencesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return p, nil
		}
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return Preferences{Theme: ThemeSystem}, err
	}
	if p.Theme != ThemeLight && p.Theme != ThemeDark && p.Theme != ThemeSystem {
		p.Theme = ThemeSystem
	}
	return p, nil
}

func SavePreferences(p Preferences) error {
	if p.Theme != ThemeLight && p.Theme != ThemeDark && p.Theme != ThemeSystem {
		p.Theme = ThemeSystem
	}
	return writeJSON(PreferencesPath(), p)
}

func LoadEditorSettings() (EditorSettings, error) {
	var settings EditorSettings
	data, err := os.ReadFile(EditorSettingsPath())
	if err != nil && !os.IsNotExist(err) {
		return settings, err
	}
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return EditorSettings{}, err
		}
	}
	if settings.Keymap != "vim" && settings.Keymap != "helix" {
		settings.Keymap = "default"
	}
	return settings, nil
}

func SaveEditorSettings(settings EditorSettings) error {
	if settings.Keymap != "vim" && settings.Keymap != "helix" {
		settings.Keymap = "default"
	}
	return writeJSON(EditorSettingsPath(), settings)
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
