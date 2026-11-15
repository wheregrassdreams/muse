package exportstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWriterWritesSelectedExportPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.html")
	if err := (Writer{}).WriteExport(context.Background(), path, []byte("html")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "html" {
		t.Fatalf("export = %q, %v", data, err)
	}
}
