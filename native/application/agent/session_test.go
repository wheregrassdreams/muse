package agent

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"muse/config"
)

type fakeStream struct {
	chunks chan func(StreamEvent)
	done   chan struct{}
	aborts int
	once   sync.Once
}

type testPolicy struct{ permission Permission }

func (p testPolicy) Enabled(config.AssistantSettings, string) bool     { return true }
func (p testPolicy) RequiresApproval(string) bool                      { return true }
func (p testPolicy) Permission() Permission                            { return p.permission }
func (p testPolicy) Schemas(config.AssistantSettings) []map[string]any { return nil }

type testTools struct{ calls int }

func (t *testTools) Execute(context.Context, ToolRequest) ToolResult {
	t.calls++
	return ToolResult{Content: "done"}
}

func (f *fakeStream) Stream(_ Request, onDelta func(StreamEvent)) StreamResult {
	f.chunks <- onDelta
	<-f.done
	return StreamResult{Aborted: true}
}
func (f *fakeStream) Abort() { f.aborts++; f.once.Do(func() { close(f.done) }) }

func TestSessionOwnsTranscriptAndStreamsDeltas(t *testing.T) {
	client := &fakeStream{chunks: make(chan func(StreamEvent), 1), done: make(chan struct{})}
	session := NewSession(config.AssistantSettings{Model: "m"}, client)
	if err := session.Send("  summarize  "); err != nil {
		t.Fatal(err)
	}
	callback := <-client.chunks
	callback(StreamEvent{Text: "A result"})
	client.once.Do(func() { close(client.done) })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot := session.Snapshot()
	if snapshot.Busy || len(snapshot.Messages) != 2 || !reflect.DeepEqual(snapshot.Messages[0], Message{Role: User, Content: "summarize"}) || !reflect.DeepEqual(snapshot.Messages[1], Message{Role: Assistant, Content: "A result", Parts: []Part{{Kind: TextPart, Text: "A result"}}}) {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestSessionApprovalAndStopAreTypedAndCancelPendingWork(t *testing.T) {
	client := &fakeStream{chunks: make(chan func(StreamEvent), 1), done: make(chan struct{})}
	session := NewSession(config.AssistantSettings{Model: "m"}, client)
	if err := session.Send("do work"); err != nil {
		t.Fatal(err)
	}
	<-client.chunks
	decision := make(chan Decision, 1)
	go func() {
		got, err := session.RequestApproval(context.Background(), ToolRequest{ID: "tool-1", Name: "write_document"})
		if err != nil {
			decision <- Reject
			return
		}
		decision <- got
	}()
	deadline := time.Now().Add(time.Second)
	for session.Snapshot().Phase != "waiting for approval" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := session.DecideApproval("tool-1", AlwaysApprove); err != nil {
		t.Fatal(err)
	}
	if got := <-decision; got != AlwaysApprove {
		t.Fatalf("approval = %v", got)
	}
	session.Stop()
	if client.aborts != 1 || session.Snapshot().Busy {
		t.Fatalf("stop state = %+v, aborts %d", session.Snapshot(), client.aborts)
	}
}

func TestExecuteToolRoutesThroughPolicyApprovalAndExecutor(t *testing.T) {
	client := &fakeStream{chunks: make(chan func(StreamEvent), 1), done: make(chan struct{})}
	tools := &testTools{}
	session := NewSession(config.AssistantSettings{Model: "m"}, client, Capabilities{Tools: tools, Policy: testPolicy{permission: AskBeforeChanges}})
	if err := session.Send("edit"); err != nil {
		t.Fatal(err)
	}
	<-client.chunks
	result := make(chan ToolResult, 1)
	go func() {
		result <- session.ExecuteTool(context.Background(), ToolRequest{ID: "t1", Name: "write_document"})
	}()
	deadline := time.Now().Add(time.Second)
	for session.Snapshot().Phase != "waiting for approval" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := session.DecideApproval("t1", Approve); err != nil {
		t.Fatal(err)
	}
	if got := <-result; got.Err != nil || got.Content != "done" || tools.calls != 1 {
		t.Fatalf("tool result=%+v calls=%d", got, tools.calls)
	}
	session.Stop()
}
