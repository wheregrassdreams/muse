// Package config owns persisted user settings and their file format.
package config

import (
	"encoding/json"
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
	WindowBlur       bool      `json:"windowBlur"`
	WindowOpacity    float64   `json:"windowOpacity"`
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

func DefaultPreferences() Preferences {
	return Preferences{Theme: ThemeSystem, WindowBlur: true, WindowOpacity: 0.7}
}

func DefaultEditorSettings() EditorSettings { return EditorSettings{Keymap: "default"} }

func DefaultAssistantSettings() AssistantSettings {
	return AssistantSettings{Tools: DefaultAssistantTools()}
}

func NormalizePreferences(p Preferences) Preferences {
	if p.Theme != ThemeLight && p.Theme != ThemeDark && p.Theme != ThemeSystem {
		p.Theme = ThemeSystem
	}
	if p.WindowOpacity < 0.2 || p.WindowOpacity > 1 {
		p.WindowOpacity = 0.7
	}
	return p
}

func NormalizeEditorSettings(settings EditorSettings) EditorSettings {
	if settings.Keymap != "vim" && settings.Keymap != "helix" {
		settings.Keymap = "default"
	}
	return settings
}

func DecodePreferences(data []byte) (Preferences, error) {
	p := DefaultPreferences()
	if err := json.Unmarshal(data, &p); err != nil {
		return DefaultPreferences(), err
	}
	return NormalizePreferences(p), nil
}

func EncodePreferences(p Preferences) ([]byte, error) {
	return json.MarshalIndent(NormalizePreferences(p), "", "  ")
}

func DecodeEditorSettings(data []byte) (EditorSettings, error) {
	var settings EditorSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return EditorSettings{}, err
	}
	return NormalizeEditorSettings(settings), nil
}

func EncodeEditorSettings(settings EditorSettings) ([]byte, error) {
	return json.MarshalIndent(NormalizeEditorSettings(settings), "", "  ")
}

func DecodeAssistantSettings(data []byte) (AssistantSettings, error) {
	var settings AssistantSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return DefaultAssistantSettings(), err
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

func EncodeAssistantSettings(settings AssistantSettings) ([]byte, error) {
	return json.MarshalIndent(settings, "", "  ")
}
