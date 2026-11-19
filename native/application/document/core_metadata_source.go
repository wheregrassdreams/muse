package document

import (
	"path/filepath"
	"strings"

	domain "muse/document"
)

// CoreMetadataDocument is one detached indexed source used to build a
// vault-wide core metadata migration preview.
type CoreMetadataDocument struct {
	Source   string
	Revision domain.Revision
}

// CoreMetadataSource is the read-only vault index port consumed by the
// document migration workflow. Implementations provide absolute paths and
// resolve a vault-relative path to its current indexed source.
type CoreMetadataSource interface {
	RootPath() string
	Paths() []string
	ReadDocument(relativePath string) (CoreMetadataDocument, bool)
}

// CollectCoreMetadataCandidates detaches the in-vault Markdown sources that
// can participate in a migration. Path containment and source filtering stay
// with the document workflow rather than UI composition.
func CollectCoreMetadataCandidates(source CoreMetadataSource) []CoreMetadataCandidate {
	if source == nil {
		return nil
	}
	root := filepath.Clean(source.RootPath())
	if root == "" || root == "." {
		return nil
	}
	paths := source.Paths()
	candidates := make([]CoreMetadataCandidate, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		doc, ok := source.ReadDocument(filepath.ToSlash(rel))
		if !ok {
			continue
		}
		candidates = append(candidates, CoreMetadataCandidate{Path: path, Source: doc.Source, Revision: doc.Revision})
	}
	return candidates
}
