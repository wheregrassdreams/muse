package nativeui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"muse/document/frontmatter"
	vaultindex "muse/vault/index"
)

func TestCoreMetadataMigrationPreviewUsesIndexedSourceAndTrustedTimes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "legacy.md")
	source := "# Legacy\n\nBody\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	docs := []vaultindex.SearchSnapshot{{Path: "legacy.md", Title: "Legacy", Source: source}}
	preview := previewCoreMetadataMigration(root, "", docs)
	if len(preview.items) != 1 || preview.updatedFromMod != 1 {
		t.Fatalf("migration preview = %+v", preview)
	}
	fields := map[string]frontmatter.Field{}
	for _, field := range frontmatter.Parse(preview.items[0].source).Fields {
		fields[field.Key] = field
	}
	for _, key := range []string{"type", "tags", "aliases", "created_at", "updated_at"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("preview omitted core key %q: %q", key, preview.items[0].source)
		}
	}
	if fields["type"].Value != "note" || fields["updated_at"].Value != info.ModTime().UTC().Format(time.RFC3339Nano) {
		t.Fatalf("preview values do not reflect defaults and source mtime: %+v", fields)
	}
	_, hasBirth := trustedFileBirthTime(info)
	if hasBirth != (fields["created_at"].Value != "") {
		t.Fatalf("created_at fabricated or missed trusted birth time: birth=%v field=%q", hasBirth, fields["created_at"].Value)
	}
	active := previewCoreMetadataMigration(root, path, docs)
	if len(active.items) != 0 || active.openDoc != 1 {
		t.Fatalf("active document was not protected: %+v", active)
	}
}

func TestCoreMetadataMigrationPreservesExplicitEmptyDates(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "legacy.md")
	source := "---\ncreated_at: \"\"\nupdated_at: \"\"\n---\n# Legacy\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	preview := previewCoreMetadataMigration(root, "", []vaultindex.SearchSnapshot{{Path: "legacy.md", Source: source}})
	if len(preview.items) != 1 {
		t.Fatalf("expected only missing type/list fields to migrate: %+v", preview)
	}
	for _, field := range frontmatter.Parse(preview.items[0].source).Fields {
		if (field.Key == "created_at" || field.Key == "updated_at") && field.Value != "" {
			t.Fatalf("explicitly empty %s was overwritten: %+v", field.Key, field)
		}
	}
}
