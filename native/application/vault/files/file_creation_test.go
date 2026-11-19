package files

import (
	"os"
	"path/filepath"
	"testing"

	"muse/document/frontmatter"
	"muse/locales"
)

func TestFileUseCasesCreateApplicationDefaultNoteAndFolder(t *testing.T) {
	root := t.TempDir()
	files := NewFileUseCases(root, NewFileOperations(&localFileSystem{}), nil)

	notePath, err := files.CreateUntitledNote(root)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(notePath) != locales.DocumentUntitled()+".md" {
		t.Fatalf("default note path = %q", notePath)
	}
	source, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	metadata := frontmatter.Parse(string(source))
	fields := map[string]frontmatter.Field{}
	for _, field := range metadata.Fields {
		fields[field.Key] = field
	}
	for _, key := range []string{"type", "tags", "aliases", "created_at", "updated_at"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("default note is missing %q: %s", key, source)
		}
	}
	if fields["type"].Value != "note" || fields["created_at"].Value == "" || fields["updated_at"].Value != "" {
		t.Fatalf("unexpected default note metadata: %+v", fields)
	}

	folderPath, err := files.CreateDefaultFolder(root)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(folderPath) != locales.FilesNewFolderDefault() {
		t.Fatalf("default folder path = %q", folderPath)
	}
}
