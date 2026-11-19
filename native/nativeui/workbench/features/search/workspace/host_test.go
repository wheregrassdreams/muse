package workspace

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	applicationsearch "muse/application/search"
	"muse/document"
	domainsearch "muse/search"
)

type hostPort struct {
	state   applicationsearch.SearchState
	queries []applicationsearch.QueryInput
}

func (p *hostPort) SearchState() applicationsearch.SearchState { return p.state }
func (p *hostPort) Query(input applicationsearch.QueryInput) (uint64, error) {
	p.state.Token++
	p.state.Query, p.state.Literal = input.Text, input.Literal
	p.queries = append(p.queries, input)
	return p.state.Token, nil
}
func (p *hostPort) ResolveHitPath(path string) (string, error) { return "/vault/" + path, nil }
func (p *hostPort) Refresh(string) error                       { return nil }

type hostRepository struct{ note document.Note }

func (r hostRepository) Read(string) (document.Note, error) { return r.note, nil }
func (r hostRepository) Save(string, string, document.Revision) (document.Revision, error) {
	return "", nil
}

func TestHostOwnsFreshnessAndReturnsTypedResults(t *testing.T) {
	now := time.Now()
	port := &hostPort{state: applicationsearch.SearchState{Cost: 12 * time.Millisecond}}
	host := NewHost(nil)
	first := InputSnapshot{Query: "needle", ActivePath: "/vault/a.md", ActiveRevision: "revision-1", ActiveSource: "first source"}
	delay, err := host.ScheduleQuery(port, first, now)
	if err != nil || delay == 0 {
		t.Fatalf("first query schedule delay=%s err=%v", delay, err)
	}
	if _, err := host.ScheduleQuery(port, first, now.Add(delay)); err != nil {
		t.Fatal(err)
	}
	if len(port.queries) != 1 || port.queries[0].ActiveSource != "first source" {
		t.Fatalf("submitted payloads = %+v", port.queries)
	}
	completed := ResultSnapshot{
		Completed: true, Token: port.state.Token, Query: "needle",
		Search: SearchSnapshot{Token: port.state.Token, Query: "needle", Hits: []domainsearch.Hit{{Path: "a.md"}}},
	}
	if outcome := host.Complete(completed); outcome.Kind != OutcomeResultsUpdated || len(outcome.Search.Hits) != 1 {
		t.Fatalf("current completion outcome=%+v", outcome)
	}

	changed := first
	changed.ActiveRevision, changed.ActiveSource = "revision-2", "edited source"
	delay, err = host.ScheduleQuery(port, changed, now.Add(time.Second))
	if err != nil || delay == 0 {
		t.Fatalf("changed overlay schedule delay=%s err=%v", delay, err)
	}
	if outcome := host.Complete(completed); outcome.Kind != OutcomeIgnored {
		t.Fatalf("old result accepted during overlay debounce: %+v", outcome)
	}
}

func TestHostRendersItsFeatureViewAndDispatchesTypedAction(t *testing.T) {
	host := NewHost(nil)
	host.Open()
	var actions []Action
	tester := ui.NewTester(func(c *ui.Context) {
		host.Render(c, View{Recent: []RecentItem{{Path: "recent.md", Title: "Recent note", PathLabel: "recent.md"}}}, func(action Action) {
			actions = append(actions, action)
		})
	}, 800, 600)
	tester.Frame()
	if !tester.HasText("Recent note") || !host.IsOpen() {
		t.Fatalf("feature host did not render its open state: %q", tester.Texts())
	}
	if host.Selection() != 0 {
		t.Fatalf("initial selection=%d, want 0", host.Selection())
	}
	tester.Type("typed")
	tester.Frame()
	if len(actions) == 0 || actions[len(actions)-1].Kind != QueryChanged || actions[len(actions)-1].Query != "typed" {
		t.Fatalf("typed input actions=%+v", actions)
	}
	if host.Query() != "typed" {
		t.Fatalf("query=%q, want typed", host.Query())
	}
	host.ClosePanel()
	if host.IsOpen() {
		t.Fatal("ClosePanel left search open")
	}
}

func TestHostStaleNavigationReturnsRequeryThenTypedIntent(t *testing.T) {
	now := time.Now()
	port := &hostPort{}
	host := NewHost(nil)
	input := InputSnapshot{Query: "needle", ActivePath: "/vault/current.md", ActiveRevision: "active", ActiveSource: "large document source"}
	hit := domainsearch.Hit{Path: "notes/a.md", Revision: string(document.RevisionOf("old")), Start: 2, End: 7}
	repo := hostRepository{note: document.NewNote("/vault/notes/a.md", "new", document.RevisionOf("new"))}
	stale := host.Navigate(port, repo, input, hit, input.ActivePath, document.Revision("active"), now)
	if stale.Kind != OutcomeResultsUpdated {
		t.Fatalf("stale navigation outcome=%+v", stale)
	}
	pending, ok := host.PendingNavigationInput()
	if !ok || pending.Query != input.Query || pending.ActiveSource != "" {
		t.Fatalf("pending navigation input=%+v ok=%v", pending, ok)
	}
	pending.ActiveSource, pending.ActiveRevision = input.ActiveSource, input.ActiveRevision
	if _, err := host.ScheduleQuery(port, pending, now.Add(applicationsearch.StaleNavigationRefreshDelay)); err != nil {
		t.Fatal(err)
	}
	refreshed := hit
	refreshed.Revision = string(document.RevisionOf("new"))
	port.state.Hits = []domainsearch.Hit{refreshed}
	result := ResultSnapshot{
		Completed: true, Token: port.state.Token, Query: input.Query,
		Search: SearchSnapshot{Token: port.state.Token, Query: input.Query, Hits: []domainsearch.Hit{refreshed}},
	}
	if outcome := host.Complete(result); outcome.Kind != OutcomeNavigate || outcome.Hit.Revision != refreshed.Revision {
		t.Fatalf("refreshed navigation outcome=%+v", outcome)
	}
}
