package nativeui

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/egoist/mygo/ui"
	"muse/application/agent"
	"muse/config"
	"muse/locales"
	mdoc "muse/markdown"
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
}

func newAssistantPanel(app *App) *assistantPanel {
	panel := &assistantPanel{}
	var client agent.StreamClient
	if app != nil && app.aiClient != nil {
		client = agent.NewProviderClient(app.aiClient)
	}
	panel.permission = agent.AskBeforeChanges
	panel.settings = app.ai
	panel.session = agent.NewSession(app.ai, client, agent.Capabilities{Tools: agent.Executor{Operations: appAssistantOperations{app}}, Policy: appAgentPolicy{panel: panel}})
	return panel
}

func (p *assistantPanel) render(c *ui.Context, app *App) {
	if p == nil || p.session == nil {
		return
	}
	snapshot := p.session.Snapshot()
	model := agentpanel.ViewModel{Draft: p.draft, Busy: snapshot.Busy, Phase: snapshot.Phase, Error: snapshot.Error, Scroll: &p.scroll,
		Icon: iconAI, ShowScrollbar: app.aiIndicator.Visible(c.Now(), p.scroll.X, p.scroll.Y),
		Suggestions: []string{locales.AiSuggestionSummarize(), locales.AiSuggestionFormalize(), locales.AiSuggestionTitles()},
		RenderMarkdown: func(c *ui.Context, source string, width float32) {
			mdoc.BlocksView(c, mdoc.Parse(source).Blocks, mdoc.Options{BodyFontSize: 12.5, SoftBreaksAsSpaces: true, ContentWidth: width, BaseDir: app.docDir(), AssetRoot: app.root, OnImagePreview: app.openImagePreview, Padding: 0, IndentStep: 14})
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
			_ = p.session.Clear()
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

type appAssistantOperations struct{ app *App }

func (o appAssistantOperations) call(ctx context.Context, name string, input agent.ToolInput) (any, error) {
	if o.app == nil {
		return nil, agent.ErrUnavailable
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	out := o.app.execTool(name, string(data))
	if !out.OK {
		return nil, errors.New(out.Content)
	}
	var result any
	if err := json.Unmarshal([]byte(out.Content), &result); err != nil {
		return out.Content, nil
	}
	return result, nil
}
func (o appAssistantOperations) ListDocuments(c context.Context, i agent.ToolInput) (any, error) {
	return o.call(c, "list_documents", i)
}
func (o appAssistantOperations) ReadDocument(c context.Context, i agent.ToolInput) (any, error) {
	return o.call(c, "read_document", i)
}
func (o appAssistantOperations) SearchDocuments(c context.Context, i agent.ToolInput) (any, error) {
	return o.call(c, "search_document", i)
}
func (o appAssistantOperations) ReadOutline(c context.Context, i agent.ToolInput) (any, error) {
	return o.call(c, "read_outline", i)
}
func (o appAssistantOperations) WorkspaceTree(c context.Context, i agent.ToolInput) (any, error) {
	return o.call(c, "workspace_tree", i)
}
func (o appAssistantOperations) DocumentStats(c context.Context, i agent.ToolInput) (any, error) {
	return o.call(c, "document_stats", i)
}
func (o appAssistantOperations) OpenDocument(c context.Context, i agent.ToolInput) (any, error) {
	return o.call(c, "open_document", i)
}
func (o appAssistantOperations) EditDocument(c context.Context, i agent.ToolInput) (any, error) {
	return o.call(c, "edit_document", i)
}
func (o appAssistantOperations) InsertIntoDocument(c context.Context, i agent.ToolInput) (any, error) {
	return o.call(c, "insert_into_document", i)
}
func (o appAssistantOperations) WriteDocument(c context.Context, i agent.ToolInput) (any, error) {
	return o.call(c, "write_document", i)
}
func (o appAssistantOperations) RenameDocument(c context.Context, i agent.ToolInput) (any, error) {
	return o.call(c, "rename_document", i)
}
func (o appAssistantOperations) DeleteDocument(c context.Context, i agent.ToolInput) (any, error) {
	return o.call(c, "delete_document", i)
}
func (o appAssistantOperations) CreateFolder(c context.Context, i agent.ToolInput) (any, error) {
	return o.call(c, "create_folder", i)
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
