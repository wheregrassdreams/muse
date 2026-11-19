package properties

import "testing"

func TestStateOwnsPreferenceSnapshots(t *testing.T) {
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
}

func TestMoveFieldStaysWithinEditablePinnedGroup(t *testing.T) {
	state := &state{}
	state.SetPreferences("note.md", map[string]FieldPreference{
		"status":   {Pinned: true},
		"custom-a": {Order: 1},
		"custom-b": {Order: 2},
	})
	fields := []Field{
		{Key: "status", Editable: true, Pinned: true, CanReorder: false},
		{Key: "custom-a", Editable: true, CanReorder: true},
		{Key: "custom-b", Editable: true, CanReorder: true},
		{Key: "tags", Editable: true, CanReorder: false},
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
