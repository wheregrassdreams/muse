package nativeui

import (
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"muse/document"
)

type countingDocumentRepository struct {
	document.Repository
	saves atomic.Int32
}

func (r *countingDocumentRepository) Save(path, source string, expected document.Revision) (document.Revision, error) {
	r.saves.Add(1)
	return r.Repository.Save(path, source, expected)
}

func TestAutosavePersistsEditAndTimestampInOneWrite(t *testing.T) {
	a, tester, path := runtimeFixture(t)
	repository := &countingDocumentRepository{Repository: a.documentRepository}
	a.SetDocumentRepositories(repository, a.documentRecovery, a.documentHistory)
	if err := a.noteEditor().ReplaceUser("one edit"); err != nil {
		t.Fatal(err)
	}
	expireAutosave(a)
	waitDocument(t, tester, func() bool {
		state := a.noteEditor().PersistenceState()
		return !state.Saving && state.AutosaveDue.IsZero() && state.MetadataDue.IsZero() && !a.noteEditor().Snapshot().Dirty
	})
	if got := repository.saves.Load(); got != 1 {
		t.Fatalf("durable save count = %d, want exactly one", got)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "updated_at:") || !strings.HasSuffix(string(source), "\n\none edit") {
		t.Fatalf("saved source does not contain the edit and its timestamp: %q", source)
	}
}
