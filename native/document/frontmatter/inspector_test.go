package frontmatter

import (
	"reflect"
	"testing"
)

func TestProjectInspectorGroupsCoreManagedAndUserFields(t *testing.T) {
	fields := []Field{
		{Key: "type", Value: "note", Kind: "text", Editable: true},
		{Key: "tags", Value: "work", Kind: "list", Editable: true},
		{Key: "aliases", Value: "memo", Kind: "list", Editable: true},
		{Key: "created_at", Value: "2026-01-02", Kind: "date", Editable: true},
		{Key: "updated_at", Value: "2026-02-03", Kind: "date", Editable: true},
		{Key: "last_opened", Value: "2026-02-04", Kind: "date", Editable: true},
		{Key: "title", Value: "Legacy title", Kind: "text", Editable: true},
		{Key: "status", Value: "draft", Kind: "text", Editable: true},
		{Key: "custom", Value: "hidden value", Kind: "text", Editable: true},
	}
	preferences := map[string]FieldPreference{"custom": {Hidden: true}}
	projection := ProjectInspector(fields, preferences)

	coreKeys := fieldKeys(projection.Core)
	wantCore := []string{"type", "tags", "aliases", "created_at", "updated_at"}
	if !reflect.DeepEqual(coreKeys, wantCore) {
		t.Fatalf("core keys = %v, want %v", coreKeys, wantCore)
	}
	if projection.Core[0].Value != "note" || !projection.Core[0].Editable {
		t.Fatalf("default type projection = %+v", projection.Core[0])
	}
	if projection.Core[3].Value != "2026-01-02" || projection.Core[3].Editable {
		t.Fatalf("managed creation timestamp should remain read-only: %+v", projection.Core[3])
	}
	if projection.Core[4].Editable {
		t.Fatalf("managed update timestamp should remain read-only: %+v", projection.Core[4])
	}
	if got, want := fieldKeys(projection.Visible), []string{"type", "tags", "aliases", "created_at", "status"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("visible keys = %v, want %v", got, want)
	}
	if got, want := fieldKeys(projection.Hidden), []string{"updated_at", "last_opened", "custom"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("hidden keys = %v, want %v", got, want)
	}
	if got, want := fieldKeys(projection.Summary), []string{"tags", "status", "aliases", "created_at", "updated_at"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("summary keys = %v, want %v", got, want)
	}
}

func TestProjectInspectorAppliesPinnedAndExplicitOrderBeforeDefaults(t *testing.T) {
	fields := []Field{
		{Key: "custom", Value: "x"},
		{Key: "date", Value: "2026-01-01"},
		{Key: "aliases", Value: "a"},
		{Key: "status", Value: "draft"},
		{Key: "tags", Value: "one"},
	}
	preferences := map[string]FieldPreference{
		"custom":  {Pinned: true},
		"aliases": {Order: 1},
		"status":  {Order: 2},
	}
	projection := ProjectInspector(fields, preferences)
	want := []string{"custom", "aliases", "status", "tags", "date"}
	if got := fieldKeys(projection.Ordered); !reflect.DeepEqual(got, want) {
		t.Fatalf("ordered keys = %v, want %v", got, want)
	}
}

func TestProjectInspectorKeepsStructureFieldInSummaryWithoutLocalizedFormatting(t *testing.T) {
	projection := ProjectInspector([]Field{{Key: "config", Kind: "structure"}}, nil)
	if got := fieldKeys(projection.Summary); !reflect.DeepEqual(got, []string{"config"}) {
		t.Fatalf("summary keys = %v, want structure field included", got)
	}
}

func fieldKeys(fields []Field) []string {
	keys := make([]string, len(fields))
	for i, field := range fields {
		keys[i] = field.Key
	}
	return keys
}
