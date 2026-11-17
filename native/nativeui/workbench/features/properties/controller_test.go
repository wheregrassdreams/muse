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

type testMetadata struct {
	setKey, setValue string
	addKey, addValue string
	addKind          FieldKind
	deleted          string
	yaml             string
	err              error
}

func (m *testMetadata) SetProperty(key, value string) error {
	m.setKey, m.setValue = key, value
	return m.err
}
func (m *testMetadata) AddProperty(key, value string, kind FieldKind) error {
	m.addKey, m.addValue, m.addKind = key, value, kind
	return m.err
}
func (m *testMetadata) DeleteProperty(key string) error { m.deleted = key; return m.err }
func (m *testMetadata) ReplaceYAML(source string) error { m.yaml = source; return m.err }

func TestControllerProjectsDocumentAndRoutesActionsThroughPorts(t *testing.T) {
	document := DocumentSnapshot{
		Path:     "/vault/note.md",
		Source:   "---\nstatus: draft\n---\n# Body\n",
		Markdown: true,
	}
	preferences := &testPreferences{values: map[string]FieldPreference{"status": {Pinned: true}}}
	metadata := &testMetadata{}
	invalidations := 0
	controller := NewController(ControllerPorts{
		Document:    func() DocumentSnapshot { return document },
		Preferences: preferences,
		Metadata:    metadata,
		SystemProperties: func(DocumentSnapshot) []SystemProperty {
			return []SystemProperty{{Key: SystemPropertyPath, Value: "/vault/note.md"}}
		},
		Backlinks:  func() []BacklinkSource { return []BacklinkSource{{Path: "links/source.md", Count: 1}} },
		Invalidate: func() { invalidations++ },
	})

	model := controller.ViewModel()
	var status *Field
	for i := range model.Fields {
		if model.Fields[i].Key == "status" {
			status = &model.Fields[i]
			break
		}
	}
	if preferences.path != document.Path || status == nil || !status.Pinned {
		t.Fatalf("document projection or preferences = %+v, loaded path %q", model, preferences.path)
	}
	if model.BacklinkCount != 1 || len(model.SystemFields) != 1 {
		t.Fatalf("host projections missing from view model: %+v", model)
	}

	controller.Dispatch(Action{Kind: ToggleHidden, FieldKey: "status"})
	if !preferences.saved["status"].Hidden || preferences.path != document.Path {
		t.Fatalf("preference action was not persisted: path=%q saved=%+v", preferences.path, preferences.saved)
	}
	controller.Dispatch(Action{Kind: BeginEdit, FieldKey: "status"})
	controller.Dispatch(Action{Kind: SaveEdit, FieldKey: "status", Value: "ready"})
	if metadata.setKey != "status" || metadata.setValue != "ready" {
		t.Fatalf("edit was not routed through metadata port: %+v", metadata)
	}
	if invalidations == 0 {
		t.Fatal("controller did not invalidate after actions")
	}
}

func TestControllerReportsMetadataErrorsAndRejectsUnsupportedDocuments(t *testing.T) {
	metadata := &testMetadata{err: errors.New("invalid YAML")}
	controller := NewController(ControllerPorts{
		Document: func() DocumentSnapshot {
			return DocumentSnapshot{Path: "/vault/note.md", Source: "---\nstatus: draft\n---\n", Markdown: true}
		},
		Metadata: metadata,
	})
	controller.Dispatch(Action{Kind: SaveEdit, FieldKey: "status", Value: "bad: ["})
	if controller.ViewModel().Error != "invalid YAML" {
		t.Fatal("metadata error was not retained for the panel")
	}

	unsupported := NewController(ControllerPorts{Document: func() DocumentSnapshot {
		return DocumentSnapshot{Path: "/vault/image.png", Markdown: false}
	}})
	if unsupported.Enabled() {
		t.Fatal("non-Markdown document unexpectedly enabled properties")
	}
	if unsupported.ViewModel().DocumentType != "" {
		t.Fatal("unsupported document received a Markdown projection")
	}
}
