package nativeui

import (
	"strings"
	"testing"

	"muse/document/frontmatter"
)

func TestPropertyCoreFieldsAlwaysPresentAndManagedDatesReadOnly(t *testing.T) {
	fields := propertyCoreFields(nil)
	want := []string{"type", "tags", "aliases", "created_at", "updated_at"}
	if len(fields) != len(want) {
		t.Fatalf("got %d core fields, want %d", len(fields), len(want))
	}
	for i, key := range want {
		if fields[i].Key != key {
			t.Errorf("field %d key = %q, want %q", i, fields[i].Key, key)
		}
	}
	if fields[0].Value != "note" || !fields[0].Editable {
		t.Errorf("empty type should display editable note default, got %#v", fields[0])
	}
	for _, f := range fields[1:3] {
		if !f.Editable || f.Value != "" {
			t.Errorf("empty user property should stay editable and empty: %#v", f)
		}
	}
	for _, f := range fields[3:] {
		if f.Editable || f.Value != "" {
			t.Errorf("managed date should be read-only and empty: %#v", f)
		}
	}
}

func TestPropertyCoreFieldsUseStoredValuesButKeepManagedDatesReadOnly(t *testing.T) {
	fields := propertyCoreFields([]frontmatter.Field{
		{Key: "type", Value: "project", Kind: "text", Editable: true},
		{Key: "tags", Value: "work, draft", Kind: "list", Editable: true},
		{Key: "created_at", Value: "2026-01-02T03:04:05Z", Kind: "date", Editable: true},
		{Key: "updated_at", Value: "2026-02-03T04:05:06Z", Kind: "date", Editable: true},
	})
	if fields[0].Value != "project" || fields[1].Value != "work, draft" {
		t.Fatalf("stored values not projected: %#v", fields[:2])
	}
	if fields[3].Value == "" || fields[3].Editable || fields[4].Value == "" || fields[4].Editable {
		t.Fatalf("stored managed timestamps should remain read-only: %#v", fields[3:])
	}
}

func TestPropertySummaryOmitsEmptyValues(t *testing.T) {
	fields := []frontmatter.Field{
		{Key: "type", Value: "note"},
		{Key: "tags", Kind: "list"},
		{Key: "aliases", Kind: "list"},
		{Key: "created_at", Kind: "date"},
		{Key: "updated_at", Value: "2026-11-13T08:54:30.456Z", Kind: "text"},
		{Key: "status", Value: "draft"},
	}
	got := summaryPropertyFields(fields, documentPropertyPrefs{})
	if len(got) != 2 || got[0].Key != "updated_at" || got[1].Key != "status" {
		t.Fatalf("summary fields = %#v, want updated_at and status only", got)
	}
	if got := displayPropertyValue(got[0]); !strings.HasPrefix(got, "2026年11月13日 ") || strings.Contains(got, "T") {
		t.Fatalf("timestamp display = %q", got)
	}
}
