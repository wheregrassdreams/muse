package nativeui

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	conversation "muse/agent"
	applicationagent "muse/application/agent"
	applicationvault "muse/application/vault"
	"muse/config"
	"muse/document"
	doctypes "muse/document/types"
	"muse/locales"
	mdoc "muse/markdown"
	componenticons "muse/nativeui/components/icons"
	agentpanel "muse/nativeui/workbench/features/agent"
	markdownui "muse/nativeui/workbench/features/markdown"
	settingsui "muse/nativeui/workbench/features/settings"
)

// assistantPanel is the desktop adapter around the application-owned session.
// It keeps only transient presentation state; transcript and stream lifecycle
// live in application/agent.Session.
type assistantPanel struct {
	session           *applicationagent.Session
	viewState         *agentpanel.Handle
	settings          config.AssistantSettings
	effects           map[string]struct{}
	markdownOptions   func(width float32) markdownui.Options
	openSettings      func()
	openImageDocument func(path string)
	openDocument      func(path string)
	insertAtStart     func(text string)
	insertAtEnd       func(text string)
}

// assistantController is the opaque lifecycle/render handle retained by App.
// The App can route the session stream and lifetime without knowing the
// presenter implementation or its transient state.
type assistantController interface {
	Render(c *ui.Context, settings config.AssistantSettings)
	SetClient(client applicationagent.StreamClient) error
	Close()
}

func newAssistantPanel(app *App) assistantController {
	panel := &assistantPanel{viewState: agentpanel.NewHandle()}
	var client applicationagent.StreamClient
	if app != nil {
		client = app.assistantStream
	}
	panel.settings = app.ai
	panel.effects = make(map[string]struct{})
	panel.markdownOptions = func(width float32) markdownui.Options { return assistantMarkdownOptions(app, width) }
	panel.openSettings = func() { app.settingsPanel.OpenAt(settingsui.AssistantPage) }
	panel.openImageDocument = func(path string) { app.openImageDocument(path, true) }
	panel.openDocument = app.requestOpen
	panel.insertAtStart = app.insertAIContentAtStart
	panel.insertAtEnd = app.insertAIContent
	panel.session = applicationagent.NewSession(app.ai, client, applicationagent.Capabilities{
		Tools:  applicationagent.Executor{Operations: applicationagent.DynamicWorkspaceOperations{Provider: appAssistantWorkspaceProvider{app: app}}},
		Policy: applicationagent.DefaultToolPolicy{},
	})
	return panel
}

