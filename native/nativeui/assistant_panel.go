package nativeui

import (
	"context"
	"errors"
	"strings"
	"time"

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
	session *agent.Session
	draft   string
	scroll  ui.ScrollState
}

func newAssistantPanel(app *App) *assistantPanel {
	panel := &assistantPanel{}
	var client agent.StreamClient
	if app != nil && app.aiClient != nil {
		client = agent.NewProviderClient(app.aiClient)
	}
	panel.session = agent.NewSession(app.ai, client, agent.Capabilities{Tools: appAgentTools{app}, Policy: appAgentPolicy{app}})
	legacy := make([]agent.Message, 0, len(app.aiMessages))
	for _, message := range app.aiMessages {
		role := agent.Assistant
		if message.Role == "user" {
			role = agent.User
		}
		converted := agent.Message{Role: role, Content: message.Content}
		for _, part := range message.Parts {
			kind := agent.TextPart
			if part.Kind == partThink {
				kind = agent.ThoughtPart
			}
			if part.Kind == partTool {
				kind = agent.ToolPart
			}
			tool := agent.ToolPartState{}
			if part.Tool != nil {
				tool = agent.ToolPartState{ID: part.Tool.ID, Name: part.Tool.Name, Arguments: part.Tool.Args, Result: part.Tool.Result, Error: part.Tool.Err}
			}
			converted.Parts = append(converted.Parts, agent.Part{Kind: kind, Text: part.Text, Duration: time.Duration(part.Ms) * time.Millisecond, Expanded: part.open, Tool: tool})
		}
		legacy = append(legacy, converted)
	}
	panel.session.RestoreMessages(legacy)
	panel.draft = app.aiInput
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
			app.aiPerm = permFull
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

type appAgentTools struct{ app *App }

func (t appAgentTools) Execute(_ context.Context, request agent.ToolRequest) agent.ToolResult {
	if t.app == nil {
		return agent.ToolResult{Err: agent.ErrUnavailable}
	}
	out := t.app.execTool(request.Name, request.Arguments)
	if !out.OK {
		return agent.ToolResult{Content: out.Content, Err: errors.New(out.Content)}
	}
	return agent.ToolResult{Content: out.Content}
}

type appAgentPolicy struct{ app *App }

func (p appAgentPolicy) Enabled(_ config.AssistantSettings, name string) bool {
	if p.app == nil {
		return false
	}
	group := toolGroup(name)
	return group == "" || p.app.toolGroupEnabled(group)
}
func (p appAgentPolicy) RequiresApproval(name string) bool { return toolNeedsApproval(name) }
func (p appAgentPolicy) Permission() agent.Permission {
	if p.app == nil {
		return agent.ReadOnly
	}
	switch p.app.aiPerm {
	case permFull:
		return agent.AllowChanges
	case permReadonly:
		return agent.ReadOnly
	default:
		return agent.AskBeforeChanges
	}
}
func (p appAgentPolicy) Schemas(_ config.AssistantSettings) []map[string]any {
	if p.app == nil {
		return nil
	}
	return agentToolSchemas(p.app.toolGroupEnabled)
}
