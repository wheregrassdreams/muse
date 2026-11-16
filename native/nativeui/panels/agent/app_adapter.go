package nativeui

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/application/agent"
	applicationvault "muse/application/vault"
	"muse/config"
	"muse/document"
	"muse/locales"
	mdoc "muse/markdown"
	"muse/nativeui/components/doctypes"
	markdownui "muse/nativeui/components/markdown"
	agentpanel "muse/nativeui/panels/agent"
)

// assistantPanel is the desktop adapter around the application-owned session.
// It keeps only transient presentation state; transcript and stream lifecycle
// live in application/agent.Session.
type assistantPanel struct {
	session    *agent.Session
	draft      string
	scroll     ui.ScrollState
	permission agent.Permission
	settings   config.AssistantSettings
	effects    map[string]struct{}
}

func newAssistantPanel(app *App) *assistantPanel {
	panel := &assistantPanel{}
	var client agent.StreamClient
	if app != nil && app.aiClient != nil {
		client = agent.NewProviderClient(app.aiClient)
	}
	panel.permission = agent.AskBeforeChanges
	panel.settings = app.ai
	panel.effects = make(map[string]struct{})
	panel.session = agent.NewSession(app.ai, client, agent.Capabilities{Tools: agent.Executor{Operations: appAssistantOperations{app}}, Policy: appAgentPolicy{panel: panel}})
	return panel
}

func (p *assistantPanel) render(c *ui.Context, app *App) {
	if p == nil || p.session == nil {
		return
	}
	if p.settings != app.ai {
		p.settings = app.ai
		p.session.SetSettings(app.ai)
	}
	snapshot := p.session.Snapshot()
	model := agentpanel.ViewModel{Draft: p.draft, Busy: snapshot.Busy, Phase: snapshot.Phase, Error: snapshot.Error, Scroll: &p.scroll,
		Icon: iconAI, ShowScrollbar: app.aiIndicator.Visible(c.Now(), p.scroll.X, p.scroll.Y),
		Suggestions: []string{locales.AiSuggestionSummarize(), locales.AiSuggestionFormalize(), locales.AiSuggestionTitles()},
		RenderMarkdown: func(c *ui.Context, source string, width float32) {
			doc := mdoc.Parse(source)
			markdownui.BlocksView(c, doc.Blocks, markdownui.Options{BodyFontSize: 12.5, SoftBreaksAsSpaces: true, ContentWidth: width, BaseDir: app.docDir(), AssetRoot: app.root, OnImagePreview: app.openImagePreview, Padding: 0, IndentStep: 14})
		}}
	if strings.TrimSpace(app.ai.BaseURL) == "" || strings.TrimSpace(app.ai.Model) == "" {
		model.SetupTitle, model.SetupDetail, model.SetupButton = locales.AiSetupModelTitle(), locales.AiSetupModelDetail(), locales.AiSetupModel()
	}
	for _, message := range snapshot.Messages {
		role := agentpanel.Assistant
		if message.Role == agent.User {
			role = agentpanel.User
		}
		out := agentpanel.Message{Role: role, Text: message.Content}
		for _, part := range message.Parts {
			p.applyEffect(app, part)
			kind := agentpanel.TextPart
			if part.Kind == agent.ThoughtPart {
				kind = agentpanel.ThoughtPart
			}
			if part.Kind == agent.ToolPart {
				kind = agentpanel.ToolPart
			}
			status := agentpanel.ToolRunning
			switch part.Tool.Status {
			case agent.PartApproval:
				status = agentpanel.ToolNeedsApproval
			case agent.PartSucceeded:
				status = agentpanel.ToolSucceeded
			case agent.PartFailed:
				status = agentpanel.ToolFailed
			}
			out.Parts = append(out.Parts, agentpanel.Part{Kind: kind, Text: part.Text, Thinking: part.Duration, Expanded: part.Expanded, Tool: agentpanel.Tool{ID: part.Tool.ID, Name: part.Tool.Name, Target: part.Tool.Arguments, Status: status, Summary: part.Tool.Result, Details: part.Tool.Error}})
		}
		model.Messages = append(model.Messages, out)
	}
	agentpanel.Render(c, model, func(action agentpanel.Action) {
		switch action.Kind {
		case agentpanel.DraftChanged:
			p.draft = action.Text
		case agentpanel.Send:
			p.draft = ""
			_ = p.session.Send(action.Text)
		case agentpanel.Stop:
			p.session.Stop()
		case agentpanel.ClearConversation:
			if p.session.Clear() == nil {
				p.effects = make(map[string]struct{})
				p.scroll.Y = 0
			}
		case agentpanel.OpenSettings:
			app.settingsOpen = true
			app.settingsTab = 2
		case agentpanel.UseSuggestion:
			p.draft = action.Text
		case agentpanel.ApproveTool:
			_ = p.session.DecideApproval(action.ToolID, agent.Approve)
		case agentpanel.AlwaysAllowTool:
			p.permission = agent.AllowChanges
			_ = p.session.DecideApproval(action.ToolID, agent.AlwaysApprove)
		case agentpanel.RejectTool:
			_ = p.session.DecideApproval(action.ToolID, agent.Reject)
		case agentpanel.TogglePart:
			p.session.TogglePart(action.MessageIndex, action.PartIndex)
		}
	})
	if snapshot.Busy {
		c.After(60 * time.Millisecond)
	}
}

