package document

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	domain "muse/document"
	"muse/document/frontmatter"
	"muse/events"
)

// CoreMetadataCandidate is the indexed document input to a vault-wide metadata
// migration. Path is absolute; Source and Revision are the same indexed
// snapshot so a later save can reject stale plans.
type CoreMetadataCandidate struct {
	Path     string
	Source   string
	Revision domain.Revision
}

type CoreMetadataChange struct {
	Path             string
	Source           string
	ExpectedRevision domain.Revision
}

// CoreMetadataPlan contains detached changes and preview counts. Paths in
// Samples are vault-relative for readable UI summaries.
type CoreMetadataPlan struct {
	Root             string
	Changes          []CoreMetadataChange
	CreatedFromBirth int
	UpdatedFromMod   int
	UnknownCreation  int
	Invalid          int
	OpenDocument     int
	Samples          []string
}

// CoreMetadataMigration applies the document-owned policy for filling missing
// core frontmatter fields. Filesystem metadata and durable storage are ports.
type CoreMetadataMigration struct {
	metadata MetadataReader
	saver    *Saver
}

func NewCoreMetadataMigration(metadata MetadataReader, repository domain.Repository, recovery domain.RecoveryRepository, dispatcher *events.Dispatcher[domain.DocumentSaved]) *CoreMetadataMigration {
	var saver *Saver
	if repository != nil {
		saver = NewSaver(repository, recovery, dispatcher)
	}
	return &CoreMetadataMigration{metadata: metadata, saver: saver}
}

// Preview selects valid Markdown notes, skips the currently open note, and
// computes missing core metadata values from injected filesystem timestamps.
// It never reads or writes files itself.
func (m *CoreMetadataMigration) Preview(root, openPath string, candidates []CoreMetadataCandidate) (CoreMetadataPlan, error) {
	if m == nil || m.metadata == nil {
		return CoreMetadataPlan{}, errors.New("document metadata reader is not configured")
	}
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return CoreMetadataPlan{}, err
	}
	openPath = cleanAbsolutePath(openPath)
	plan := CoreMetadataPlan{Root: root}
	for _, candidate := range candidates {
		path := cleanAbsolutePath(candidate.Path)
		if path == "" || !strings.EqualFold(filepath.Ext(path), ".md") {
			continue
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || !isVaultRelative(rel) {
			plan.Invalid++
			continue
		}
		if path == openPath {
			plan.OpenDocument++
			continue
		}

		parsed := frontmatter.Parse(candidate.Source)
		if parsed.Err != nil {
			plan.Invalid++
			continue
		}
		present := make(map[string]bool, len(parsed.Fields))
		for _, field := range parsed.Fields {
			present[field.Key] = true
		}
		next, err := frontmatter.EnsureCore(candidate.Source)
		if err != nil {
			plan.Invalid++
			continue
		}
		metadata := FileMetadata{}
		if !present["created_at"] || !present["updated_at"] {
			metadata, err = m.metadata.ReadMetadata(path)
			if err != nil {
				plan.Invalid++
				continue
			}
		}
		if !present["created_at"] {
			if !metadata.CreatedAt.IsZero() {
				next, err = frontmatter.SetTyped(next, "created_at", metadata.CreatedAt.UTC().Format(time.RFC3339Nano), "text")
				if err == nil {
					plan.CreatedFromBirth++
				}
			} else {
				plan.UnknownCreation++
			}
		}
		if err == nil && !present["updated_at"] && !metadata.ModifiedAt.IsZero() {
			next, err = frontmatter.SetTyped(next, "updated_at", metadata.ModifiedAt.UTC().Format(time.RFC3339Nano), "text")
			if err == nil {
				plan.UpdatedFromMod++
			}
		}
		if err != nil {
			plan.Invalid++
			continue
		}
		if next == candidate.Source {
			continue
		}
		revision := candidate.Revision
		if revision == "" {
			revision = domain.RevisionOf(candidate.Source)
		}
		plan.Changes = append(plan.Changes, CoreMetadataChange{
			Path: path, Source: next, ExpectedRevision: revision,
		})
		if len(plan.Samples) < 5 {
			plan.Samples = append(plan.Samples, filepath.ToSlash(rel))
		}
	}
	return plan, nil
}

type CoreMetadataApplyResult struct {
	AppliedPaths []string
	Skipped      int
}

// Apply saves planned sources through the normal revision-checked document
// workflow. It rechecks the open document at apply time because the user may
// have navigated after previewing the plan.
func (m *CoreMetadataMigration) Apply(plan CoreMetadataPlan, openPath string) (CoreMetadataApplyResult, error) {
	if m == nil || m.saver == nil {
		return CoreMetadataApplyResult{}, errors.New("document repository is not configured")
	}
	openPath = cleanAbsolutePath(openPath)
	root := cleanAbsolutePath(plan.Root)
	result := CoreMetadataApplyResult{AppliedPaths: make([]string, 0, len(plan.Changes))}
	for _, change := range plan.Changes {
		path := cleanAbsolutePath(change.Path)
		rel, relErr := filepath.Rel(root, path)
		if root == "" || path == "" || relErr != nil || !isVaultRelative(rel) || path == openPath {
			result.Skipped++
			continue
		}
		if _, err := m.saver.Save("", path, change.Source, change.ExpectedRevision); err != nil {
			result.Skipped++
			continue
		}
		result.AppliedPaths = append(result.AppliedPaths, path)
	}
	return result, nil
}

func isVaultRelative(path string) bool {
	return path != "." && path != ".." && !filepath.IsAbs(path) && !strings.HasPrefix(path, ".."+string(filepath.Separator))
}

func cleanAbsolutePath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}
