package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAssistantSettingsDefaultsAndLegacyTools(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MUSE_CONFIG_DIR", dir)
	settings, err := LoadAssistantSettings()
	if err != nil || settings.Tools != DefaultAssistantTools() {
		t.Fatalf("missing settings = %+v, %v", settings, err)
	}
	if err := os.WriteFile(AssistantSettingsPath(), []byte(`{"baseUrl":"https://example.test/v1","model":"m"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err = LoadAssistantSettings()
	if err != nil || settings.BaseURL == "" || settings.Tools != DefaultAssistantTools() {
		t.Fatalf("legacy settings = %+v, %v", settings, err)
	}
}

func TestAssistantSettingsRoundTripAndExplicitToolValues(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	want := AssistantSettings{Provider: AssistantProviderMagpie, BaseURL: AssistantMagpieBaseURL, Model: "m", Tools: AssistantTools{Browse: true}}
	if err := SaveAssistantSettings(want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadAssistantSettings()
	if err != nil || got != want {
		t.Fatalf("round trip = %+v, %v; want %+v", got, err, want)
	}
	info, err := os.Stat(filepath.Join(os.Getenv("MUSE_CONFIG_DIR"), "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("settings mode = %o, want 600", info.Mode().Perm())
	}
}
