package document

import (
	"errors"
	"io/fs"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	domain "muse/document"
	"muse/document/frontmatter"
	"muse/events"
)

const (
	AutosaveDelay       = 750 * time.Millisecond
	MetadataUpdateDelay = time.Second
)

// Session owns the mutable persistence lifecycle for one open editing
// session. It deliberately contains no editor buffer, widget, focus, or view
// state. A UI adapter observes Snapshot and drives Tick/Save when appropriate.
type Session struct {
	mu          sync.Mutex
	saveMu      sync.Mutex
	token       atomic.Uint64
	repository  domain.Repository
	recovery    domain.RecoveryRepository
	history     domain.HistoryRepository
	saver       *Saver
	state       SessionState
	results     chan SaveResult
	closed      bool
	lastSave    SaveResult
	hasLastSave bool
}

// SessionState is an immutable copy of the persistence state needed by a
// presentation adapter. Dialog visibility and scroll positions stay in UI.
type SessionState struct {
	Path             string
	Generation       uint64
	Revision         domain.Revision
	BaseSource       string
	AutosaveDue      time.Time
	MetadataDue      time.Time
	Saving           bool
	InFlight         SaveRequest
	HasInFlight      bool
	DeferredExternal domain.Note
	HasDeferred      bool
	DraftKey         string
	ConflictPath     string
}

type SaveRequest struct {
	Generation uint64
	Key        string
	Path       string
	Source     string
	Expected   domain.Revision
	Conflict   bool
}

type SaveResult struct {
	Request SaveRequest
	Note    domain.Note
	Err     error
}

func NewSession(repository domain.Repository, recovery domain.RecoveryRepository, history domain.HistoryRepository, dispatcher *events.Dispatcher[domain.DocumentSaved]) *Session {
	return &Session{
		repository: repository,
		recovery:   recovery,
		history:    history,
		saver:      NewSaver(repository, recovery, dispatcher),
		results:    make(chan SaveResult, 4),
		state:      SessionState{DraftKey: newDraftKey()},
	}
}

func newDraftKey() string { return "untitled-" + time.Now().Format("20060102T150405.000000000") }

// UpdateManagedTimestamp applies the document metadata policy used when a
// content edit needs an updated_at field. Callers apply the returned source
// through their normal editor transaction.
func UpdateManagedTimestamp(source string, now time.Time) (string, error) {
	next, err := frontmatter.EnsureCore(source)
	if err != nil {
		return "", err
	}
	return frontmatter.SetTyped(next, "updated_at", now.UTC().Format(time.RFC3339Nano), "text")
}

// EnsureCreatedAt preserves an authored created_at value or adds one using
// the supplied fallback clock value.
func EnsureCreatedAt(source string, fallback time.Time) (string, string, error) {
	core, err := frontmatter.EnsureCore(source)
	if err != nil {
		return source, "", err
	}
	for _, field := range frontmatter.Parse(core).Fields {
		if field.Key == "created_at" && strings.TrimSpace(field.Value) != "" {
			return core, field.Value, nil
		}
	}
	stamp := fallback.UTC().Format(time.RFC3339Nano)
	updated, err := frontmatter.SetTyped(core, "created_at", stamp, "text")
	return updated, stamp, err
}

func (s *Session) Snapshot() SessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// DeferExternal retains the newest disk observation while a save is in
// flight. The workflow applies it only after the exact submitted write has
// been acknowledged or rejected.
func (s *Session) DeferExternal(note domain.Note) bool {
	if s == nil || note.Path == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || !s.state.Saving || s.state.Path != note.Path {
		return false
	}
	if s.state.HasInFlight && note.Source == s.state.InFlight.Source {
		return true
	}
	if s.state.HasDeferred && s.state.DeferredExternal.Revision == note.Revision {
		return true
	}
	s.state.DeferredExternal = note
	s.state.HasDeferred = true
	return true
}

