// Package notestore provides revision-checked document writes and local recovery data.
package notestore

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const historyLimit = 20

// ErrConflict indicates that the on-disk document no longer has the revision
// supplied to Save.
var ErrConflict = errors.New("document revision conflict")

// ConflictError describes a failed revision check. Actual is empty when the
// file does not exist; Expected is empty when Save expected a new file.
type ConflictError struct {
	Path     string
	Expected string
	Actual   string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%s: %v", e.Path, ErrConflict)
}

func (e *ConflictError) Unwrap() error { return ErrConflict }

// Document is the text and content revision of a stored document or recovery
// snapshot.
type Document struct {
	Path     string
	Text     string
	Revision string
}

// Draft is an autosave recovery record. Key is caller-controlled and may be an
// untitled-document key; UpdatedAt is UTC.
type Draft struct {
	Key       string    `json:"key"`
	Path      string    `json:"path"`
	Text      string    `json:"text"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// HistoryEntry describes a saved pre-write version.
type HistoryEntry struct {
	ID        string
	Path      string
	Revision  string
	CreatedAt time.Time
}

// Store persists drafts and history below dir. It does not own or change the
// location of the original documents.
type Store struct {
	dir string
	mu  sync.Mutex
}

// New creates a Store rooted at dir. The directory is typically the
// application's config/recovery directory.
func New(dir string) *Store { return &Store{dir: dir} }

// Revision returns the SHA-256 revision of text.
func Revision(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// Read returns a document and its current on-disk revision.
func (s *Store) Read(path string) (Document, error) {
	full, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return Document{}, err
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return Document{}, err
	}
	text := string(data)
	return Document{Path: path, Text: text, Revision: Revision(text)}, nil
}

// Save atomically writes text only if the current disk revision equals
// expectedRevision. Use an empty expected revision only when the path is
// expected not to exist. Before replacing an existing file, Save stores its
// complete contents as a history entry.
func (s *Store) Save(path, text, expectedRevision string) (string, error) {
	full, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := readCurrent(full)
	if err != nil {
		return "", err
	}
	if current.revision != expectedRevision {
		return "", &ConflictError{Path: path, Expected: expectedRevision, Actual: current.revision}
	}
	if current.exists && current.text == text {
		return current.revision, nil
	}
	if current.exists {
		if err := s.addHistory(path, current.text, current.revision); err != nil {
			return "", err
		}
	}
	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(full); statErr == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", statErr
	}
	if err := atomicWriteChecked(full, []byte(text), mode, func() error {
		// The temporary file is already fully written and synced. Recheck just
		// before rename so edits observed during snapshot/write cannot be lost.
		latest, err := readCurrent(full)
		if err != nil {
			return err
		}
		if latest.revision != expectedRevision {
			return &ConflictError{Path: path, Expected: expectedRevision, Actual: latest.revision}
		}
		return nil
	}); err != nil {
		return "", err
	}
	return Revision(text), nil
}

// WriteDraft atomically stores or replaces a recovery draft. Existing drafts
// remain untouched if a write fails.
func (s *Store) WriteDraft(key, path, text string) error {
	if key == "" {
		return errors.New("draft key must not be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	draft := Draft{Key: key, Path: path, Text: text, UpdatedAt: time.Now().UTC()}
	data, err := json.Marshal(draft)
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.dir, "drafts", Revision(key)+".json"), data, 0o600)
}

// ListDrafts returns all draft records, newest first.
func (s *Store) ListDrafts() ([]Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(s.dir, "drafts"))
	if errors.Is(err, os.ErrNotExist) {
		return []Draft{}, nil
	}
	if err != nil {
		return nil, err
	}
	drafts := make([]Draft, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var draft Draft
		if err := readJSON(filepath.Join(s.dir, "drafts", entry.Name()), &draft); err != nil {
			return nil, err
		}
		if draft.Key == "" || Revision(draft.Key)+".json" != entry.Name() {
			return nil, fmt.Errorf("invalid draft metadata: %s", entry.Name())
		}
		drafts = append(drafts, draft)
	}
	sort.Slice(drafts, func(i, j int) bool { return drafts[i].UpdatedAt.After(drafts[j].UpdatedAt) })
	return drafts, nil
}

// ReadDraft reads one draft by its caller-supplied key.
func (s *Store) ReadDraft(key string) (Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var draft Draft
	err := readJSON(filepath.Join(s.dir, "drafts", Revision(key)+".json"), &draft)
	if err == nil && draft.Key != key {
		return Draft{}, os.ErrNotExist
	}
	return draft, err
}

// DeleteDraft removes a draft. A missing key returns os.ErrNotExist.
func (s *Store) DeleteDraft(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.Remove(filepath.Join(s.dir, "drafts", Revision(key)+".json"))
}

// ListHistory returns saved versions for path, newest first.
func (s *Store) ListHistory(path string) ([]HistoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, canonical, err := s.historyDir(path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []HistoryEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]HistoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var item historyRecord
		if err := readJSON(filepath.Join(dir, entry.Name()), &item); err != nil {
			return nil, err
		}
		if item.ID+".json" != entry.Name() || canonicalPath(item.Path) != canonical {
			return nil, fmt.Errorf("invalid history metadata: %s", entry.Name())
		}
		result = append(result, HistoryEntry{ID: item.ID, Path: item.Path, Revision: item.Revision, CreatedAt: item.CreatedAt})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

// ReadHistory reads a saved version by original path and history ID.
func (s *Store) ReadHistory(path, id string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validID(id) {
		return Document{}, os.ErrNotExist
	}
	dir, canonical, err := s.historyDir(path)
	if err != nil {
		return Document{}, err
	}
	var item historyRecord
	err = readJSON(filepath.Join(dir, id+".json"), &item)
	if err != nil {
		return Document{}, err
	}
	if item.ID != id || canonicalPath(item.Path) != canonical || Revision(item.Text) != item.Revision {
		return Document{}, errors.New("invalid history record")
	}
	return Document{Path: item.Path, Text: item.Text, Revision: item.Revision}, nil
}

// DeleteHistory removes one saved version. A missing ID returns os.ErrNotExist.
func (s *Store) DeleteHistory(path, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validID(id) {
		return os.ErrNotExist
	}
	dir, canonical, err := s.historyDir(path)
	if err != nil {
		return err
	}
	file := filepath.Join(dir, id+".json")
	var item historyRecord
	if err := readJSON(file, &item); err != nil {
		return err
	}
	if canonicalPath(item.Path) != canonical || item.ID != id {
		return os.ErrNotExist
	}
	return os.Remove(file)
}

type currentDocument struct {
	text     string
	revision string
	exists   bool
}

func readCurrent(path string) (currentDocument, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return currentDocument{}, nil
	}
	if err != nil {
		return currentDocument{}, err
	}
	text := string(data)
	return currentDocument{text: text, revision: Revision(text), exists: true}, nil
}

type historyRecord struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	Text      string    `json:"text"`
	Revision  string    `json:"revision"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) addHistory(path, text, revision string) error {
	dir, _, err := s.historyDir(path)
	if err != nil {
		return err
	}
	id, err := newID()
	if err != nil {
		return err
	}
	item := historyRecord{ID: id, Path: path, Text: text, Revision: revision, CreatedAt: time.Now().UTC()}
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(dir, id+".json"), data, 0o600); err != nil {
		return err
	}
	return s.pruneHistory(dir)
}

func (s *Store) pruneHistory(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	items := make([]historyRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var item historyRecord
		if err := readJSON(filepath.Join(dir, entry.Name()), &item); err != nil {
			return err
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	if len(items) <= historyLimit {
		return nil
	}
	for _, item := range items[historyLimit:] {
		if err := os.Remove(filepath.Join(dir, item.ID+".json")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (s *Store) historyDir(path string) (dir, canonical string, err error) {
	canonical, err = filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", "", err
	}
	return filepath.Join(s.dir, "history", Revision(canonical)), canonical, nil
}

func canonicalPath(path string) string {
	full, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return ""
	}
	return full
}

func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func newID() (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func readJSON(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	return atomicWriteChecked(path, data, mode, nil)
}

func atomicWriteChecked(path string, data []byte, mode os.FileMode, beforeRename func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".notestore-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if beforeRename != nil {
		if err := beforeRename(); err != nil {
			return err
		}
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
