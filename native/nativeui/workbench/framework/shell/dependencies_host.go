package shell

import (
	"muse/document"
	"muse/events"
	"muse/workspace"
)

// EventPorts adapts application lifecycle events to the UI owner thread. The
// shell only owns subscription lifetime and thread marshalling; handlers own
// workspace restoration and document-index policy.
type EventPorts struct {
	RunOnMain         func(func())
	WorkspaceRestored func(workspace.State)
	DocumentSaved     func(document.DocumentSaved)
}

type EventSubscriptions struct {
	workspace *events.Subscription
	document  *events.Subscription
}

func SubscribeEvents(
	workspaceEvents *events.Dispatcher[workspace.WorkspaceRestored],
	documentEvents *events.Dispatcher[document.DocumentSaved],
	ports EventPorts,
) *EventSubscriptions {
	subscriptions := &EventSubscriptions{}
	if workspaceEvents != nil && ports.WorkspaceRestored != nil {
		subscriptions.workspace = workspaceEvents.Subscribe(func(event workspace.WorkspaceRestored) {
			dispatch(ports.RunOnMain, func() { ports.WorkspaceRestored(event.State) })
		})
	}
	if documentEvents != nil && ports.DocumentSaved != nil {
		subscriptions.document = documentEvents.Subscribe(func(event document.DocumentSaved) {
			dispatch(ports.RunOnMain, func() { ports.DocumentSaved(event) })
		})
	}
	return subscriptions
}

func (s *EventSubscriptions) Close() {
	if s == nil {
		return
	}
	if s.workspace != nil {
		s.workspace.Close()
		s.workspace = nil
	}
	if s.document != nil {
		s.document.Close()
		s.document = nil
	}
}

func dispatch(runOnMain func(func()), callback func()) {
	if runOnMain == nil {
		callback()
		return
	}
	runOnMain(callback)
}
