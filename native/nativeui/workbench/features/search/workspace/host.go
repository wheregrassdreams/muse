package workspace

import (
	"time"

	"github.com/egoist/mygo/ui"
	applicationsearch "muse/application/search"
	applicationvault "muse/application/vault"
	"muse/document"
	domainsearch "muse/search"
)

// InputSnapshot is the feature's complete query input. ActiveSource is only a
// transient query payload; Workflow stores ActiveRevision as its identity.
type InputSnapshot struct {
	Query          string
	Literal        bool
	ActivePath     string
	ActiveRevision string
	ActiveSource   string
}

func (s InputSnapshot) queryInput() applicationsearch.QueryInput {
	return applicationsearch.QueryInput{
		Text: s.Query, Literal: s.Literal, ActivePath: s.ActivePath,
		ActiveRevision: s.ActiveRevision, ActiveSource: s.ActiveSource,
	}
}

// SearchSnapshot is a detached, backend-neutral search result state supplied
// by the application port.
type SearchSnapshot struct {
	Token   uint64
	Query   string
	Literal bool
	Hits    []domainsearch.Hit
	Busy    bool
	Cost    time.Duration
}

// ResultSnapshot is the typed completion event consumed by this feature.
type ResultSnapshot struct {
	Completed bool
	Token     uint64
	Query     string
	Literal   bool
	Search    SearchSnapshot
}

type OutcomeKind uint8

const (
	OutcomeIgnored OutcomeKind = iota
	OutcomeResultsUpdated
	OutcomeNavigate
	OutcomeNoMatch
)

// Outcome is an intent for the root App/router to present or execute. The
// feature never captures App, opens a document, or changes editor focus.
type Outcome struct {
	Kind   OutcomeKind
	Search SearchSnapshot
	Hit    domainsearch.Hit
	Path   string
	Err    error
}

// Host owns workspace-search workflow policy for one search surface.
type Host struct {
	view     *Handle
	workflow *applicationsearch.Workflow
}

func NewHost(view *Handle) *Host {
	if view == nil {
		view = NewHandle()
	}
	return &Host{view: view, workflow: applicationsearch.NewWorkflow()}
}

func (h *Host) Open() {
	if h != nil {
		h.view.Open()
	}
}

func (h *Host) ClosePanel() {
	if h != nil {
		h.view.Close()
	}
}

func (h *Host) IsOpen() bool { return h != nil && h.view.IsOpen() }

func (h *Host) Query() string {
	if h == nil {
		return ""
	}
	return h.view.Query()
}

func (h *Host) Mode() Mode {
	if h == nil {
		return FullText
	}
	return h.view.Mode()
}

func (h *Host) Selection() int {
	if h == nil {
		return 0
	}
	return h.view.Selection()
}

func (h *Host) Apply(action Action) {
	if h != nil {
		h.view.Apply(action)
	}
}

func (h *Host) FocusInput(c *ui.Context) {
	if h != nil {
		h.view.FocusInput(c)
	}
}

func (h *Host) VisibleResults() (int, int) {
	if h == nil {
		return 0, -1
	}
	return h.view.VisibleResults()
}

// Render delegates toolkit state and drawing to the feature-owned Handle and
// returns typed actions to the caller for application routing.
func (h *Host) Render(c *ui.Context, view View, dispatch func(Action)) {
	if h != nil {
		h.view.Render(c, view, dispatch)
	}
}

func (h *Host) ScheduleQuery(port applicationsearch.Port, input InputSnapshot, now time.Time) (time.Duration, error) {
	if h == nil || h.workflow == nil {
		return 0, applicationsearch.ErrWorkflowUnavailable
	}
	return h.workflow.ScheduleQuery(port, input.queryInput(), now)
}

func (h *Host) ForceQuery(now time.Time, wait time.Duration) {
	if h != nil && h.workflow != nil {
		h.workflow.ForceQuery(now, wait)
	}
}

func (h *Host) IndexChanged(now time.Time) {
	if h != nil && h.workflow != nil {
		h.workflow.IndexChanged(now)
	}
}

func (h *Host) CloseSearch(port applicationsearch.Port, input InputSnapshot) error {
	if h == nil || h.workflow == nil {
		return applicationsearch.ErrWorkflowUnavailable
	}
	h.view.Close()
	return h.workflow.Close(port, input.queryInput())
}

func (h *Host) Reset() {
	if h != nil && h.workflow != nil {
		h.workflow.Reset()
	}
}

func (h *Host) PendingNavigationInput() (InputSnapshot, bool) {
	if h == nil || h.workflow == nil {
		return InputSnapshot{}, false
	}
	input, ok := h.workflow.PendingNavigationInput()
	return InputSnapshot{
		Query: input.Text, Literal: input.Literal, ActivePath: input.ActivePath,
		ActiveRevision: input.ActiveRevision,
	}, ok
}

func (h *Host) Navigate(port applicationsearch.Port, repository document.Repository, input InputSnapshot, hit domainsearch.Hit, currentPath string, currentRevision document.Revision, now time.Time) Outcome {
	if h == nil || h.workflow == nil {
		return Outcome{Err: applicationsearch.ErrWorkflowUnavailable}
	}
	result := h.workflow.Navigate(port, repository, input.queryInput(), hit, currentPath, currentRevision, now)
	switch result.Kind {
	case applicationsearch.NavigationAccepted:
		return Outcome{Kind: OutcomeNavigate, Hit: result.Hit, Path: result.Path}
	case applicationsearch.NavigationRequery:
		return Outcome{Kind: OutcomeResultsUpdated, Path: result.Path}
	case applicationsearch.NavigationNoMatch:
		return Outcome{Kind: OutcomeNoMatch, Path: result.Path}
	default:
		return Outcome{Err: result.Err, Path: result.Path}
	}
}

func (h *Host) Complete(result ResultSnapshot) Outcome {
	if h == nil || h.workflow == nil {
		return Outcome{Kind: OutcomeIgnored}
	}
	queryResult := applicationsearch.QueryResult{
		Completed: result.Completed, Token: result.Token, Query: result.Query, Literal: result.Literal,
	}
	state := applicationsearch.SearchState{
		Token: result.Search.Token, Query: result.Search.Query, Literal: result.Search.Literal,
		Hits: result.Search.Hits, Busy: result.Search.Busy, Cost: result.Search.Cost,
	}
	if !h.workflow.IsCurrentResult(queryResult, state) {
		return Outcome{Kind: OutcomeIgnored}
	}
	outcome := Outcome{Kind: OutcomeResultsUpdated, Search: result.Search}
	navigation := h.workflow.AcceptResult(queryResult, state)
	switch navigation.Kind {
	case applicationsearch.NavigationAccepted:
		outcome.Kind, outcome.Hit = OutcomeNavigate, navigation.Hit
	case applicationsearch.NavigationNoMatch:
		outcome.Kind, outcome.Path = OutcomeNoMatch, navigation.Path
	}
	return outcome
}

func ResultSnapshotFromVault(result applicationvault.Result) ResultSnapshot {
	state := result.Snapshot.Search
	return ResultSnapshot{
		Completed: result.Kind == applicationvault.QueryCompleted,
		Token:     result.Token, Query: result.Query, Literal: result.Literal,
		Search: SearchSnapshot{Token: state.Token, Query: state.Query, Literal: state.Literal, Hits: state.Hits, Busy: state.Busy, Cost: state.Cost},
	}
}