func (p *assistantPanel) Render(c *ui.Context, settings config.AssistantSettings) {
	if p == nil || p.session == nil {
		return
	}
	if p.settings != settings {
		p.settings = settings
		p.session.SetSettings(settings)
	}
	snapshot := p.session.Snapshot()
	model := agentpanel.View{Busy: snapshot.Busy, Phase: snapshot.Phase, Error: snapshot.Error,
		Icon:        componenticons.AI(),
		Suggestions: []string{locales.AiSuggestionSummarize(), locales.AiSuggestionFormalize(), locales.AiSuggestionTitles()},
		RenderMarkdown: func(c *ui.Context, source string, width float32) {
			doc := mdoc.Parse(source)
			markdownui.BlocksView(c, doc.Blocks, p.markdownOptions(width))
		}}
	if strings.TrimSpace(settings.BaseURL) == "" || strings.TrimSpace(settings.Model) == "" {
		model.SetupTitle, model.SetupDetail, model.SetupButton = locales.AiSetupModelTitle(), locales.AiSetupModelDetail(), locales.AiSetupModel()
	}
	for _, message := range snapshot.Messages {
		role := agentpanel.Assistant
		if message.Role == conversation.User {
			role = agentpanel.User
		}
		out := agentpanel.Message{Role: role, Text: message.Content}
		for _, part := range message.Parts {
			p.applyEffect(part)
			kind := agentpanel.TextPart
			if part.Kind == conversation.ThoughtPart {
				kind = agentpanel.ThoughtPart
			}
			if part.Kind == conversation.ToolPart {
				kind = agentpanel.ToolPart
			}
			status := agentpanel.ToolRunning
			switch part.Tool.Status {
			case conversation.PartApproval:
				status = agentpanel.ToolNeedsApproval
			case conversation.PartSucceeded:
				status = agentpanel.ToolSucceeded
			case conversation.PartFailed:
				status = agentpanel.ToolFailed
			}
			out.Parts = append(out.Parts, agentpanel.Part{Kind: kind, Text: part.Text, Thinking: part.Duration, Expanded: part.Expanded, Tool: agentpanel.Tool{ID: part.Tool.ID, Name: part.Tool.Name, Target: part.Tool.Arguments, Status: status, Summary: part.Tool.Result, Details: part.Tool.Error}})
		}
		model.Messages = append(model.Messages, out)
	}
	agentpanel.Render(c, p.viewState, model, func(action agentpanel.Action) {
		switch action.Kind {
		case agentpanel.Send:
			_ = p.session.Send(action.Text)
		case agentpanel.Stop:
			p.session.Stop()
		case agentpanel.ClearConversation:
			if p.session.Clear() == nil {
				p.effects = make(map[string]struct{})
				p.viewState.ResetScroll()
			}
		case agentpanel.OpenSettings:
			if p.openSettings != nil {
				p.openSettings()
			}
		case agentpanel.ApproveTool:
			_ = p.session.DecideApproval(action.ToolID, applicationagent.Approve)
		case agentpanel.AlwaysAllowTool:
			_ = p.session.DecideApproval(action.ToolID, applicationagent.AlwaysApprove)
		case agentpanel.RejectTool:
			_ = p.session.DecideApproval(action.ToolID, applicationagent.Reject)
		case agentpanel.TogglePart:
			p.session.TogglePart(action.MessageIndex, action.PartIndex)
		}
	})
	if snapshot.Busy {
		c.After(60 * time.Millisecond)
	}
}

func (p *assistantPanel) SetClient(client applicationagent.StreamClient) error {
	if p == nil || p.session == nil {
		return applicationagent.ErrUnavailable
	}
	return p.session.SetClient(client)
}

func (p *assistantPanel) Close() {
	if p != nil && p.session != nil {
		p.session.Close()
	}
}

func assistantMarkdownOptions(app *App, width float32) markdownui.Options {
	if app == nil {
		return markdownui.Options{}
	}
	return markdownui.Options{
		BodyFontSize: 12.5, SoftBreaksAsSpaces: true, ContentWidth: width,
		BaseDir: app.docDir(), AssetRoot: app.root, LocalFiles: app.imageLocalFiles,
		ImageCacheStore: app.imageCacheStore, OnImagePreview: app.openImagePreview,
		Padding: 0, IndentStep: 14,
	}
}

func (a *App) aiView(c *ui.Context) {
	if a.assistant != nil {
		a.assistant.Render(c, a.ai)
	}
}

func (p *assistantPanel) applyEffect(part conversation.Part) {
	if p == nil || part.Kind != conversation.ToolPart || part.Tool.Status != conversation.PartSucceeded || part.Tool.Result == "" {
		return
	}
	var effect applicationagent.ToolEffect
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
	case applicationagent.OpenDocumentEffect:
		if doctypes.IsImagePath(effect.Path) {
			if p.openImageDocument != nil {
				p.openImageDocument(effect.Path)
			}
		} else if p.openDocument != nil {
			p.openDocument(effect.Path)
		}
	case applicationagent.InsertDocumentEffect:
		if effect.Position == "start" {
			if p.insertAtStart != nil {
				p.insertAtStart(effect.Text)
			}
		} else if p.insertAtEnd != nil {
			p.insertAtEnd(effect.Text)
		}
	}
}