func (a *App) aiView(c *ui.Context) {
	if a.assistant != nil {
		a.assistant.render(c, a)
	}
}

func (p *assistantPanel) applyEffect(app *App, part agent.Part) {
	if app == nil || part.Kind != agent.ToolPart || part.Tool.Status != agent.PartSucceeded || part.Tool.Result == "" {
		return
	}
	var effect agent.ToolEffect
	if json.Unmarshal([]byte(part.Tool.Result), &effect) != nil || effect.Kind == "" {
		return
	}
	key := part.Tool.ID
	if key == "" {
		key = part.Tool.Name + "\x00" + part.Tool.Arguments
	}
	if _, done := p.effects[key]; done {
		return
	}
	p.effects[key] = struct{}{}
	switch effect.Kind {
	case agent.OpenDocumentEffect:
		if doctypes.IsImagePath(effect.Path) {
			app.openImageDocument(effect.Path, true)
		} else {
			app.requestOpen(effect.Path)
		}
	case agent.InsertDocumentEffect:
		if effect.Position == "start" {
			app.insertAIContentAtStart(effect.Text)
		} else {
			app.insertAIContent(effect.Text)
		}
	}
}

func (a *App) insertAIContentAtStart(text string) {
	if a == nil || a.ed == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	a.ed.PrependBlock(text)
	a.docChanged()
	a.notice(locales.DocumentInserted())
	a.invalidate()
}

type appAssistantOperations struct{ app *App }

func (o appAssistantOperations) operations() (agent.WorkspaceOperations, error) {
	if o.app == nil {
		return agent.WorkspaceOperations{}, agent.ErrUnavailable
	}
	var root, path, source string
	o.app.onMain(func() {
		root, path, source = o.app.root, o.app.path, o.app.source
	})
	if root == "" {
		return agent.WorkspaceOperations{}, errors.New("workspace is unavailable")
	}
	if o.app.newAgentWorkspace == nil {
		return agent.WorkspaceOperations{}, errors.New("assistant workspace adapter is not configured")
	}
	workspace, err := o.app.newAgentWorkspace(root, func() (string, string) { return path, source })
	if err != nil {
		return agent.WorkspaceOperations{}, err
	}
	directory, _ := workspace.(agent.WorkspaceDirectoryPort)
	port := &appAssistantWorkspace{app: o.app, WorkspacePort: workspace, Directory: directory, root: root}
	return agent.WorkspaceOperations{Read: agent.WorkspaceReadTools{Workspace: port}, Write: agent.WorkspaceWriteTools{Workspace: port}}, nil
}

type appAssistantWorkspace struct {
	app *App
	agent.WorkspacePort
	Directory agent.WorkspaceDirectoryPort
	root      string
}