// TakeDeferredExternal returns a disk observation once no save is in flight.
func (s *Session) TakeDeferredExternal() (domain.Note, bool) {
	if s == nil {
		return domain.Note{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.state.Saving || !s.state.HasDeferred {
		return domain.Note{}, false
	}
	note := s.state.DeferredExternal
	s.state.DeferredExternal = domain.Note{}
	s.state.HasDeferred = false
	return note, true
}

func (s *Session) Results() <-chan SaveResult { return s.results }

// Close invalidates pending save work and clears session deadlines. The
// repositories remain owned by the composition root and are not closed here.
func (s *Session) Close() {
	if s == nil {
		return
	}
	generation := s.token.Add(1)
	s.mu.Lock()
	s.closed = true
	s.state.Generation = generation
	s.state.Path = ""
	s.state.BaseSource = ""
	s.state.ConflictPath = ""
	s.state.Saving = false
	s.state.InFlight = SaveRequest{}
	s.state.HasInFlight = false
	s.state.DeferredExternal = domain.Note{}
	s.state.HasDeferred = false
	s.state.AutosaveDue = time.Time{}
	s.state.MetadataDue = time.Time{}
	s.mu.Unlock()
	for {
		select {
		case <-s.results:
		default:
			return
		}
	}
}

// StartSave executes a request asynchronously and publishes its completion on
// the session-owned result stream. The presentation adapter applies results
// with CompleteSave on its event loop.
func (s *Session) StartSave(request SaveRequest) {
	if s == nil {
		return
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return
	}
	go func() {
		result := s.Save(request)
		s.mu.Lock()
		publish := !s.closed && request.Generation == s.state.Generation
		s.mu.Unlock()
		if publish {
			s.results <- result
		}
	}()
}

// Load reads a durable note through the configured repository.
func (s *Session) Load(path string) (domain.Note, error) {
	if s == nil {
		return Read(nil, path)
	}
	return Read(s.repository, path)
}

// Read performs a read-only document request through the configured
// repository. It is also used by presentation actions that must inspect disk
// state before deciding whether to replace or merge an open buffer.
func Read(repository domain.Repository, path string) (domain.Note, error) {
	if repository == nil {
		return domain.Note{}, errors.New("document repository is not configured")
	}
	return repository.Read(path)
}

// DestinationRevision returns the current revision when saving over the open
// path, or reads the destination revision for a Save As operation.
func (s *Session) DestinationRevision(path, currentPath string, expected domain.Revision) (domain.Revision, error) {
	if path == currentPath {
		return expected, nil
	}
	if s == nil || s.repository == nil {
		return "", errors.New("document repository is not configured")
	}
	note, err := s.repository.Read(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return note.Revision, err
}

// ListDrafts returns recoverable editing snapshots for the recovery dialog.
func (s *Session) ListDrafts() ([]domain.Draft, error) {
	if s == nil || s.recovery == nil {
		return nil, errors.New("document recovery repository is not configured")
	}
	return s.recovery.ListDrafts()
}

func (s *Session) ReadDraft(key string) (domain.Draft, error) {
	if s == nil || s.recovery == nil {
		return domain.Draft{}, errors.New("document recovery repository is not configured")
	}
	return s.recovery.ReadDraft(key)
}

func (s *Session) DeleteDraft(key string) error {
	if s == nil || s.recovery == nil {
		return errors.New("document recovery repository is not configured")
	}
	return s.recovery.DeleteDraft(key)
}

// PreserveDraft writes the active source to recovery storage and returns the
// stable key used by the recovery list. Untitled documents use the session key.
func (s *Session) PreserveDraft(path, source string) (string, error) {
	if s == nil || s.recovery == nil {
		return "", errors.New("document recovery repository is not configured")
	}
	key := path
	if key == "" {
		key = s.Snapshot().DraftKey
	}
	if err := s.recovery.WriteDraft(key, path, source); err != nil {
		return "", err
	}
	return key, nil
}

func (s *Session) ListHistory(path string) ([]domain.HistoryEntry, error) {
	if s == nil || s.history == nil {
		return nil, errors.New("document history repository is not configured")
	}
	return s.history.ListHistory(path)
}

func (s *Session) ReadHistory(path, id string) (domain.Note, error) {
	if s == nil || s.history == nil {
		return domain.Note{}, errors.New("document history repository is not configured")
	}
	return s.history.ReadHistory(path, id)
}

// Open starts a new generation, invalidating any save completion that belongs
// to the document that was previously active.
func (s *Session) Open(path, source string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.state.Generation = s.token.Add(1)
	s.state.Path = path
	s.state.Saving = false
	s.state.InFlight = SaveRequest{}
	s.state.HasInFlight = false
	s.state.DeferredExternal = domain.Note{}
	s.state.HasDeferred = false
	if path == "" {
		s.state.DraftKey = newDraftKey()
	}
	s.state.Revision = domain.RevisionOf(source)
	s.state.BaseSource = source
	s.state.AutosaveDue = time.Time{}
	s.state.MetadataDue = time.Time{}
	s.state.ConflictPath = ""
}

// Changed records a user source edit and schedules both autosave and the
// managed timestamp update.
func (s *Session) Changed(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.AutosaveDue = now.Add(AutosaveDelay)
	s.state.MetadataDue = now.Add(MetadataUpdateDelay)
}

// MetadataChanged records a system metadata patch without moving an existing
// autosave deadline. If an earlier save already claimed its deadline, queue a
// follow-up so the metadata patch is persisted after that write completes.
func (s *Session) MetadataChanged(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.AutosaveDue.IsZero() {
		s.state.AutosaveDue = now.Add(AutosaveDelay)
	}
}

// Rebase replaces the durable comparison baseline after an external edit or
// a successful merge. It only applies to the currently open path.
func (s *Session) Rebase(path string, revision domain.Revision, source string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Path != path {
		return false
	}
	s.state.Revision, s.state.BaseSource = revision, source
	s.state.ConflictPath = ""
	return true
}

func (s *Session) SetConflict(path string) {
	s.mu.Lock()
	s.state.ConflictPath = path
	s.mu.Unlock()
}

func (s *Session) ClearAutosave() {
	s.mu.Lock()
	s.state.AutosaveDue = time.Time{}
	s.state.MetadataDue = time.Time{}
	s.mu.Unlock()
}

// BeginSave atomically claims the pending autosave after both the autosave
// deadline and any managed-metadata patch have settled. The caller can then
// run Save in a worker goroutine.
func (s *Session) BeginSave(path, source string, dirty bool, now time.Time) (SaveRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Saving || !s.state.MetadataDue.IsZero() || s.state.AutosaveDue.IsZero() || now.Before(s.state.AutosaveDue) || !dirty {
		return SaveRequest{}, false
	}
	key := path
	if key == "" {
		key = s.state.DraftKey
	}
	request := SaveRequest{
		Generation: s.state.Generation,
		Key:        key,
		Path:       path,
		Source:     source,
		Expected:   s.state.Revision,
		Conflict:   s.state.ConflictPath != "",
	}
	s.state.Saving = true
	s.state.InFlight = request
	s.state.HasInFlight = true
	s.state.AutosaveDue = time.Time{}
	return request, true
}

// Save performs persistence for one claimed request. Generation checks keep a
// stale worker from overwriting a newer document session.
func (s *Session) Save(request SaveRequest) SaveResult {
	if s == nil || s.saver == nil {
		return SaveResult{Request: request, Err: errors.New("document saver is not configured")}
	}
	if request.Key != "" && s.recovery == nil {
		return SaveResult{Request: request, Err: errors.New("document recovery repository is not configured")}
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.lastSave = SaveResult{}
	s.hasLastSave = false
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return SaveResult{Request: request}
	}
	if s.token.Load() != request.Generation {
		return SaveResult{Request: request}
	}
	var note domain.Note
	var err error
	if request.Path != "" && !request.Conflict {
		note, err = s.saver.Save(request.Key, request.Path, request.Source, request.Expected)
	} else if request.Key != "" {
		err = s.recovery.WriteDraft(request.Key, request.Path, request.Source)
	}
	result := SaveResult{Request: request, Note: note, Err: err}
	s.lastSave, s.hasLastSave = result, true
	return result
}

// InvalidatePendingSave serializes a document transition and removal of its
// recovery draft with the save commit gate, then invalidates all claimed work.
// It returns the completed result for an active save that committed first.
func (s *Session) InvalidatePendingSave(draftKey string) (SaveResult, bool, error) {
	if s == nil {
		return SaveResult{}, false, errors.New("document session is unavailable")
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	var completed SaveResult
	committed := false
	if s.hasLastSave && s.lastSave.Request.Path == s.state.Path {
		completed = s.lastSave
		committed = completed.Err == nil && completed.Note.Path != ""
	}
	s.state.Generation = s.token.Add(1)
	s.state.Saving = false
	s.state.InFlight = SaveRequest{}
	s.state.HasInFlight = false
	s.state.DeferredExternal = domain.Note{}
	s.state.HasDeferred = false
	s.state.AutosaveDue = time.Time{}
	s.state.MetadataDue = time.Time{}
	s.state.ConflictPath = ""
	s.lastSave = SaveResult{}
	s.hasLastSave = false
	if draftKey != "" && s.recovery != nil {
		if err := s.recovery.DeleteDraft(draftKey); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return completed, committed, err
		}
	}
	return completed, committed, nil
}

// SaveNow serializes an explicit save with any autosave already in flight.
func (s *Session) SaveNow(key, path, source string, expected domain.Revision) (domain.Note, error) {
	if s == nil || s.saver == nil {
		return domain.Note{}, errors.New("document saver is not configured")
	}
	if key != "" && s.recovery == nil {
		return domain.Note{}, errors.New("document recovery repository is not configured")
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return domain.Note{}, errors.New("document session is closed")
	}
	note, err := s.saver.Save(key, path, source, expected)
	if errors.Is(err, domain.ErrConflict) {
		s.mu.Lock()
		if s.state.Path == path {
			s.state.ConflictPath = path
		}
		s.mu.Unlock()
	}
	if err == nil {
		s.mu.Lock()
		if s.state.Path == path {
			s.state.Revision = note.Revision
			s.state.BaseSource = note.Source
			s.state.AutosaveDue = time.Time{}
			s.state.MetadataDue = time.Time{}
			s.state.ConflictPath = ""
		}
		s.mu.Unlock()
	}
	return note, err
}

func (s *Session) ClearMetadataDeadline() {
	s.mu.Lock()
	s.state.MetadataDue = time.Time{}
	s.mu.Unlock()
}

// CompleteSave applies a worker result only if its generation is still
// current. It returns whether the result belongs to the active session.
func (s *Session) CompleteSave(result SaveResult, currentPath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if result.Request.Generation != s.state.Generation || result.Request.Path != currentPath ||
		!s.state.HasInFlight || result.Request != s.state.InFlight {
		return false
	}
	s.state.Saving = false
	s.state.InFlight = SaveRequest{}
	s.state.HasInFlight = false
	if result.Err != nil {
		if errors.Is(result.Err, domain.ErrConflict) && result.Request.Path != "" {
			s.state.ConflictPath = result.Request.Path
		} else {
			// Transient IO failures must not leave a dirty document permanently
			// unscheduled. Retry with the ordinary debounce interval; CAS conflicts
			// require user resolution and are intentionally paused above.
			s.state.AutosaveDue = time.Now().Add(AutosaveDelay)
		}
		return true
	}
	if result.Note.Path != "" && result.Note.Revision != "" {
		s.state.Revision = result.Note.Revision
		s.state.BaseSource = result.Note.Source
	}
	// Accept the persistence result even when editing continued while the write
	// was in flight. The result still advances the durable baseline and must
	// release deferred filesystem observations; the editor adapter separately
	// keeps the newer buffer dirty when currentSource differs.
	return true
}
