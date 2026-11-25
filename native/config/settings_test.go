package config

import (
	"reflect"
	"testing"
)

func TestDecodeAssistantSettingsDefaultsLegacyToolsAndPreservesExplicitValues(t *testing.T) {
	legacy, err := DecodeAssistantSettings([]byte(`{"baseUrl":"https://example.test/v1","model":"m"}`))
	if err != nil || legacy.Tools != DefaultAssistantTools() {
		t.Fatalf("legacy assistant settings = %+v, %v", legacy, err)
	}
	explicit, err := DecodeAssistantSettings([]byte(`{"tools":{"browse":true}}`))
	if err != nil || explicit.Tools != (AssistantTools{Browse: true}) {
		t.Fatalf("explicit tools = %+v, %v", explicit.Tools, err)
	}
}

func TestSettingsEncodeDecodeNormalizeValues(t *testing.T) {
	prefs, err := DecodePreferences([]byte(`{"theme":"invalid","accent":"red"}`))
	if err != nil || prefs.Theme != ThemeSystem || prefs.Accent != "red" {
		t.Fatalf("preferences normalization = %+v, %v", prefs, err)
	}
	encoded, err := EncodeEditorSettings(EditorSettings{Keymap: "unknown"})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := DecodeEditorSettings(encoded)
	if err != nil || settings.Keymap != "default" {
		t.Fatalf("editor settings normalization = %+v, %v", settings, err)
	}
}

func TestWindowBackdropPreferencesDefaultAndLegacyDecode(t *testing.T) {
	defaults := DefaultPreferences()
	if !defaults.WindowBlur || defaults.WindowOpacity != 0.7 {
		t.Fatalf("window backdrop defaults = blur %v opacity %.2f", defaults.WindowBlur, defaults.WindowOpacity)
	}
	got, err := DecodePreferences([]byte(`{"theme":"dark"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !got.WindowBlur || got.WindowOpacity != 0.7 {
		t.Fatalf("legacy preferences did not receive window defaults: %+v", got)
	}
}

func TestDocumentPropertyFileRoundTripAndPathNormalization(t *testing.T) {
	want := DocumentPropertyPreferences{Fields: map[string]FieldDisplayPreference{
		"tags":   {Pinned: true, Hidden: true},
		"status": {Order: 2},
	}}
	all := SetDocumentPreferences(nil, "notes/today.md", want)
	encoded, err := EncodeDocumentPropertyFile(all)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeDocumentPropertyFile(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got := DocumentPreferences(decoded, "notes/archive/../today.md"); !reflect.DeepEqual(got, want) {
		t.Fatalf("document preferences = %#v, want %#v", got, want)
	}
	if got := DocumentPreferences(decoded, "notes/other.md"); len(got.Fields) != 0 {
		t.Fatalf("other note preferences = %#v", got)
	}
}
