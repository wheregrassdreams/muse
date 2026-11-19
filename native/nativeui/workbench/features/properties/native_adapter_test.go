package properties

import (
	"path/filepath"
	"testing"

	"muse/platform/filesystem/configstore"
)

func TestNativeHostProjectsDocumentAndPersistsFieldPreferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	source := "---\ntags: [design]\n---\nBody text.\n"
	host := NewNativeHost(NativePorts{
		Configuration: configstore.NewAt(t.TempDir()),
		CurrentPath:   func() string { return path },
		Document:      func() (string, string) { return path, source },
	})
	host.Open()
	model := host.ViewModel()
	if !host.Enabled() || model.DocumentType == "" || len(model.Fields) == 0 {
		t.Fatalf("projected properties: enabled=%t model=%+v", host.Enabled(), model)
	}
	var tags string
	for _, field := range model.Fields {
		if field.Key == "tags" {
			tags = field.Value
		}
	}
	if tags != "design" {
		t.Fatalf("tags field=%q; fields=%+v", tags, model.Fields)
	}
	host.Dispatch(Action{Kind: TogglePinned, FieldKey: "tags"})
	if !host.Preference("tags").Pinned {
		t.Fatal("native host did not retain the pinned-field preference")
	}
}