func (a *App) insertAIContentAtStart(text string) {
	if a == nil || a.noteEditor() == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	a.noteEditor().FormattingCommands().PrependBlock(text)
	a.notice(locales.DocumentInserted())
	a.invalidate()
}

type appAssistantWorkspaceProvider struct{ app *App }

func (p appAssistantWorkspaceProvider) Workspace(ctx context.Context) (applicationagent.WorkspaceMutationPort, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.app == nil {
		return nil, applicationagent.ErrUnavailable
	}
	var root, path, source string
	p.app.onMain(func() {
		root, path, source = p.app.root, p.app.documentPath(), p.app.documentSource()
	})
	if root == "" {
		return nil, errors.New("workspace is unavailable")
	}
	if p.app.newAgentWorkspace == nil {
		return nil, errors.New("assistant workspace adapter is not configured")
	}
	workspace, err := p.app.newAgentWorkspace(root, func() (string, string) { return path, source })
	if err != nil {
		return nil, err
	}
	directory, _ := workspace.(applicationagent.WorkspaceDirectoryPort)
	return &appAssistantWorkspace{app: p.app, WorkspacePort: workspace, Directory: directory}, nil
}

type appAssistantWorkspace struct {
	app *App
	applicationagent.WorkspacePort
	Directory applicationagent.WorkspaceDirectoryPort
}

func (w *appAssistantWorkspace) DestinationRevision(path string) (string, error) {
	revision, err := w.app.destinationRevision(path)
	return string(revision), err
}

func (w *appAssistantWorkspace) CurrentDocumentDirty() bool {
	var dirty bool
	w.app.onMain(func() { dirty = w.app.documentDirty() })
	return dirty
}

func (w *appAssistantWorkspace) SaveDocument(path, content, expected string) error {
	return w.app.saveExternalDocument(path, content, document.Revision(expected))
}

func (w *appAssistantWorkspace) ApplyCurrentDocument(content string) error {
	var err error
	w.app.onMain(func() {
		if w.app.documentPath() == "" && w.app.documentSource() == "" {
			err = errors.New("current document is empty")
			return
		}
		w.app.editDoc(content)
	})
	return err
}

func (w *appAssistantWorkspace) MoveDocument(root, source, target string) (string, error) {
	files := applicationvault.NewFileUseCases(root, w.app.fileOperations, w.app.isVaultDirectory)
	mutation, err := files.MoveFileTo(source, target)
	if err != nil {
		return "", err
	}
	if len(mutation.Moves) == 0 {
		return "", applicationvault.ErrFileOperationsUnavailable
	}
	w.app.onMain(func() {
		w.app.clearTreeCache()
		w.app.remapTreePaths(mutation.Moves)
		w.app.addRecent(target, false)
		w.app.invalidate()
	})
	w.refresh(source, target)
	return mutation.Moves[0].Target, nil
}

func (w *appAssistantWorkspace) TrashDocuments(root string, paths []string) error {
	files := applicationvault.NewFileUseCases(root, w.app.fileOperations, w.app.isVaultDirectory)
	mutation, err := files.TrashFiles(paths)
	if err != nil {
		return err
	}
	w.app.onMain(func() {
		for _, path := range mutation.Trashed {
			w.app.clearTreeCache()
			if filepath.Clean(path) == filepath.Clean(w.app.documentPath()) {
				w.app.resetDoc()
			}
		}
		w.app.invalidate()
	})
	w.refresh(mutation.Trashed...)
	return nil
}

func (w *appAssistantWorkspace) CreateFolder(path string) error {
	if w.app == nil {
		return applicationvault.ErrFileOperationsUnavailable
	}
	root := ""
	w.app.onMain(func() { root = w.app.root })
	files := applicationvault.NewFileUseCases(root, w.app.fileOperations, w.app.isVaultDirectory)
	if _, err := files.CreateFolderAt(path); err != nil {
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

var _ applicationagent.WorkspaceMutationPort = (*appAssistantWorkspace)(nil)
