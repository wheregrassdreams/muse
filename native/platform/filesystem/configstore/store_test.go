package configstore

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"muse/config"
)

func TestNewUsesMuseConfigDirThenStandardMuseDirectory(t *testing.T) {
	custom := t.TempDir()
	t.Setenv("MUSE_CONFIG_DIR", custom)
	if got := New().Root(); got != custom {
		t.Fatalf("MUSE_CONFIG_DIR root = %q, want %q", got, custom)
	}
	t.Setenv("MUSE_CONFIG_DIR", "")
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	if got, want := New().Root(), filepath.Join(base, "Muse"); got != want {
		t.Fatalf("standard root = %q, want %q", got, want)
	}
}

func TestSettingsDefaultsRoundTripAndPrivatePermissions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Muse")
	store := NewAt(root)
	if got, err := store.LoadPreferences(); err != nil || got != config.DefaultPreferences() {
		t.Fatalf("missing preferences = %+v, %v", got, err)
	}
	if got, err := store.LoadEditorSettings(); err != nil || got != config.DefaultEditorSettings() {
		t.Fatalf("missing editor settings = %+v, %v", got, err)
	}
	if got, err := store.LoadAssistantSettings(); err != nil || got != config.DefaultAssistantSettings() {
		t.Fatalf("missing assistant settings = %+v, %v", got, err)
	}
	want := config.Preferences{Theme: config.ThemeDark, Accent: "blue"}
	if err := store.SavePreferences(want); err != nil {
		t.Fatal(err)
	}
	if got, err := store.LoadPreferences(); err != nil || got != want {
		t.Fatalf("preferences round trip = %+v, %v", got, err)
	}
	info, err := os.Stat(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("settings mode = %o, want 600", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("config directory mode = %o, want 700", dirInfo.Mode().Perm())
	}
}

func TestAssistantAndDocumentPropertySettingsRoundTrip(t *testing.T) {
	store := NewAt(t.TempDir())
	assistant := config.AssistantSettings{Provider: config.AssistantProviderMagpie, BaseURL: config.AssistantMagpieBaseURL, Model: "m", Tools: config.AssistantTools{Browse: true}}
	if err := store.SaveAssistantSettings(assistant); err != nil {
		t.Fatal(err)
	}
	if got, err := store.LoadAssistantSettings(); err != nil || got != assistant {
		t.Fatalf("assistant settings = %+v, %v; want %+v", got, err, assistant)
	}
	want := config.DocumentPropertyPreferences{Fields: map[string]config.FieldDisplayPreference{
		"tags":   {Pinned: true, Hidden: true},
		"status": {Order: 2},
	}}
	if err := store.SaveDocumentPropertyPreferences("notes/today.md", want); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDocumentPropertyPreferences("notes/other.md", config.DocumentPropertyPreferences{Fields: map[string]config.FieldDisplayPreference{"type": {Hidden: true}}}); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadDocumentPropertyPreferences("notes/archive/../today.md")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("document preferences = %#v, %v; want %#v", got, err, want)
	}
	other, err := store.LoadDocumentPropertyPreferences("notes/other.md")
	if err != nil || !other.Fields["type"].Hidden {
		t.Fatalf("other document preferences = %#v, %v", other, err)
	}
	if got := store.AssistantSettingsLocation(); got != filepath.Join(store.Root(), "ai.json") {
		t.Fatalf("assistant settings location = %q", got)
	}
}

func TestMalformedPropertyFileIsNotSilentlyOverwritten(t *testing.T) {
	root := t.TempDir()
	store := NewAt(root)
	if err := os.WriteFile(filepath.Join(root, "document-properties.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDocumentPropertyPreferences("note.md", config.DocumentPropertyPreferences{}); err == nil {
		t.Fatal("expected malformed property file to be reported")
	}
}
