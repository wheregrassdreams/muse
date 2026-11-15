package app

import (
	"testing"

	"muse/document"
	"muse/events"
	"muse/workspace"
)

func TestRuntimeCloseClosesOwnedDispatchersIdempotently(t *testing.T) {
	workspaceEvents := events.New[workspace.WorkspaceRestored]()
	documentEvents := events.New[document.DocumentSaved]()
	workspaceCalls, documentCalls := 0, 0
	workspaceEvents.Subscribe(func(workspace.WorkspaceRestored) { workspaceCalls++ })
	documentEvents.Subscribe(func(document.DocumentSaved) { documentCalls++ })
	runtime := &Runtime{WorkspaceEvents: workspaceEvents, DocumentEvents: documentEvents}

	runtime.Close()
	runtime.Close()
	workspaceEvents.Publish(workspace.WorkspaceRestored{})
	documentEvents.Publish(document.DocumentSaved{})
	if workspaceCalls != 0 || documentCalls != 0 {
		t.Fatalf("events dispatched after runtime close: workspace=%d document=%d", workspaceCalls, documentCalls)
	}
}
