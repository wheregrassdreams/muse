package nativeui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	applicationagent "muse/application/agent"
	"muse/config"
)

type assistantWorkspaceStub struct {
	root, path, source string
}

func (w *assistantWorkspaceStub) Root() string                          { return w.root }
func (w *assistantWorkspaceStub) CurrentDocument() (string, string)     { return w.path, w.source }
func (w *assistantWorkspaceStub) ReadFile(string) ([]byte, error)       { return nil, os.ErrNotExist }
func (w *assistantWorkspaceStub) ReadDir(string) ([]os.DirEntry, error) { return nil, nil }
func (w *assistantWorkspaceStub) ResolveWithinRoot(path string) (string, error) {
	return filepath.Join(w.root, path), nil
}
func (w *assistantWorkspaceStub) DestinationRevision(string) (string, error) { return "", nil }
func (w *assistantWorkspaceStub) SaveDocument(string, string, string) error  { return nil }
func (w *assistantWorkspaceStub) ApplyCurrentDocument(source string) error {
	w.source = source
	return nil
}
func (w *assistantWorkspaceStub) MoveDocument(string, string, string) (string, error) {
	return "", errors.New("unused")
}
func (w *assistantWorkspaceStub) TrashDocuments(string, []string) error { return errors.New("unused") }
func (w *assistantWorkspaceStub) CreateFolder(string) error             { return nil }
func (w *assistantWorkspaceStub) Exists(string) (bool, error)           { return false, nil }

func TestAssistantOperationsUseTypedWorkspaceCapabilities(t *testing.T) {
	app := New()
	app.root = t.TempDir()
	app.path = filepath.Join(app.root, "note.md")
	app.loadDoc("# Title\n\nbody\n")
	port := &assistantWorkspaceStub{root: app.root, path: app.path, source: app.source}
	app.newAgentWorkspace = func(string, applicationagent.CurrentDocument) (applicationagent.WorkspacePort, error) {
		return port, nil
	}
	operations := appAssistantOperations{app: app}

	out := (applicationagent.Executor{Operations: operations}).Execute(context.Background(), applicationagent.ToolRequest{
		Name: "edit_document", Arguments: `{"old_text":"body","new_text":"updated"}`,
	})
	if out.Err != nil {
		t.Fatalf("edit tool failed: %v", out.Err)
	}
	if app.source != "# Title\n\nupdated\n" || !app.dirty {
		t.Fatalf("current document edit = %q, dirty=%v", app.source, app.dirty)
	}
}

func TestAssistantPresenterAppliesTypedEffectsOnce(t *testing.T) {
	app := New()
	app.loadDoc("# Note\n")
	presenter := &assistantPanel{effects: make(map[string]struct{})}
	part := applicationagent.Part{Kind: applicationagent.ToolPart, Tool: applicationagent.ToolPartState{
		ID: "insert-1", Name: "insert_into_document", Status: applicationagent.PartSucceeded,
		Result: `{"kind":"insert_document","text":"new paragraph","position":"end"}`,
	}}
	presenter.applyEffect(app, part)
	first := app.source
	presenter.applyEffect(app, part)
	if first == "# Note\n" || app.source != first {
		t.Fatalf("typed effect was not applied exactly once: first=%q after=%q", first, app.source)
	}
}

type assistantToolClient struct{ turn int }

func (c *assistantToolClient) Stream(_ applicationagent.Request, emit func(applicationagent.StreamEvent)) applicationagent.StreamResult {
	c.turn++
	if c.turn == 1 {
		return applicationagent.StreamResult{Tools: []applicationagent.ToolRequest{{
			ID: "edit-1", Name: "edit_document", Arguments: `{"old_text":"before","new_text":"after"}`,
		}}}
	}
	emit(applicationagent.StreamEvent{Text: "已完成修改"})
	return applicationagent.StreamResult{}
}
func (*assistantToolClient) Abort() {}

func TestAssistantSessionExecutesTypedWorkspaceToolAndContinues(t *testing.T) {
	app := New()
	app.root = t.TempDir()
	app.path = filepath.Join(app.root, "note.md")
	app.loadDoc("before\n")
	port := &assistantWorkspaceStub{root: app.root, path: app.path, source: app.source}
	app.newAgentWorkspace = func(string, applicationagent.CurrentDocument) (applicationagent.WorkspacePort, error) {
		return port, nil
	}
	presenter := &assistantPanel{settings: config.AssistantSettings{Tools: config.AssistantTools{Edit: true}}, permission: applicationagent.AllowChanges}
	session := applicationagent.NewSession(presenter.settings, &assistantToolClient{}, applicationagent.Capabilities{
		Tools:  applicationagent.Executor{Operations: appAssistantOperations{app: app}},
		Policy: appAgentPolicy{panel: presenter},
	})
	presenter.session = session
	if err := session.Send("把 before 改成 after"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := session.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot := session.Snapshot()
	if app.source != "after\n" || !app.dirty {
		t.Fatalf("source=%q dirty=%v", app.source, app.dirty)
	}
	if len(snapshot.Messages) != 2 || len(snapshot.Messages[1].Parts) != 2 || snapshot.Messages[1].Parts[1].Text != "已完成修改" {
		t.Fatalf("unexpected session transcript after tool turn: %+v", snapshot.Messages)
	}
	if got := snapshot.Messages[1].Parts[0].Tool.Status; got != applicationagent.PartSucceeded {
		t.Fatalf("tool status = %v", got)
	}
}
