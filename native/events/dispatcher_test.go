package events

import (
	"reflect"
	"testing"

	"muse/workspace"
)

func TestWorkspaceRestoredSubscriptionsAreScopedAndCloseable(t *testing.T) {
	dispatcher := New[workspace.WorkspaceRestored]()
	var first, second int
	subscription := dispatcher.Subscribe(func(workspace.WorkspaceRestored) { first++ })
	dispatcher.Subscribe(func(workspace.WorkspaceRestored) { second++ })
	dispatcher.Publish(workspace.WorkspaceRestored{State: workspace.State{Version: 1, VaultPath: "/vault"}})
	subscription.Close()
	subscription.Close()
	dispatcher.Publish(workspace.WorkspaceRestored{State: workspace.State{Version: 1, VaultPath: "/vault"}})
	if first != 1 || second != 2 {
		t.Fatalf("handler calls = (%d, %d), want (1, 2)", first, second)
	}
	dispatcher.Close()
	dispatcher.Publish(workspace.WorkspaceRestored{})
	if second != 2 {
		t.Fatalf("closed dispatcher delivered another event: %d", second)
	}
}

func TestDispatcherPublishesInRegistrationOrder(t *testing.T) {
	dispatcher := New[int]()
	var order []int
	dispatcher.Subscribe(func(int) { order = append(order, 1) })
	dispatcher.Subscribe(func(int) { order = append(order, 2) })
	dispatcher.Subscribe(func(int) { order = append(order, 3) })
	dispatcher.Publish(0)
	if want := []int{1, 2, 3}; !reflect.DeepEqual(order, want) {
		t.Fatalf("dispatch order = %v, want %v", order, want)
	}
}
