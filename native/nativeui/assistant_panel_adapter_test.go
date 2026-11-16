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
	"muse/platform/filesystem/imagefiles"
)

type assistantWorkspaceStub struct {
	root, path, source string
}

func (w *assistantWorkspaceStub) Root() string                                     { return w.root }
func (w *assistantWorkspaceStub) CurrentDocument() (string, string)                { return w.path, w.source }
func (w *assistantWorkspaceStub) ReadFile(string) ([]byte, error)                  { return nil, os.ErrNotExist }
func (w *assistantWorkspaceStub) ReadDir(string) ([]applicationagent.Entry, error) { return nil, nil }
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
	setDocumentPathForTest(app, filepath.Join(app.root, "note.md"))
	app.loadDoc("# Title\n\nbody\n")
	port := &assistantWorkspaceStub{root: app.root, path: app.documentPath(), source: app.documentSource()}
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
	if app.documentSource() != "# Title\n\nupdated\n" || !app.documentDirty() {
		t.Fatalf("current document edit = %q, dirty=%v", app.documentSource(), app.documentDirty())
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
	first := app.documentSource()
	presenter.applyEffect(app, part)
	if first == "# Note\n" || app.documentSource() != first {
		t.Fatalf("typed effect was not applied exactly once: first=%q after=%q", first, app.documentSource())
	}
}

func TestAssistantMarkdownReceivesInjectedLocalImageFiles(t *testing.T) {
	app := New()
	files := imagefiles.New()
	app.imageLocalFiles = files
	app.root = t.TempDir()
	options := assistantMarkdownOptions(app, 480)
	if options.LocalFiles != files {
		t.Fatal("assistant Markdown renderer did not receive the injected local image capability")
	}
	if options.ContentWidth != 480 || options.AssetRoot != app.root {
		t.Fatalf("assistant Markdown options lost its layout or vault context: %+v", options)
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
	setDocumentPathForTest(app, filepath.Join(app.root, "note.md"))
	app.loadDoc("before\n")
	port := &assistantWorkspaceStub{root: app.root, path: app.documentPath(), source: app.documentSource()}
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
	if app.documentSource() != "after\n" || !app.documentDirty() {
		t.Fatalf("source=%q dirty=%v", app.documentSource(), app.documentDirty())
	}
	if len(snapshot.Messages) != 2 || len(snapshot.Messages[1].Parts) != 2 || snapshot.Messages[1].Parts[1].Text != "已完成修改" {
		t.Fatalf("unexpected session transcript after tool turn: %+v", snapshot.Messages)
	}
	if got := snapshot.Messages[1].Parts[0].Tool.Status; got != applicationagent.PartSucceeded {
		t.Fatalf("tool status = %v", got)
	}
}
