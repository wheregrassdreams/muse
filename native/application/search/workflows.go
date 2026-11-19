package search

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"muse/document"
	domainsearch "muse/search"
)

const StaleNavigationRefreshDelay = 120 * time.Millisecond

var ErrWorkflowUnavailable = errors.New("application/search: workflow capability is unavailable")

type QueryInput struct {
	Text       string
	Literal    bool
	ActivePath string
	// ActiveRevision identifies the editor snapshot represented by ActiveSource.
	// Callers with an editor revision should supply it; otherwise it is derived
	// from the payload before workflow state is updated.
	ActiveRevision string
	// ActiveSource is a transient payload passed to the vault query. Workflow
	// state and scheduler keys retain only ActiveRevision, never the document.
	ActiveSource string
}

func (q QueryInput) request() QueryRequest {
	revision := q.ActiveRevision
	if revision == "" && q.ActiveSource != "" {
		revision = string(document.RevisionOf(q.ActiveSource))
	}
	return QueryRequest{Text: q.Text, Literal: q.Literal, ActivePath: q.ActivePath, ActiveRevision: revision}
}

type SearchState struct {
	Token   uint64
	Query   string
	Literal bool
	Hits    []domainsearch.Hit
	Busy    bool
	Cost    time.Duration
}

// Port is the narrow vault capability used by interactive workspace search.
// Vault owns index lifetime and path confinement; this workflow owns when to
// query, whether a result is current, and how stale navigation is retried.
type Port interface {
	SearchState() SearchState
	Query(QueryInput) (uint64, error)
	ResolveHitPath(path string) (string, error)
	Refresh(path string) error
}

type QueryResult struct {
	Completed bool
	Token     uint64
	Query     string
	Literal   bool
}

type NavigationKind uint8

const (
	NavigationIgnored NavigationKind = iota
	NavigationAccepted
	NavigationRequery
	NavigationNoMatch
)

type NavigationOutcome struct {
	Kind NavigationKind
	Hit  domainsearch.Hit
	Path string
	Err  error
}

// Workflow owns interactive query scheduling and stale-hit navigation for a
// single UI search surface. It has no timer or toolkit dependency; callers
// supply frame time and poll typed results.
type Workflow struct {
	mu sync.Mutex

	scheduler QueryScheduler
	latest    QueryRequest
	hasLatest bool

	submitted      QueryRequest
	submittedToken uint64
	hasSubmitted   bool
	invalidated    bool
	pendingPath    string
}

func NewWorkflow() *Workflow { return &Workflow{} }

// ScheduleQuery records the latest input, coalesces a burst using measured
// vault search cost, and submits only when its application-owned deadline is
// due. The returned duration is a scheduling hint for the caller's next frame.
func (w *Workflow) ScheduleQuery(port Port, input QueryInput, now time.Time) (time.Duration, error) {
	if w == nil || port == nil {
		return 0, ErrWorkflowUnavailable
	}
	state := port.SearchState()
	request := input.request()
	w.mu.Lock()
	if w.pendingPath != "" && w.hasLatest &&
		(w.latest.Text != request.Text || w.latest.Literal != request.Literal) {
		w.pendingPath = ""
	}
	w.latest, w.hasLatest = request, true
	if strings.TrimSpace(input.Text) == "" {
		shouldClear := state.Query != "" || state.Literal != input.Literal || w.pendingPath != ""
		w.scheduler.Reset()
		w.hasSubmitted = false
		w.invalidated = false
		w.pendingPath = ""
		w.mu.Unlock()
		if shouldClear {
			_, err := port.Query(input)
			return 0, err
		}
		return 0, nil
	}
	wait := w.scheduler.Schedule(request, state.Cost, state.Busy, now)
	request, due := w.scheduler.Take(now)
	if !due {
		w.mu.Unlock()
		return wait, nil
	}
	input.Text, input.Literal, input.ActivePath, input.ActiveRevision = request.Text, request.Literal, request.ActivePath, request.ActiveRevision
	w.mu.Unlock()

	token, err := port.Query(input)
	w.mu.Lock()
	if err != nil {
		w.scheduler.Reset()
		w.hasSubmitted = false
	}
	if err == nil && w.hasLatest && w.latest == request {
		w.submitted = request
		w.submittedToken = token
		w.hasSubmitted = true
		w.invalidated = false
	}
	w.mu.Unlock()
	return 0, err
}

// ForceQuery invalidates current results and schedules the current input even
// when the query text is unchanged, such as after an index refresh.
func (w *Workflow) ForceQuery(now time.Time, wait time.Duration) {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.scheduler.Force(now, wait)
	w.hasSubmitted = false
	w.invalidated = true
	w.mu.Unlock()
}

// Reset clears the scheduler and pending navigation when the search surface
// or vault session is replaced.
func (w *Workflow) Reset() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.scheduler.Reset()
	w.latest = QueryRequest{}
	w.hasLatest = false
	w.submitted = QueryRequest{}
	w.submittedToken = 0
	w.hasSubmitted = false
	w.invalidated = false
	w.pendingPath = ""
	w.mu.Unlock()
}

