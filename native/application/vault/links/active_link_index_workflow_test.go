package links

import (
	"errors"
	"testing"
	"time"

	vaultapp "muse/application/vault"
)

type fakeActiveLinkGraph struct {
	snapshot vaultapp.Snapshot
	path     string
	source   string
	err      error
}

func (f *fakeActiveLinkGraph) ApplyActiveDocument(path, source string) error {
	f.path, f.source = path, source
	if f.err != nil {
		return f.err
	}
	f.snapshot.IndexRevision++
	return nil
}
func (f *fakeActiveLinkGraph) Snapshot() vaultapp.Snapshot { return f.snapshot }

func TestActiveLinkIndexWorkflowAppliesAndAcceptsCurrentTicket(t *testing.T) {
	now := time.Now()
	workflow := &ActiveLinkIndex{}
	graph := &fakeActiveLinkGraph{snapshot: vaultapp.Snapshot{Ready: true}}
	ticket, scheduled := workflow.ScheduleDocument("notes/one.md", "# one", now)
	if !scheduled {
		t.Fatal("ScheduleDocument rejected Markdown document")
	}
	if _, wait, due := workflow.Take(now.Add(ActiveLinkIndexDebounce)); !due || wait != 0 {
		t.Fatalf("Take() = due %v, wait %v", due, wait)
	}
	result := workflow.Apply(ticket, graph)
	if result.Err != nil || graph.path != ticket.Path || graph.source != ticket.Source {
		t.Fatalf("Apply() = %+v, graph applied %q %q", result, graph.path, graph.source)
	}
	got, accepted := workflow.Accept(result, ticket.Path, ticket.Source)
	if !accepted || got.IndexRevision != 1 {
		t.Fatalf("Accept() = %+v, %v", got, accepted)
	}
}

func TestActiveLinkIndexWorkflowOnlySchedulesActiveMarkdownDocuments(t *testing.T) {
	now := time.Now()
	workflow := &ActiveLinkIndex{}
	first, ok := workflow.ScheduleDocument("notes/one.md", "# one", now)
	if !ok || first.Generation == 0 {
		t.Fatalf("ScheduleDocument(markdown) = %+v, %v", first, ok)
	}
	if _, ok := workflow.ScheduleDocument("assets/diagram.png", "", now.Add(time.Second)); ok {
		t.Fatal("ScheduleDocument(image) scheduled active-source indexing")
	}
	if _, wait, due := workflow.Take(now.Add(2 * time.Second)); due || wait != 0 {
		t.Fatalf("non-Markdown selection left work pending: due %v, wait %v", due, wait)
	}
	if _, accepted := workflow.Accept(ActiveLinkIndexResult{Ticket: first, Snapshot: vaultapp.Snapshot{Ready: true}}, first.Path, "# one"); accepted {
		t.Fatal("Accept() accepted Markdown result after a non-Markdown document cleared it")
	}
}

func TestActiveLinkIndexWorkflowRejectsStaleOrMismatchedResults(t *testing.T) {
	now := time.Now()
	workflow := &ActiveLinkIndex{}
	graph := &fakeActiveLinkGraph{}
	oldTicket, scheduled := workflow.ScheduleDocument("notes/one.md", "old", now)
	if !scheduled {
		t.Fatal("ScheduleDocument rejected old Markdown document")
	}
	newTicket, scheduled := workflow.ScheduleDocument("notes/one.md", "new", now.Add(time.Second))
	if !scheduled {
		t.Fatal("ScheduleDocument rejected new Markdown document")
	}
	if result := workflow.Apply(oldTicket, graph); result.Snapshot.Ready || result.Err != nil {
		t.Fatalf("stale Apply() unexpectedly ran: %+v", result)
	}
	workflow.Take(now.Add(2 * time.Second))
	result := workflow.Apply(newTicket, graph)
	if _, accepted := workflow.Accept(result, newTicket.Path, "old"); accepted {
		t.Fatal("Accept() accepted a source different from the active editor")
	}
	if _, accepted := workflow.Accept(result, "notes/other.md", newTicket.Source); accepted {
		t.Fatal("Accept() accepted a different active path")
	}
	workflow.Clear()
	if _, accepted := workflow.Accept(result, newTicket.Path, newTicket.Source); accepted {
		t.Fatal("Accept() accepted a result after workflow reset")
	}
}

func TestActiveLinkIndexWorkflowPropagatesApplyError(t *testing.T) {
	now := time.Now()
	workflow := &ActiveLinkIndex{}
	ticket, scheduled := workflow.ScheduleDocument("notes/one.md", "# one", now)
	if !scheduled {
		t.Fatal("ScheduleDocument rejected Markdown document")
	}
	workflow.Take(now.Add(ActiveLinkIndexDebounce))
	graph := &fakeActiveLinkGraph{err: errors.New("index rejected source")}
	result := workflow.Apply(ticket, graph)
	if result.Err == nil {
		t.Fatal("Apply() lost the application error")
	}
	if _, accepted := workflow.Accept(result, ticket.Path, ticket.Source); accepted {
		t.Fatal("Accept() accepted a failed graph update")
	}
}
