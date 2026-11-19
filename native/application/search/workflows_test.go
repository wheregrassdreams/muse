package search

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"muse/document"
	domainsearch "muse/search"
)

type workflowPort struct {
	state     SearchState
	queries   []QueryInput
	refreshes []string
	readPath  func(string) (string, error)
}

func (p *workflowPort) SearchState() SearchState { return p.state }
func (p *workflowPort) Query(input QueryInput) (uint64, error) {
	p.state.Token++
	p.state.Query, p.state.Literal = input.Text, input.Literal
	p.queries = append(p.queries, input)
	return p.state.Token, nil
}
func (p *workflowPort) ResolveHitPath(path string) (string, error) {
	if filepath.IsAbs(path) || path == ".." || len(path) > 3 && path[:3] == "../" {
		return "", errors.New("outside vault")
	}
	return filepath.Join("/vault", filepath.FromSlash(path)), nil
}
func (p *workflowPort) Refresh(path string) error {
	p.refreshes = append(p.refreshes, path)
	return nil
}

type workflowRepository struct {
	note  document.Note
	err   error
	reads int
}

func newWorkflowFixture(state SearchState) (*workflowPort, *Workflow, time.Time) {
	return &workflowPort{state: state}, NewWorkflow(), time.Now()
}

func mustScheduleQuery(t *testing.T, workflow *Workflow, port *workflowPort, input QueryInput, at time.Time) {
	t.Helper()
	if _, err := workflow.ScheduleQuery(port, input, at); err != nil {
		t.Fatal(err)
	}
}

func (r *workflowRepository) Read(string) (document.Note, error) {
	r.reads++
	return r.note, r.err
}
func (r *workflowRepository) Save(string, string, document.Revision) (document.Revision, error) {
	return "", nil
}

func TestWorkflowSchedulesLatestInputAndRejectsOldCompletionDuringDebounce(t *testing.T) {
	now := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	port := &workflowPort{state: SearchState{Cost: 12 * time.Millisecond}}
	workflow := NewWorkflow()
	first := QueryInput{Text: "first", ActivePath: "/vault/a.md", ActiveRevision: "rev-a", ActiveSource: "a"}
	if wait, err := workflow.ScheduleQuery(port, first, now); err != nil || wait != balancedWait {
		t.Fatalf("first schedule wait=%s err=%v", wait, err)
	}
	mustScheduleQuery(t, workflow, port, first, now.Add(balancedWait))
	if len(port.queries) != 1 || port.queries[0] != first {
		t.Fatalf("submitted queries = %+v", port.queries)
	}
	completed := QueryResult{Completed: true, Token: port.state.Token, Query: "first"}
	if !workflow.IsCurrentResult(completed, port.state) {
		t.Fatal("latest completion was rejected")
	}

	next := QueryInput{Text: "second", ActivePath: "/vault/a.md", ActiveRevision: "rev-a", ActiveSource: "a"}
	if wait, err := workflow.ScheduleQuery(port, next, now.Add(40*time.Millisecond)); err != nil || wait != balancedWait {
		t.Fatalf("next schedule wait=%s err=%v", wait, err)
	}
	if workflow.IsCurrentResult(completed, port.state) {
		t.Fatal("old completion was accepted during the debounce interval")
	}
	mustScheduleQuery(t, workflow, port, next, now.Add(56*time.Millisecond))
	if len(port.queries) != 2 || port.queries[1] != next {
		t.Fatalf("latest query was not submitted: %+v", port.queries)
	}
	if !workflow.IsCurrentResult(QueryResult{Completed: true, Token: port.state.Token, Query: "second"}, port.state) {
		t.Fatal("current completion was rejected")
	}
}

func TestWorkflowQueryIdentityIncludesLiteralAndActiveDocumentOverlay(t *testing.T) {
	port, workflow, now := newWorkflowFixture(SearchState{})
	base := QueryInput{Text: "same", ActivePath: "/vault/a.md", ActiveRevision: "rev-one", ActiveSource: "one"}
	mustScheduleQuery(t, workflow, port, base, now)
	changedOverlay := base
	changedOverlay.ActiveSource = "two"
	changedOverlay.ActiveRevision = "rev-two"
	mustScheduleQuery(t, workflow, port, changedOverlay, now.Add(time.Millisecond))
	mustScheduleQuery(t, workflow, port, changedOverlay, now.Add(2*time.Millisecond))
	literal := changedOverlay
	literal.Literal = true
	mustScheduleQuery(t, workflow, port, literal, now.Add(3*time.Millisecond))
	if len(port.queries) != 3 || port.queries[1] != changedOverlay || port.queries[2] != literal {
		t.Fatalf("query identity failed to include overlay/mode: %+v", port.queries)
	}
}

