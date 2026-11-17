package document

import (
	"errors"
	"testing"
)

type metadataSourceStub struct {
	source       string
	replacements []string
	replaceErr   error
}

func (s *metadataSourceStub) Source() string { return s.source }

func (s *metadataSourceStub) ReplaceUser(source string) error {
	if s.replaceErr != nil {
		return s.replaceErr
	}
	s.source = source
	s.replacements = append(s.replacements, source)
	return nil
}

func TestMetadataEditorAddsAndSetsPropertiesThroughSourcePort(t *testing.T) {
	port := &metadataSourceStub{source: "# Note\n"}
	editor := NewMetadataEditor(port)
	if err := editor.AddProperty("tags", "work, personal", PropertyText); err != nil {
		t.Fatal(err)
	}
	if got, want := port.source, "---\ntags: [\"work\", \"personal\"]\n---\n\n# Note\n"; got != want {
		t.Fatalf("added metadata source = %q, want %q", got, want)
	}
	if err := editor.SetProperty("tags", "work"); err != nil {
		t.Fatal(err)
	}
	if got, want := port.source, "---\ntags: [\"work\"]\n---\n\n# Note\n"; got != want {
		t.Fatalf("updated metadata source = %q, want %q", got, want)
	}
	if len(port.replacements) != 2 {
		t.Fatalf("ReplaceUser calls = %d, want 2", len(port.replacements))
	}
}

func TestMetadataEditorNormalizesPropertyKey(t *testing.T) {
	port := &metadataSourceStub{source: "# Note\n"}
	if err := NewMetadataEditor(port).AddProperty("  tags  ", "work", PropertyText); err != nil {
		t.Fatal(err)
	}
	if got, want := port.source, "---\ntags: [\"work\"]\n---\n\n# Note\n"; got != want {
		t.Fatalf("normalized metadata source = %q, want %q", got, want)
	}
}

func TestMetadataEditorDoesNotReplaceSourceForNoOp(t *testing.T) {
	port := &metadataSourceStub{source: "---\ntags: [\"work\"]\n---\n\n# Note\n"}
	editor := NewMetadataEditor(port)
	if err := editor.SetProperty("tags", "work"); err != nil {
		t.Fatal(err)
	}
	if len(port.replacements) != 0 {
		t.Fatalf("ReplaceUser calls = %d, want 0 for a no-op", len(port.replacements))
	}
}

func TestMetadataEditorRejectsInvalidOrManagedProperties(t *testing.T) {
	port := &metadataSourceStub{source: "---\ncreated_at: 2026-01-01\n---\n# Note\n"}
	editor := NewMetadataEditor(port)
	if err := editor.AddProperty("date", "not-a-date", PropertyDate); err == nil {
		t.Fatal("AddProperty accepted an invalid date")
	}
	if err := editor.SetProperty("created_at", "2026-02-01"); err == nil {
		t.Fatal("SetProperty accepted a managed timestamp")
	}
	if err := editor.DeleteProperty("tags"); err == nil {
		t.Fatal("DeleteProperty accepted a core property")
	}
	if len(port.replacements) != 0 {
		t.Fatalf("invalid operations called ReplaceUser %d times", len(port.replacements))
	}
}

func TestMetadataEditorReplacesYAMLAndPreservesBody(t *testing.T) {
	port := &metadataSourceStub{source: "---\nstatus: draft\n---\nBody\n"}
	editor := NewMetadataEditor(port)
	if err := editor.ReplaceYAML("---\nstatus: done\ntags: [work]\n---\n"); err != nil {
		t.Fatal(err)
	}
	if got, want := port.source, "---\nstatus: done\ntags: [work]\n---\nBody\n"; got != want {
		t.Fatalf("YAML replacement = %q, want %q", got, want)
	}
	if err := editor.ReplaceYAML("---\ninvalid: [\n---\n"); err == nil {
		t.Fatal("ReplaceYAML accepted invalid YAML")
	}
}

func TestMetadataEditorPropagatesSourceAndKindErrors(t *testing.T) {
	replaceErr := errors.New("replace failed")
	port := &metadataSourceStub{source: "# Note\n", replaceErr: replaceErr}
	if err := NewMetadataEditor(port).AddProperty("status", "done", PropertyText); !errors.Is(err, replaceErr) {
		t.Fatalf("AddProperty error = %v, want %v", err, replaceErr)
	}
	if err := NewMetadataEditor(port).AddProperty("status", "done", PropertyValueKind(255)); err == nil {
		t.Fatal("AddProperty accepted an unsupported kind")
	}
}

func TestMetadataEditorRequiresSourcePort(t *testing.T) {
	if err := NewMetadataEditor(nil).SetProperty("status", "done"); !errors.Is(err, ErrMetadataSourceUnavailable) {
		t.Fatalf("SetProperty error = %v, want %v", err, ErrMetadataSourceUnavailable)
	}
}
