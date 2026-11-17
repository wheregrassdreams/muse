package properties

import (
	"testing"
	"time"

	"muse/document/frontmatter"
)

func TestStateOwnsPreferenceSnapshotsAndParsedDocumentCache(t *testing.T) {
	state := &state{}
	preferences := map[string]FieldPreference{"tags": {Pinned: true, Order: 2}}
	state.SetPreferences("/vault/note.md", preferences)
	preferences["tags"] = FieldPreference{}
	if got := state.Preference("tags"); !got.Pinned || got.Order != 2 {
		t.Fatalf("preference aliased caller map: %+v", got)
	}
	snapshot := state.Preferences()
	snapshot["tags"] = FieldPreference{}
	if got := state.Preference("tags"); !got.Pinned || got.Order != 2 {
		t.Fatalf("preference snapshot aliased panel state: %+v", got)
	}

	source := "---\ntags: [one]\n---\nBody"
	first := state.Document("/vault/note.md", source)
	second := state.Document("/vault/note.md", source)
	if len(first.Fields) != 1 || &first.Fields[0] != &second.Fields[0] {
		t.Fatalf("unchanged document did not reuse its parsed projection: %+v", second.Fields)
	}
	changed := state.Document("/vault/note.md", "---\nstatus: draft\n---\nBody")
	if len(changed.Fields) != 1 || changed.Fields[0].Key != "status" {
		t.Fatalf("changed source retained stale projection: %+v", changed.Fields)
	}
}

func TestMoveFieldStaysWithinEditablePinnedGroup(t *testing.T) {
	state := &state{}
	state.SetPreferences("note.md", map[string]FieldPreference{
		"status":   {Pinned: true},
		"custom-a": {Order: 1},
		"custom-b": {Order: 2},
	})
	fields := []frontmatter.Field{
		{Key: "status", Value: "draft", Kind: "text", Editable: true},
		{Key: "custom-a", Value: "a", Kind: "text", Editable: true},
		{Key: "custom-b", Value: "b", Kind: "text", Editable: true},
		{Key: "tags", Value: "one", Kind: "list", Editable: true},
	}
	if !state.MoveField("custom-b", -1, fields) {
		t.Fatal("expected a move within the unpinned group")
	}
	if state.Preference("custom-b").Order >= state.Preference("custom-a").Order {
		t.Fatalf("custom order was not updated: a=%+v b=%+v", state.Preference("custom-a"), state.Preference("custom-b"))
	}
	if state.MoveField("status", 1, fields) {
		t.Fatal("a pinned field crossed into the unpinned group")
	}
	if state.MoveField("tags", 1, fields) {
		t.Fatal("a core field was reordered")
	}
}

func TestStateOwnsSystemPropertyAndOpenTimeSnapshots(t *testing.T) {
	state := &state{}
	properties := []SystemProperty{{Key: "path", Value: "/vault/a.md"}}
	state.SetSystemProperties(properties)
	properties[0].Value = "mutated"
	if got := state.SystemProperties(); len(got) != 1 || got[0].Value != "/vault/a.md" {
		t.Fatalf("system properties aliased caller: %+v", got)
	}
	openedAt := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	state.SetOpenedAt(openedAt)
	if got := state.OpenedAt(); !got.Equal(openedAt) {
		t.Fatalf("opened at = %v, want %v", got, openedAt)
	}
}

func TestApplyTogglesPreferencesInsidePanelState(t *testing.T) {
	state := &state{}
	state.SetPreferences("note.md", nil)
	state.Apply(Action{Kind: ToggleHidden, FieldKey: "tags"})
	if !state.Preference("tags").Hidden {
		t.Fatal("ToggleHidden did not update panel-owned preference")
	}
	state.Apply(Action{Kind: TogglePinned, FieldKey: "tags"})
	if !state.Preference("tags").Pinned || !state.Preference("tags").Hidden {
		t.Fatalf("toggle actions did not preserve independent preference values: %+v", state.Preference("tags"))
	}
}
