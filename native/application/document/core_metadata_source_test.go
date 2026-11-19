package document

import (
	"path/filepath"
	"testing"

	domain "muse/document"
)

type fakeCoreMetadataSource struct {
	root      string
	paths     []string
	documents map[string]CoreMetadataDocument
	reads     []string
}

func (s *fakeCoreMetadataSource) RootPath() string { return s.root }
func (s *fakeCoreMetadataSource) Paths() []string  { return append([]string(nil), s.paths...) }
func (s *fakeCoreMetadataSource) ReadDocument(path string) (CoreMetadataDocument, bool) {
	s.reads = append(s.reads, path)
	doc, ok := s.documents[path]
	return doc, ok
}

func TestCollectCoreMetadataCandidatesContainsOnlyReadableInVaultPaths(t *testing.T) {
	root := filepath.Clean("/vault")
	source := &fakeCoreMetadataSource{
		root: root,
		paths: []string{
			filepath.Join(root, "one.md"),
			filepath.Join(root, "nested", "two.md"),
			filepath.Clean("/other/outside.md"),
			filepath.Join(root, "..", "outside.md"),
			filepath.Join(root, "missing.md"),
		},
		documents: map[string]CoreMetadataDocument{
			"one.md":        {Source: "first", Revision: domain.RevisionOf("first")},
			"nested/two.md": {Source: "second", Revision: domain.RevisionOf("second")},
		},
	}

	candidates := CollectCoreMetadataCandidates(source)
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2: %+v", len(candidates), candidates)
	}
	if candidates[0].Path != filepath.Join(root, "one.md") || candidates[0].Source != "first" {
		t.Fatalf("first candidate = %+v", candidates[0])
	}
	if candidates[1].Path != filepath.Join(root, "nested", "two.md") || candidates[1].Revision != domain.RevisionOf("second") {
		t.Fatalf("second candidate = %+v", candidates[1])
	}
	if len(source.reads) != 3 || source.reads[0] != "one.md" || source.reads[1] != "nested/two.md" || source.reads[2] != "missing.md" {
		t.Fatalf("ReadDocument calls = %#v", source.reads)
	}
}

func TestCollectCoreMetadataCandidatesHandlesMissingSourceAndRoot(t *testing.T) {
	if got := CollectCoreMetadataCandidates(nil); got != nil {
		t.Fatalf("nil source candidates = %#v", got)
	}
	if got := CollectCoreMetadataCandidates(&fakeCoreMetadataSource{root: ""}); got != nil {
		t.Fatalf("empty-root candidates = %#v", got)
	}
}
