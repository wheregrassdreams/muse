package properties

import (
	"errors"
	"testing"
)

type testPreferences struct {
	values map[string]FieldPreference
	path   string
	saved  map[string]FieldPreference
}

func (p *testPreferences) Load(path string) (map[string]FieldPreference, error) {
	p.path = path
	return clonePreferences(p.values), nil
}

func (p *testPreferences) Save(path string, preferences map[string]FieldPreference) error {
	p.path = path
	p.saved = clonePreferences(preferences)
	return nil
}

func TestHostOwnsPresentationPreferencesAndLocalDrafts(t *testing.T) {
	preferences := &testPreferences{values: map[string]FieldPreference{"status": {Pinned: true}}}
	var gotWorkflow WorkflowRequest
	invalidations := 0
	host := NewHost(Ports{
		CurrentPath: func() string { return "/vault/note.md" },
		Preferences: preferences,
		Project: func(prefs map[string]FieldPreference) Projection {
			return Projection{Model: ViewModel{Fields: []Field{{Key: "status", Value: "draft", Editable: true, Pinned: prefs["status"].Pinned}}}, Markdown: true, FrontmatterValid: true}
		},
		Route:      func(request WorkflowRequest) error { gotWorkflow = request; return nil },
		Invalidate: func() { invalidations++ },
	})

	model := host.ViewModel()
	if preferences.path != "/vault/note.md" || len(model.Fields) != 1 || !model.Fields[0].Pinned {
		t.Fatalf("projection/preferences = %+v, loaded path %q", model, preferences.path)
	}
	host.Dispatch(Action{Kind: ToggleHidden, FieldKey: "status"})
	if !preferences.saved["status"].Hidden || preferences.path != "/vault/note.md" {
		t.Fatalf("presentation preference not persisted: path=%q saved=%+v", preferences.path, preferences.saved)
	}
	host.Dispatch(Action{Kind: BeginEdit, FieldKey: "status"})
	host.Dispatch(Action{Kind: DraftChanged, Value: "ready"})
	if model = host.ViewModel(); model.EditingKey != "status" || model.Draft != "ready" {
		t.Fatalf("edit draft not retained locally: %+v", model)
	}
	host.Dispatch(Action{Kind: SaveEdit, FieldKey: "status", Value: "ready"})
	if gotWorkflow.Kind != SetValue || gotWorkflow.FieldKey != "status" || gotWorkflow.Value != "ready" {
		t.Fatalf("workflow request = %+v", gotWorkflow)
	}
	if model = host.ViewModel(); model.EditingKey != "" || model.Draft != "" {
		t.Fatalf("successful workflow did not clear local draft: %+v", model)
	}
	if invalidations == 0 {
		t.Fatal("host did not invalidate after local actions")
	}
}

func TestHostRetainsDraftOnWorkflowErrorAndKeepsSourceModeLocal(t *testing.T) {
	wantErr := errors.New("invalid frontmatter")
	host := NewHost(Ports{
		CurrentPath: func() string { return "/vault/note.md" },
		Project: func(map[string]FieldPreference) Projection {
			return Projection{Model: ViewModel{Fields: []Field{{Key: "status", Value: "draft", Editable: true}}}, YAMLSource: "status: draft", Markdown: true, FrontmatterValid: false}
		},
		Route: func(WorkflowRequest) error { return wantErr },
	})
	host.Dispatch(Action{Kind: BeginEdit, FieldKey: "status"})
	host.Dispatch(Action{Kind: DraftChanged, Value: "bad: ["})
	host.Dispatch(Action{Kind: SaveEdit, FieldKey: "status", Value: "bad: ["})
	model := host.ViewModel()
	if model.Error != wantErr.Error() || model.EditingKey != "status" || model.Draft != "bad: [" {
		t.Fatalf("failed workflow state = %+v", model)
	}
	host.Dispatch(Action{Kind: OpenSource})
	if !host.IsSourceMode() || host.ViewModel().SourceDraft != "status: draft" {
		t.Fatal("source draft/mode did not remain local to the host")
	}
	if host.Enabled() != true || host.FrontmatterValid() {
		t.Fatal("document capabilities were not read from the host projection")
	}
}

func TestHostRoutesOnlyTypedWorkflowRequests(t *testing.T) {
	var got WorkflowRequest
	host := NewHost(Ports{Route: func(request WorkflowRequest) error { got = request; return nil }})
	host.Dispatch(Action{Kind: OpenBacklink, Backlink: Backlink{Path: "source.md", Offset: 12}})
	if got.Kind != NavigateBacklink || got.Backlink.Path != "source.md" || got.Backlink.Offset != 12 {
		t.Fatalf("backlink route = %+v", got)
	}
	host.Dispatch(Action{Kind: ToggleHiddenSection})
	if got.Kind != NavigateBacklink {
		t.Fatalf("presentation action unexpectedly crossed workflow port: %+v", got)
	}
}