func TestWorkflowNavigationRevalidatesAndAcceptsRefreshedHit(t *testing.T) {
	port, workflow, now := newWorkflowFixture(SearchState{Cost: time.Millisecond})
	repository := &workflowRepository{note: document.NewNote("/vault/notes/a.md", "new", document.RevisionOf("new"))}
	input := QueryInput{Text: "needle", Literal: true}
	mustScheduleQuery(t, workflow, port, input, now)
	old := domainsearch.Hit{Path: "notes/a.md", Revision: string(document.RevisionOf("old")), Start: 1}
	outcome := workflow.Navigate(port, repository, input, old, "/vault/other.md", "", now.Add(time.Millisecond))
	if outcome.Kind != NavigationRequery || len(port.refreshes) != 1 || port.refreshes[0] != "/vault/notes/a.md" {
		t.Fatalf("stale navigation outcome=%+v refreshes=%v", outcome, port.refreshes)
	}
	if pending, ok := workflow.PendingNavigationInput(); !ok || pending.ActiveSource != "" {
		t.Fatalf("pending navigation retained an active source: %+v, %v", pending, ok)
	}
	mustScheduleQuery(t, workflow, port, input, now.Add(time.Millisecond+StaleNavigationRefreshDelay))
	refreshed := domainsearch.Hit{Path: "notes/a.md", Revision: string(document.RevisionOf("new")), Start: 9, End: 15}
	port.state.Hits = []domainsearch.Hit{refreshed}
	result := QueryResult{Completed: true, Token: port.state.Token, Query: input.Text, Literal: input.Literal}
	accepted := workflow.AcceptResult(result, port.state)
	if accepted.Kind != NavigationAccepted || accepted.Hit.Start != 9 {
		t.Fatalf("refreshed navigation = %+v", accepted)
	}
	if next := workflow.AcceptResult(result, port.state); next.Kind != NavigationIgnored {
		t.Fatalf("pending navigation was not consumed: %+v", next)
	}
}

func TestWorkflowNavigationShortCircuitsCurrentDocumentAndReportsMissing(t *testing.T) {
	port, workflow, _ := newWorkflowFixture(SearchState{})
	repository := &workflowRepository{}
	hit := domainsearch.Hit{Path: "notes/a.md", Revision: "current"}
	input := QueryInput{Text: "needle"}
	current := workflow.Navigate(port, repository, input, hit, "/vault/notes/a.md", document.Revision("current"), time.Now())
	if current.Kind != NavigationAccepted || repository.reads != 0 {
		t.Fatalf("current-document navigation=%+v disk reads=%d", current, repository.reads)
	}
	repository.err = os.ErrNotExist
	missing := workflow.Navigate(port, repository, input, hit, "", "", time.Now())
	if !errors.Is(missing.Err, os.ErrNotExist) || missing.Kind != NavigationIgnored || len(port.refreshes) != 0 {
		t.Fatalf("missing navigation=%+v refreshes=%v", missing, port.refreshes)
	}
}

func TestWorkflowNavigationRejectsEscapingPathAndCancelsObsoletePendingHit(t *testing.T) {
	port, workflow, _ := newWorkflowFixture(SearchState{Cost: time.Millisecond})
	repository := &workflowRepository{note: document.NewNote("/vault/notes/a.md", "new", document.RevisionOf("new"))}
	input := QueryInput{Text: "needle", Literal: true, ActiveRevision: "active-rev"}
	outside := domainsearch.Hit{Path: "../outside.md", Revision: "old"}
	if outcome := workflow.Navigate(port, repository, input, outside, "", "", time.Now()); outcome.Err == nil || repository.reads != 0 {
		t.Fatalf("escaping hit outcome=%+v repository reads=%d", outcome, repository.reads)
	}

	stale := domainsearch.Hit{Path: "notes/a.md", Revision: string(document.RevisionOf("old"))}
	if outcome := workflow.Navigate(port, repository, input, stale, "", "", time.Now()); outcome.Kind != NavigationRequery {
		t.Fatalf("stale hit outcome=%+v", outcome)
	}
	if _, ok := workflow.PendingNavigationInput(); !ok {
		t.Fatal("stale hit was not pending re-query")
	}
	changed := input
	changed.Text = "different query"
	mustScheduleQuery(t, workflow, port, changed, time.Now())
	if _, ok := workflow.PendingNavigationInput(); ok {
		t.Fatal("query change retained stale navigation")
	}
}

func TestWorkflowResetAndIndexChange(t *testing.T) {
	port, workflow, now := newWorkflowFixture(SearchState{})
	input := QueryInput{Text: "needle"}
	mustScheduleQuery(t, workflow, port, input, now)
	workflow.IndexChanged(now.Add(time.Millisecond))
	if workflow.IsCurrentResult(QueryResult{Completed: true, Token: port.state.Token, Query: input.Text}, port.state) {
		t.Fatal("index invalidation left old results current")
	}
	mustScheduleQuery(t, workflow, port, input, now.Add(time.Millisecond))
	if len(port.queries) != 2 {
		t.Fatalf("index change did not force a query: %+v", port.queries)
	}
	workflow.Reset()
	if workflow.IsCurrentResult(QueryResult{Completed: true, Token: port.state.Token, Query: input.Text}, port.state) {
		t.Fatal("Reset retained a current result")
	}
}
