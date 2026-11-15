package filemetadata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadMetadataReturnsSizeAndModificationTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	metadata, err := New().ReadMetadata(path)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Size != info.Size() {
		t.Fatalf("size = %d, want %d", metadata.Size, info.Size())
	}
	if !metadata.ModifiedAt.Equal(info.ModTime()) {
		t.Fatalf("modified = %v, want %v", metadata.ModifiedAt, info.ModTime())
	}
}
