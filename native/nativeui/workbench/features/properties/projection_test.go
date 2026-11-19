package properties

import (
	"testing"

	"muse/document/frontmatter"
)

func TestProjectViewModelOwnsDisplayProjection(t *testing.T) {
	input := ProjectionInput{
		Title:        "Properties",
		DocumentType: "note",
		Projection: frontmatter.InspectorProjection{
			Ordered: []frontmatter.Field{
				{Key: "tags", Value: "one, two", Kind: "list", Editable: true},
				{Key: "status", Value: "draft", Kind: "text", Editable: true},
			},
			Visible: []frontmatter.Field{
				{Key: "tags", Value: "one, two", Kind: "list", Editable: true},
				{Key: "status", Value: "draft", Kind: "text", Editable: true},
			},
			Summary: []frontmatter.Field{
				{Key: "tags", Value: "one, two", Kind: "list", Editable: true},
				{Key: "status", Value: "draft", Kind: "text", Editable: true},
				{Key: "date", Value: "2026-11-16", Kind: "date", Editable: true},
				{Key: "aliases", Value: "a", Kind: "list", Editable: true},
				{Key: "updated_at", Value: "today", Kind: "date", Editable: false},
				{Key: "custom", Value: "x", Kind: "text", Editable: true},
			},
		},
		Preferences: map[string]FieldPreference{
			"tags":   {Pinned: true},
			"status": {Pinned: true},
		},
		System: []SystemProperty{
			{Key: "created-at", Value: "2026-01-01"},
			{Key: "path", Value: "/vault/Plan.md"},
		},
		LastOpened: "2026-11-16 12:00",
		Backlinks:  []BacklinkSource{{Path: "Projects/Plan.md", Offset: 42, Count: 2}},
	}

	model := ProjectViewModel(input)
	if model.Title != "Properties" || model.DocumentType != "note" || len(model.Fields) != 2 {
		t.Fatalf("base projection = %+v", model)
	}
	if model.Fields[0].Kind != List || len(model.Fields[0].Tags) != 2 || !model.Fields[1].CanMoveUp {
		t.Fatalf("field projection = %+v", model.Fields)
	}
	if model.MoreCount != 2 || len(model.Summary) != 5 {
		t.Fatalf("summary projection = %d fields, MoreCount=%d", len(model.Summary), model.MoreCount)
	}
	if len(model.SystemFields) != 1 || model.SystemFields[0].Key != "system-path" {
		t.Fatalf("system fields = %+v", model.SystemFields)
	}
	if len(model.HiddenFields) != 1 || model.HiddenFields[0].Key != "system-last-opened" {
		t.Fatalf("last-opened field = %+v", model.HiddenFields)
	}
	if model.BacklinkCount != 1 || model.Backlinks[0].Label != "Plan" || model.Backlinks[0].Offset != 42 {
		t.Fatalf("backlinks = %+v count=%d", model.Backlinks, model.BacklinkCount)
	}
}

func TestProjectViewModelStopsAtProjectionError(t *testing.T) {
	model := ProjectViewModel(ProjectionInput{Title: "Properties", Error: "invalid frontmatter"})
	if model.Error != "invalid frontmatter" || len(model.Fields) != 0 || len(model.Backlinks) != 0 {
		t.Fatalf("error projection = %+v", model)
	}
}
