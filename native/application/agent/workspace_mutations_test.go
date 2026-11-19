package agent

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	vaultfileapp "muse/application/vault/files"
	"muse/document"
	"muse/events"
	"muse/platform/filesystem/documentstore"
	vaultfs "muse/platform/filesystem/vaultfiles"
)

func TestWorkspaceMutationServiceSaveUsesRevisionAndPublishesOnlySuccess(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := documentstore.New(filepath.Join(t.TempDir(), "recovery"))
	changed := events.New[document.DocumentSaved]()
	var saved []document.DocumentSaved
	changed.Subscribe(func(event document.DocumentSaved) { saved = append(saved, event) })
	// A nil recovery repository proves that the Saver intentionally uses an
	// empty draft key for the assistant's direct, revision-checked save.
	service := NewWorkspaceMutationService(store, nil, changed, nil, nil)

	revision, err := service.DestinationRevision(path)
	if err != nil || revision != string(document.RevisionOf("before")) {
		t.Fatalf("DestinationRevision = %q, %v", revision, err)
	}
	if missing, err := service.DestinationRevision(filepath.Join(root, "new.md")); err != nil || missing != "" {
		t.Fatalf("missing destination revision = %q, %v", missing, err)
	}
	if err := service.SaveDocument(path, "stale", "wrong-revision"); err == nil {
		t.Fatal("stale revision saved")
	}
	if len(saved) != 0 {
		t.Fatalf("failed save published event: %+v", saved)
	}
	if err := service.SaveDocument(path, "after", revision); err != nil {
		t.Fatalf("SaveDocument = %v", err)
	}
	if len(saved) != 1 || saved[0].Path != path || saved[0].Revision != document.RevisionOf("after") {
		t.Fatalf("successful save events = %+v", saved)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "after" {
		t.Fatalf("saved source = %q, %v", data, err)
	}
}

func TestWorkspaceMutationServiceScopesFileOperationsAndPublishesResults(t *testing.T) {
	root := t.TempDir()
	trashRoot := t.TempDir()
	trash := func(path string) error {
		if filepath.Base(path) == "bad.md" {
			return errors.New("trash failed")
		}
		return os.Remove(path)
	}
	changed := events.New[WorkspaceMutation]()
	var mutations []WorkspaceMutation
	changed.Subscribe(func(event WorkspaceMutation) { mutations = append(mutations, event) })
	operations := vaultfileapp.NewFileOperations(vaultfs.New(trash))
	service := NewWorkspaceMutationService(nil, nil, nil, func(vaultRoot string) *vaultfileapp.FileUseCases {
		return vaultfileapp.NewFileUseCases(vaultRoot, operations, nil)
	}, changed)

	from := filepath.Join(root, "from.md")
	if err := os.WriteFile(from, []byte("note"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "archive"), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "archive", "moved.md")
	if _, err := service.MoveDocument(root, from, target); err != nil {
		t.Fatalf("MoveDocument = %v", err)
	}
	if len(mutations) != 1 || mutations[0].Kind != WorkspaceDocumentMoved || mutations[0].Moves[0].Target != target {
		t.Fatalf("move event = %+v", mutations)
	}

	first, second := filepath.Join(root, "first.md"), filepath.Join(root, "bad.md")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte(path), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.TrashDocuments(root, []string{first, second}); err == nil {
		t.Fatal("partial trash failure was hidden")
	}
	if len(mutations) != 2 || mutations[1].Kind != WorkspaceDocumentsTrashed || len(mutations[1].Trashed) != 1 || mutations[1].Trashed[0] != first {
		t.Fatalf("partial trash event = %+v", mutations)
	}

	outside := filepath.Join(trashRoot, "outside")
	if err := service.CreateFolder(root, outside); err == nil {
		t.Fatal("CreateFolder accepted a path outside the vault")
	}
	folder := filepath.Join(root, "created")
	if err := service.CreateFolder(root, folder); err != nil {
		t.Fatalf("CreateFolder = %v", err)
	}
	if len(mutations) != 3 || mutations[2].Kind != WorkspaceFolderCreated || mutations[2].Path != folder {
		t.Fatalf("create event = %+v", mutations)
	}
}
