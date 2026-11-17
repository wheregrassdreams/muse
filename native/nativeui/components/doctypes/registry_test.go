package doctypes

import (
	"testing"
)

func TestIsTextDataRejectsBinaryContent(t *testing.T) {
	for _, data := range [][]byte{
		{'a', 0, 'b'},
		{0xff, 0xfe},
	} {
		if IsTextData(data) {
			t.Errorf("IsTextData(%v) = true, want false", data)
		}
	}
	if !IsTextData(nil) || !IsTextData([]byte("plain text 中文")) {
		t.Fatal("IsTextData rejected empty or valid UTF-8 text")
	}
}

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

func TestResolveSelectsParserWithoutParsing(t *testing.T) {
	r := NewRegistry()
	tp, err := r.Resolve("notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if tp.ID != "text" || !tp.Editable || tp.Parser != TextParser {
		t.Errorf("type = %+v, want editable text", tp)
	}
	if tp, err := r.Resolve("note.md"); err != nil || tp.Parser != MarkdownParser {
		t.Errorf("Markdown parser selection = %v, %v", tp, err)
	}
}

func TestResolveRefusesBinary(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Resolve("logo.png"); err == nil {
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

func TestEditableVisibilityUsesRegisteredAllowlist(t *testing.T) {
	r := NewRegistry()
	for _, path := range []string{"note.md", "src/main.go", "config.json5", "README", "Makefile"} {
		if !r.IsEditablePath(path) {
			t.Errorf("IsEditablePath(%q) = false, want true", path)
		}
	}
	for _, path := range []string{"payload.unknown", "photo.png", "archive.zip"} {
		if r.IsEditablePath(path) {
			t.Errorf("IsEditablePath(%q) = true, want false", path)
		}
	}
	if got := r.EditableExtensions(); len(got) == 0 || got[0] == ".md" {
		t.Fatalf("EditableExtensions should be a non-empty suffix list without dots: %v", got)
	}
}

func TestImageExtensionsAreSharedWithPathClassification(t *testing.T) {
	for _, ext := range ImageExtensions() {
		if !IsImagePath("image" + ext) {
			t.Errorf("IsImagePath(image%s) = false", ext)
		}
	}
	if IsImagePath("note.md") {
		t.Fatal("markdown path classified as image")
	}
}
