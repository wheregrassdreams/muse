package document

import (
	"strings"
	"testing"
	"time"

	"muse/document/frontmatter"
)

func TestNewNoteSourceIncludesCoreFieldsAndCreatedAt(t *testing.T) {
	now := time.Date(2026, 11, 16, 9, 30, 0, 123, time.FixedZone("UTC+8", 8*60*60))
	source, err := NewNoteSource(now)
	if err != nil {
		t.Fatal(err)
	}
	doc := frontmatter.Parse(source)
	if doc.Err != nil {
		t.Fatal(doc.Err)
	}
	fields := make(map[string]frontmatter.Field, len(doc.Fields))
	for _, field := range doc.Fields {
		fields[field.Key] = field
	}
	if fields["type"].Value != "note" || !fields["type"].Editable {
		t.Fatalf("new note type = %+v", fields["type"])
	}
	for _, key := range []string{"tags", "aliases", "created_at", "updated_at"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("new note is missing core field %q: %q", key, source)
		}
	}
	wantCreated := now.UTC().Format(time.RFC3339Nano)
	if got := fields["created_at"].Value; got != wantCreated {
		t.Errorf("created_at = %q, want %q", got, wantCreated)
	}
	if strings.TrimSpace(source) == "" {
		t.Fatal("new note source is empty")
	}
}
