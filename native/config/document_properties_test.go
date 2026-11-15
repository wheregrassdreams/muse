package config

import (
	"reflect"
	"testing"
)

func TestDocumentPropertyPreferencesRoundTripByNotePath(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	want := DocumentPropertyPreferences{Fields: map[string]FieldDisplayPreference{
		"tags":   {Pinned: true, Hidden: true},
		"status": {Order: 2},
	}}
	if err := SaveDocumentPropertyPreferences("notes/today.md", want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDocumentPropertyPreferences("notes/today.md")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("preferences = %#v, want %#v", got, want)
	}
	normalized, err := LoadDocumentPropertyPreferences("notes/archive/../today.md")
	if err != nil || !reflect.DeepEqual(normalized, want) {
		t.Fatalf("normalized preferences = %#v, err=%v", normalized, err)
	}
	other, err := LoadDocumentPropertyPreferences("notes/other.md")
	if err != nil || len(other.Fields) != 0 {
		t.Fatalf("other note preferences = %#v, err=%v", other, err)
	}
}
