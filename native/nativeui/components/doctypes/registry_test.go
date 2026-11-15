package doc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLookupKnownAndFallback(t *testing.T) {
	r := NewRegistry()

	if tp, ok := r.Lookup(".md"); !ok || tp.ID != "markdown" {
		t.Errorf(".md -> %v %v, want markdown", tp, ok)
	}
	// Case does not matter.
	if tp, ok := r.Lookup(".TXT"); !ok || tp.ID != "text" {
		t.Errorf(".TXT -> %v %v, want text", tp, ok)
	}
	// An unknown but text-editable extension opens as plain text.
	if tp, ok := r.Lookup(".rs"); !ok || tp.ID != "text" {
		t.Errorf(".rs -> %v %v, want the text fallback", tp, ok)
	}
	// A binary extension is refused.
	if tp, ok := r.Lookup(".png"); ok {
		t.Errorf(".png -> %v, want not openable", tp)
	}
}

func TestOpenPlainTextFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry()
	tp, d, err := r.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if tp.ID != "text" || !tp.Editable {
		t.Errorf("type = %+v, want editable text", tp)
	}
	if got := d.PlainText(); got != "line one\nline two\n" {
		t.Errorf("text = %q", got)
	}
}

func TestOpenRefusesBinary(t *testing.T) {
	r := NewRegistry()
	if _, _, err := r.Open("logo.png"); err == nil {
		t.Error("opening logo.png succeeded, want an error")
	}
}

// A plugin can claim a new extension, and it wins over the fallback.
func TestRegisterOverrides(t *testing.T) {
	r := NewRegistry()
	r.Register(&Type{ID: "diary", Name: "Diary", Extensions: []string{".diary"}})
	if tp, ok := r.Lookup(".diary"); !ok || tp.ID != "diary" {
		t.Errorf(".diary -> %v %v, want diary", tp, ok)
	}
}
