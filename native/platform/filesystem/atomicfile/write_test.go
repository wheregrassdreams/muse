package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReplacesAndCleansUpFailedRename(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "state.json")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if stage, err := Write(target, ".state-*.tmp", []byte("new"), 0o600); err != nil || stage != "" {
		t.Fatalf("Write replacement = %q, %v", stage, err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "new" {
		t.Fatalf("replacement contents = %q, %v", got, err)
	}

	blocked := filepath.Join(dir, "blocked")
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(blocked, "keep")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	stage, err := Write(blocked, ".blocked-*.tmp", []byte("cannot publish"), 0o600)
	if err == nil || stage != RenameFile {
		t.Fatalf("Write to directory = %q, %v; want rename failure", stage, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("directory entries after failure = %v, %v; want only target and blocked", entries, err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "keep" {
		t.Fatalf("existing target contents = %q, %v", got, err)
	}
}
