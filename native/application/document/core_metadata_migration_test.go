package document

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain "muse/document"
	"muse/events"
)

type migrationMetadata map[string]FileMetadata

func (m migrationMetadata) ReadMetadata(path string) (FileMetadata, error) {
	metadata, ok := m[path]
	if !ok {
		return FileMetadata{}, errors.New("metadata unavailable")
	}
	return metadata, nil
}

type migrationRepository struct{ sources map[string]string }

func (r *migrationRepository) Read(path string) (domain.Note, error) {
	source, ok := r.sources[path]
	if !ok {
		return domain.Note{}, errors.New("not found")
	}
	return domain.NewNote(path, source, domain.RevisionOf(source)), nil
}

func (r *migrationRepository) Save(path, source string, expected domain.Revision) (domain.Revision, error) {
	current, ok := r.sources[path]
	if !ok || domain.RevisionOf(current) != expected {
		return "", domain.ErrConflict
	}
	r.sources[path] = source
	return domain.RevisionOf(source), nil
}

type migrationRecovery struct{}

func (migrationRecovery) WriteDraft(string, string, string) error { return nil }
func (migrationRecovery) ListDrafts() ([]domain.Draft, error)     { return nil, nil }
func (migrationRecovery) ReadDraft(string) (domain.Draft, error)  { return domain.Draft{}, nil }
func (migrationRecovery) DeleteDraft(string) error                { return nil }

