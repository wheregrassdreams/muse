package app

import (
	"path/filepath"
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

func TestRuntimeInjectsConfigurationRepositoryAndLocation(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("MUSE_CONFIG_DIR", configRoot)
	t.Setenv("MUSE_CACHE_DIR", t.TempDir())
	runtime, err := NewRuntime(func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if runtime.Configuration == nil {
		t.Fatal("configuration repository was not constructed")
	}
	if got, want := runtime.AssistantSettingsLocation, filepath.Join(configRoot, "ai.json"); got != want {
		t.Fatalf("assistant settings location = %q, want %q", got, want)
	}
}