// IsCurrentResult accepts only completions matching both the vault query
// token and the latest UI input. This also rejects old results during the
// debounce interval before a replacement query has started.
func (w *Workflow) IsCurrentResult(result QueryResult, state SearchState) bool {
	if w == nil || !result.Completed {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return !w.invalidated && w.hasLatest && w.hasSubmitted &&
		w.latest == w.submitted && result.Token == w.submittedToken &&
		state.Token == result.Token && state.Query == result.Query && state.Literal == result.Literal &&
		result.Query == w.latest.Text && result.Literal == w.latest.Literal
}

// Navigate validates a selected hit against the current document and durable
// repository. A stale hit refreshes its file and schedules the latest query;
// AcceptResult later resolves the same path to its new revision/offset.
func (w *Workflow) Navigate(port Port, repository document.Repository, input QueryInput, hit domainsearch.Hit, currentPath string, currentRevision document.Revision, now time.Time) NavigationOutcome {
	if w == nil || port == nil || repository == nil {
		return NavigationOutcome{Err: ErrWorkflowUnavailable}
	}
	w.mu.Lock()
	w.latest, w.hasLatest = input.request(), true
	w.pendingPath = ""
	w.mu.Unlock()
	input.ActiveSource = ""
	fullPath, err := port.ResolveHitPath(hit.Path)
	if err != nil {
		return NavigationOutcome{Err: err}
	}
	if filepath.Clean(fullPath) == filepath.Clean(currentPath) && currentRevision == document.Revision(hit.Revision) {
		w.mu.Lock()
		w.pendingPath = ""
		w.mu.Unlock()
		return NavigationOutcome{Kind: NavigationAccepted, Hit: hit, Path: fullPath}
	}
	note, err := repository.Read(fullPath)
	if err != nil {
		w.mu.Lock()
		w.pendingPath = ""
		w.mu.Unlock()
		return NavigationOutcome{Path: fullPath, Err: err}
	}
	if note.Revision == document.Revision(hit.Revision) {
		w.mu.Lock()
		w.pendingPath = ""
		w.mu.Unlock()
		return NavigationOutcome{Kind: NavigationAccepted, Hit: hit, Path: fullPath}
	}
	if err := port.Refresh(fullPath); err != nil {
		w.mu.Lock()
		w.pendingPath = ""
		w.mu.Unlock()
		return NavigationOutcome{Path: fullPath, Err: err}
	}
	w.mu.Lock()
	w.pendingPath = hit.Path
	w.scheduler.Force(now, StaleNavigationRefreshDelay)
	w.invalidated = true
	w.hasSubmitted = false
	w.mu.Unlock()
	return NavigationOutcome{Kind: NavigationRequery, Path: fullPath}
}

// AcceptResult returns the refreshed hit for a pending navigation only when
// the result belongs to the current submitted query. A missing path in a
// successful current result clears the pending request instead of leaving a
// navigation that could unexpectedly fire after a later search.
func (w *Workflow) AcceptResult(result QueryResult, state SearchState) NavigationOutcome {
	if !w.IsCurrentResult(result, state) {
		return NavigationOutcome{Kind: NavigationIgnored}
	}
	w.mu.Lock()
	path := w.pendingPath
	if path == "" {
		w.mu.Unlock()
		return NavigationOutcome{Kind: NavigationIgnored}
	}
	w.pendingPath = ""
	w.mu.Unlock()
	for _, hit := range state.Hits {
		if hit.Path == path {
			return NavigationOutcome{Kind: NavigationAccepted, Hit: hit}
		}
	}
	return NavigationOutcome{Kind: NavigationNoMatch, Path: path}
}

// IndexChanged requests a fresh query for the latest non-empty input. The
// frame loop remains the only place that submits it.
func (w *Workflow) IndexChanged(now time.Time) {
	if w == nil {
		return
	}
	w.mu.Lock()
	if w.hasLatest && strings.TrimSpace(w.latest.Text) != "" {
		w.scheduler.Force(now, 0)
		w.invalidated = true
		w.hasSubmitted = false
	}
	w.mu.Unlock()
}

// Close cancels any scheduled query and clears the vault's visible result set,
// while retaining the typed query input so a just-selected stale hit can be
// revalidated and re-searched after the panel closes.
func (w *Workflow) Close(port Port, input QueryInput) error {
	if w == nil || port == nil {
		return ErrWorkflowUnavailable
	}
	w.mu.Lock()
	w.latest, w.hasLatest = input.request(), true
	w.scheduler.Reset()
	w.hasSubmitted = false
	w.submittedToken = 0
	w.invalidated = w.pendingPath != ""
	w.mu.Unlock()
	input.Text = ""
	input.ActiveSource = ""
	_, err := port.Query(input)
	return err
}

// PendingNavigationInput returns the current query to the frame poller when a
// stale selected hit needs re-querying after the search panel has closed.
func (w *Workflow) PendingNavigationInput() (QueryInput, bool) {
	if w == nil {
		return QueryInput{}, false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return QueryInput{Text: w.latest.Text, Literal: w.latest.Literal, ActivePath: w.latest.ActivePath, ActiveRevision: w.latest.ActiveRevision},
		w.pendingPath != "" && w.hasLatest && strings.TrimSpace(w.latest.Text) != ""
}