func TestCoreMetadataMigrationPreviewUsesInjectedMetadataAndSkipsOpenNote(t *testing.T) {
	root := t.TempDir()
	created := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	modified := time.Date(2026, 11, 15, 6, 7, 8, 0, time.UTC)
	path := filepath.Join(root, "legacy.md")
	openPath := filepath.Join(root, "open.md")
	explicitPath := filepath.Join(root, "explicit.md")
	metadata := migrationMetadata{
		path:         {CreatedAt: created, ModifiedAt: modified},
		explicitPath: {},
	}
	migration := NewCoreMetadataMigration(metadata, nil, nil, nil)
	plan, err := migration.Preview(root, openPath, []CoreMetadataCandidate{
		{Path: path, Source: "# Legacy\n", Revision: domain.RevisionOf("# Legacy\n")},
		{Path: openPath, Source: "# Open\n"},
		{Path: explicitPath, Source: "---\ncreated_at: \"\"\nupdated_at: \"\"\n---\n# Existing\n"},
		{Path: filepath.Join(root, "image.png"), Source: "not markdown"},
		{Path: filepath.Join(root, "bad.md"), Source: "---\ninvalid: [\n---\n"},
		{Path: filepath.Join(root, "..", "outside.md"), Source: "# Outside\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 2 || plan.CreatedFromBirth != 1 || plan.UpdatedFromMod != 1 || plan.OpenDocument != 1 || plan.Invalid != 2 {
		t.Fatalf("unexpected migration plan: %+v", plan)
	}
	if len(plan.Samples) != 2 || plan.Samples[0] != "legacy.md" {
		t.Fatalf("sample paths = %v", plan.Samples)
	}
	if plan.Changes[0].ExpectedRevision != domain.RevisionOf("# Legacy\n") {
		t.Fatalf("revision not retained: %+v", plan.Changes[0])
	}
	if got := plan.Changes[0].Source; !containsAll(got, created.Format(time.RFC3339Nano), modified.Format(time.RFC3339Nano)) {
		t.Fatalf("injected times missing from planned source: %q", got)
	}
	if got := plan.Changes[1].Source; !containsAll(got, "created_at: \"\"", "updated_at: \"\"") {
		t.Fatalf("explicit empty timestamps were changed: %q", got)
	}
}

func TestCoreMetadataMigrationPreviewDoesNotReadMetadataWhenDatesExist(t *testing.T) {
	root := t.TempDir()
	source := "---\ncreated_at: \"\"\nupdated_at: \"\"\n---\n# Note\n"
	migration := NewCoreMetadataMigration(migrationMetadata{}, nil, nil, nil)
	plan, err := migration.Preview(root, "", []CoreMetadataCandidate{{Path: filepath.Join(root, "note.md"), Source: source}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 1 || plan.Invalid != 0 {
		t.Fatalf("migration should only add missing core keys without metadata access: %+v", plan)
	}
}

func TestCoreMetadataMigrationLeavesUnavailableTimesEmpty(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "unknown.md")
	migration := NewCoreMetadataMigration(migrationMetadata{path: {}}, nil, nil, nil)
	plan, err := migration.Preview(root, "", []CoreMetadataCandidate{{Path: path, Source: "# Unknown\n"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 1 || plan.UnknownCreation != 1 || plan.UpdatedFromMod != 0 {
		t.Fatalf("unavailable times produced unexpected plan: %+v", plan)
	}
	if strings.Contains(plan.Changes[0].Source, "0001-01-01") {
		t.Fatalf("zero modification time was fabricated into source: %q", plan.Changes[0].Source)
	}
}

func TestCoreMetadataMigrationApplyRechecksOpenPathAndRevision(t *testing.T) {
	root := t.TempDir()
	activePath := filepath.Join(root, "active.md")
	stalePath := filepath.Join(root, "stale.md")
	applyPath := filepath.Join(root, "apply.md")
	outsidePath := filepath.Join(root, "..", "outside.md")
	activeSource, staleSource, applySource := "# active\n", "# stale\n", "# apply\n"
	repository := &migrationRepository{sources: map[string]string{
		activePath: activeSource,
		stalePath:  staleSource,
		applyPath:  applySource,
	}}
	dispatcher := events.New[domain.DocumentSaved]()
	defer dispatcher.Close()
	var published []domain.DocumentSaved
	dispatcher.Subscribe(func(event domain.DocumentSaved) { published = append(published, event) })
	migration := NewCoreMetadataMigration(nil, repository, migrationRecovery{}, dispatcher)
	plan := CoreMetadataPlan{Root: root, Changes: []CoreMetadataChange{
		{Path: activePath, Source: "# migrated active\n", ExpectedRevision: domain.RevisionOf(activeSource)},
		{Path: stalePath, Source: "# migrated stale\n", ExpectedRevision: domain.RevisionOf(staleSource)},
		{Path: applyPath, Source: "# migrated apply\n", ExpectedRevision: domain.RevisionOf(applySource)},
	}}
	// Simulate an external edit after preview. Its stale plan must not overwrite it.
	repository.sources[stalePath] = "# external edit\n"
	result, err := migration.Apply(plan, activePath)
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped != 2 || len(result.AppliedPaths) != 1 || result.AppliedPaths[0] != applyPath {
		t.Fatalf("apply result = %+v", result)
	}
	if repository.sources[activePath] != activeSource || repository.sources[stalePath] != "# external edit\n" {
		t.Fatalf("protected sources were overwritten: %+v", repository.sources)
	}
	if repository.sources[applyPath] != "# migrated apply\n" || len(published) != 1 || published[0].Path != applyPath {
		t.Fatalf("successful save/event mismatch: source=%q events=%+v", repository.sources[applyPath], published)
	}
	outsidePlan := CoreMetadataPlan{Root: root, Changes: []CoreMetadataChange{{Path: outsidePath, Source: "# escaped\n", ExpectedRevision: domain.RevisionOf("# escaped\n")}}}
	outsideResult, err := migration.Apply(outsidePlan, "")
	if err != nil || outsideResult.Skipped != 1 || len(outsideResult.AppliedPaths) != 0 {
		t.Fatalf("out-of-root plan was not rejected: result=%+v error=%v", outsideResult, err)
	}
}

func containsAll(source string, fragments ...string) bool {
	for _, fragment := range fragments {
		if !strings.Contains(source, fragment) {
			return false
		}
	}
	return true
}