func (w *appAssistantWorkspace) DestinationRevision(path string) (string, error) {
	if w.app.documentSession == nil {
		return "", errors.New("document repository is not configured")
	}
	revision, err := w.app.documentSession.DestinationRevision(path, "", "")
	return string(revision), err
}

func (w *appAssistantWorkspace) SaveDocument(path, content, expected string) error {
	if w.app.documentSession == nil {
		return errors.New("document repository is not configured")
	}
	return w.app.writeDocument(path, content, document.Revision(expected))
}

func (w *appAssistantWorkspace) ApplyCurrentDocument(content string) error {
	var err error
	w.app.onMain(func() {
		if w.app.path == "" && w.app.source == "" {
			err = errors.New("current document is empty")
			return
		}
		w.app.editDoc(content)
	})
	return err
}

func (w *appAssistantWorkspace) MoveDocument(root, source, target string) (string, error) {
	if w.app.fileOperations == nil {
		return "", errors.New("vault file operations are not configured")
	}
	moved, err := w.app.fileOperations.MoveTo(root, source, target)
	if err != nil {
		return "", err
	}
	w.app.onMain(func() {
		previousPath := w.app.path
		w.app.clearTreeCache()
		w.app.remapTreePaths([]applicationvault.FileMove{moved})
		if previousPath != w.app.path {
			w.app.documentLoaded(w.app.diskSource())
		}
		w.app.addRecent(target, false)
		w.app.invalidate()
	})
	w.refresh(source, target)
	return moved.Target, nil
}

func (w *appAssistantWorkspace) TrashDocuments(root string, paths []string) error {
	if w.app.fileOperations == nil {
		return errors.New("vault file operations are not configured")
	}
	var current string
	var dirty bool
	w.app.onMain(func() { current, dirty = w.app.path, w.app.dirty })
	for _, path := range paths {
		if filepath.Clean(path) == filepath.Clean(current) && dirty {
			return errors.New("cannot delete the current document while it has unsaved changes")
		}
	}
	trashed, err := w.app.fileOperations.Trash(root, paths)
	if err != nil {
		return err
	}
	w.app.onMain(func() {
		for _, path := range trashed {
			w.app.clearTreeCache()
			if filepath.Clean(path) == filepath.Clean(w.app.path) {
				w.app.resetDoc()
			}
		}
		w.app.invalidate()
	})
	w.refresh(trashed...)
	return nil
}

func (w *appAssistantWorkspace) CreateFolder(path string) error {
	if w.Directory == nil {
		return errors.New("workspace directory operations are unavailable")
	}
	if err := w.Directory.CreateFolder(path); err != nil {
		return err
	}
	w.app.onMain(func() {
		w.app.clearTreeCache()
		w.app.invalidate()
	})
	w.refresh(path)
	return nil
}

func (w *appAssistantWorkspace) Exists(path string) (bool, error) {
	if w.Directory == nil {
		return false, errors.New("workspace directory operations are unavailable")
	}
	return w.Directory.Exists(path)
}

func (w *appAssistantWorkspace) refresh(paths ...string) {
	var vault *applicationvault.Session
	w.app.onMain(func() { vault = w.app.vault })
	if vault != nil {
		_ = vault.Refresh(paths...)
	}
}

func (o appAssistantOperations) delegate(ctx context.Context, input agent.ToolInput, invoke func(agent.WorkspaceOperations, context.Context, agent.ToolInput) (any, error)) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	operations, err := o.operations()
	if err != nil {
		return nil, err
	}
	return invoke(operations, ctx, input)
}

