package document

import (
	"errors"
	"time"
)

// Repository is the persistence port owned by the document module. Adapters
// must compare the expected revision before replacing source.
type Repository interface {
	Read(path string) (Note, error)
	Save(path, source string, expected Revision) (Revision, error)
}

type Draft struct {
	Key       string    `json:"key"`
	Path      string    `json:"path"`
	Text      string    `json:"text"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type HistoryEntry struct {
	ID        string
	Path      string
	Revision  Revision
	CreatedAt time.Time
}

type RecoveryRepository interface {
	WriteDraft(key, path, source string) error
	ListDrafts() ([]Draft, error)
	ReadDraft(key string) (Draft, error)
	DeleteDraft(key string) error
}

type HistoryRepository interface {
	ListHistory(path string) ([]HistoryEntry, error)
	ReadHistory(path, id string) (Note, error)
	DeleteHistory(path, id string) error
}

// ErrConflict means a save's expected disk revision no longer matches the
// current file. Adapters should wrap this sentinel in their path-rich error.
var ErrConflict = errors.New("document: revision conflict")

// DocumentSaved is emitted by the application after a successful durable save.
// Path is the current identity, and the event intentionally carries no source.
type DocumentSaved struct {
	Path     string
	Revision Revision
}