func (o appAssistantOperations) ListDocuments(c context.Context, i agent.ToolInput) (any, error) {
	return o.delegate(c, i, func(op agent.WorkspaceOperations, c context.Context, i agent.ToolInput) (any, error) {
		return op.ListDocuments(c, i)
	})
}
func (o appAssistantOperations) ReadDocument(c context.Context, i agent.ToolInput) (any, error) {
	return o.delegate(c, i, func(op agent.WorkspaceOperations, c context.Context, i agent.ToolInput) (any, error) {
		return op.ReadDocument(c, i)
	})
}
func (o appAssistantOperations) SearchDocuments(c context.Context, i agent.ToolInput) (any, error) {
	return o.delegate(c, i, func(op agent.WorkspaceOperations, c context.Context, i agent.ToolInput) (any, error) {
		return op.SearchDocuments(c, i)
	})
}
func (o appAssistantOperations) ReadOutline(c context.Context, i agent.ToolInput) (any, error) {
	return o.delegate(c, i, func(op agent.WorkspaceOperations, c context.Context, i agent.ToolInput) (any, error) {
		return op.ReadOutline(c, i)
	})
}
func (o appAssistantOperations) WorkspaceTree(c context.Context, i agent.ToolInput) (any, error) {
	return o.delegate(c, i, func(op agent.WorkspaceOperations, c context.Context, i agent.ToolInput) (any, error) {
		return op.WorkspaceTree(c, i)
	})
}
func (o appAssistantOperations) DocumentStats(c context.Context, i agent.ToolInput) (any, error) {
	return o.delegate(c, i, func(op agent.WorkspaceOperations, c context.Context, i agent.ToolInput) (any, error) {
		return op.DocumentStats(c, i)
	})
}
func (o appAssistantOperations) OpenDocument(c context.Context, i agent.ToolInput) (any, error) {
	return o.delegate(c, i, func(op agent.WorkspaceOperations, c context.Context, i agent.ToolInput) (any, error) {
		return op.OpenDocument(c, i)
	})
}
func (o appAssistantOperations) EditDocument(c context.Context, i agent.ToolInput) (any, error) {
	return o.delegate(c, i, func(op agent.WorkspaceOperations, c context.Context, i agent.ToolInput) (any, error) {
		return op.EditDocument(c, i)
	})
}
func (o appAssistantOperations) InsertIntoDocument(c context.Context, i agent.ToolInput) (any, error) {
	return o.delegate(c, i, func(op agent.WorkspaceOperations, c context.Context, i agent.ToolInput) (any, error) {
		return op.InsertIntoDocument(c, i)
	})
}
func (o appAssistantOperations) WriteDocument(c context.Context, i agent.ToolInput) (any, error) {
	return o.delegate(c, i, func(op agent.WorkspaceOperations, c context.Context, i agent.ToolInput) (any, error) {
		return op.WriteDocument(c, i)
	})
}
func (o appAssistantOperations) RenameDocument(c context.Context, i agent.ToolInput) (any, error) {
	return o.delegate(c, i, func(op agent.WorkspaceOperations, c context.Context, i agent.ToolInput) (any, error) {
		return op.RenameDocument(c, i)
	})
}
func (o appAssistantOperations) DeleteDocument(c context.Context, i agent.ToolInput) (any, error) {
	return o.delegate(c, i, func(op agent.WorkspaceOperations, c context.Context, i agent.ToolInput) (any, error) {
		return op.DeleteDocument(c, i)
	})
}
func (o appAssistantOperations) CreateFolder(c context.Context, i agent.ToolInput) (any, error) {
	return o.delegate(c, i, func(op agent.WorkspaceOperations, c context.Context, i agent.ToolInput) (any, error) {
		return op.CreateFolder(c, i)
	})
}

type appAgentPolicy struct{ panel *assistantPanel }

func (p appAgentPolicy) Enabled(settings config.AssistantSettings, name string) bool {
	return agent.ToolEnabled(settings, name)
}
func (p appAgentPolicy) RequiresApproval(name string) bool { return agent.ToolRequiresApproval(name) }
func (p appAgentPolicy) Permission() agent.Permission {
	if p.panel == nil {
		return agent.ReadOnly
	}
	return p.panel.permission
}
func (p appAgentPolicy) Schemas(_ config.AssistantSettings) []map[string]any {
	if p.panel == nil {
		return nil
	}
	return agent.ToolSchemas(p.panel.settings)
}

var _ agent.Operations = appAssistantOperations{}
var _ agent.WorkspaceMutationPort = (*appAssistantWorkspace)(nil)
