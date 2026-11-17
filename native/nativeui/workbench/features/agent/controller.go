package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	transcript "muse/agent"
	applicationagent "muse/application/agent"
	"muse/config"
	"muse/document/types"
	"muse/locales"
)

// Session is the application-owned conversation capability consumed by the
// assistant panel. Tool execution, provider protocols, and session lifecycle
// stay in application/agent.
type Session interface {
	Snapshot() applicationagent.Snapshot
	SetSettings(config.AssistantSettings)
	SetClient(applicationagent.StreamClient) error
	Send(string) error
	Stop()
	Clear() error
	DecideApproval(string, applicationagent.Decision) error
	TogglePart(messageIndex, partIndex int)
	Close()
}

// ModelProvider is the one provider capability needed by the panel settings
// workflow. Provider health checks remain owned by the settings/application
// composition that invokes them.
type ModelProvider interface {
	ListModels(context.Context, applicationagent.ProviderCredentials) (applicationagent.ProviderModelList, error)
}

// Ports are application actions needed by the assistant presentation. The
// feature never stores an App or reaches into editor, vault, or settings state.
type Ports struct {
	Icon              func() *ui.SVG
	RenderMarkdown    func(*ui.Context, string, float32)
	OpenSettings      func()
	OpenImageDocument func(path string)
	OpenDocument      func(path string)
	InsertAtStart     func(text string)
	InsertAtEnd       func(text string)
	SaveSettings      func(config.AssistantSettings) error
	Provider          ModelProvider
	RunOnMain         func(func())
	SetDefaultModel   func(string)
	Notify            func(string)
	Invalidate        func()
}

type ControllerOptions struct {
	Session  Session
	Settings config.AssistantSettings
	Ports    Ports
}

// Controller joins the application conversation session to the assistant
// presentation. It owns only transient view state and one-time tool effects.
type Controller struct {
	session      Session
	viewState    *Handle
	settings     config.AssistantSettings
	hasSettings  bool
	effects      map[string]struct{}
	ports        Ports
	catalog      *applicationagent.ModelCatalog
	modelMu      sync.Mutex
	modelCancel  context.CancelFunc
	modelRequest uint64
}

func NewController(options ControllerOptions) *Controller {
	return &Controller{
		session: options.Session, viewState: NewHandle(), settings: options.Settings,
		hasSettings: true, effects: make(map[string]struct{}), ports: options.Ports,
		catalog: &applicationagent.ModelCatalog{},
	}
}

func (c *Controller) Render(ctx *ui.Context, settings config.AssistantSettings) {
	if c == nil || c.session == nil || ctx == nil {
		return
	}
	if !c.hasSettings || c.settings != settings {
		c.settings, c.hasSettings = settings, true
		c.session.SetSettings(settings)
	}
	snapshot := c.session.Snapshot()
	model := View{
		Busy: snapshot.Busy, Phase: snapshot.Phase, Error: snapshot.Error,
		Suggestions: []string{
			locales.AiSuggestionSummarize(),
			locales.AiSuggestionFormalize(),
			locales.AiSuggestionTitles(),
		},
		RenderMarkdown: c.ports.RenderMarkdown,
	}
	if c.ports.Icon != nil {
		model.Icon = c.ports.Icon()
	}
	if strings.TrimSpace(settings.BaseURL) == "" || strings.TrimSpace(settings.Model) == "" {
		model.SetupTitle, model.SetupDetail, model.SetupButton = locales.AiSetupModelTitle(), locales.AiSetupModelDetail(), locales.AiSetupModel()
	}
	for _, message := range snapshot.Messages {
		role := Assistant
		if message.Role == transcript.User {
			role = User
		}
		out := Message{Role: role, Text: message.Content}
		for _, part := range message.Parts {
			c.applyEffect(part)
			kind := TextPart
			if part.Kind == transcript.ThoughtPart {
				kind = ThoughtPart
			} else if part.Kind == transcript.ToolPart {
				kind = ToolPart
			}
			status := ToolRunning
			switch part.Tool.Status {
			case transcript.PartApproval:
				status = ToolNeedsApproval
			case transcript.PartSucceeded:
				status = ToolSucceeded
			case transcript.PartFailed:
				status = ToolFailed
			}
			out.Parts = append(out.Parts, Part{
				Kind: kind, Text: part.Text, Thinking: part.Duration, Expanded: part.Expanded,
				Tool: Tool{ID: part.Tool.ID, Name: part.Tool.Name, Target: part.Tool.Arguments,
					Status: status, Summary: part.Tool.Result, Details: part.Tool.Error},
			})
		}
		model.Messages = append(model.Messages, out)
	}
	Render(ctx, c.viewState, model, c.Dispatch)
	if snapshot.Busy {
		ctx.After(60 * time.Millisecond)
	}
}

func (c *Controller) Dispatch(action Action) {
	if c == nil || c.session == nil {
		return
	}
	switch action.Kind {
	case Send:
		_ = c.session.Send(action.Text)
	case Stop:
		c.session.Stop()
	case ClearConversation:
		if c.session.Clear() == nil && c.viewState != nil {
			c.effects = make(map[string]struct{})
			c.viewState.ResetScroll()
		}
	case OpenSettings:
		if c.ports.OpenSettings != nil {
			c.ports.OpenSettings()
		}
	case ApproveTool:
		_ = c.session.DecideApproval(action.ToolID, applicationagent.Approve)
	case AlwaysAllowTool:
		_ = c.session.DecideApproval(action.ToolID, applicationagent.AlwaysApprove)
	case RejectTool:
		_ = c.session.DecideApproval(action.ToolID, applicationagent.Reject)
	case TogglePart:
		c.session.TogglePart(action.MessageIndex, action.PartIndex)
	}
}

func (c *Controller) SetClient(client applicationagent.StreamClient) error {
	if c == nil || c.session == nil {
		return applicationagent.ErrUnavailable
	}
	return c.session.SetClient(client)
}

// SetSettings synchronizes settings loaded by the host before the next panel
// frame is rendered.
func (c *Controller) SetSettings(settings config.AssistantSettings) {
	if c == nil {
		return
	}
	c.settings, c.hasSettings = settings, true
	if c.session != nil {
		c.session.SetSettings(settings)
	}
}

func (c *Controller) Close() {
	if c == nil {
		return
	}
	c.cancelModelRequest()
	if c.catalog != nil {
		c.catalog.Reset()
	}
	if c.session != nil {
		c.session.Close()
	}
}

// SelectProvider applies the assistant provider preset while preserving a
// custom endpoint when the selected provider remains unchanged.
func (c *Controller) SelectProvider(settings config.AssistantSettings, magpie bool) config.AssistantSettings {
	if settings.UsesMagpie() == magpie {
		return settings
	}
	c.cancelModelRequest()
	if magpie {
		settings.Provider = config.AssistantProviderMagpie
		settings.BaseURL = config.AssistantMagpieBaseURL
	} else {
		settings.Provider = config.AssistantProviderCustom
		if settings.BaseURL == config.AssistantMagpieBaseURL {
			settings.BaseURL = ""
		}
	}
	settings.Model = ""
	c.catalog.Reset()
	return settings
}

func (c *Controller) SaveSettings(settings config.AssistantSettings) error {
	if c == nil || c.ports.SaveSettings == nil {
		return applicationagent.ErrUnavailable
	}
	return c.ports.SaveSettings(settings)
}

func (c *Controller) ModelCatalogSnapshot() applicationagent.ModelCatalogSnapshot {
	if c == nil {
		return applicationagent.ModelCatalogSnapshot{}
	}
	return c.catalog.Snapshot()
}

// FetchModels owns model discovery cancellation and request deduplication.
// Results return through RunOnMain before the catalog and selected model are
// changed, so UI-owned settings are only touched on their owner thread.
func (c *Controller) FetchModels(settings config.AssistantSettings) {
	if c == nil || c.catalog == nil {
		return
	}
	generation, begun := c.catalog.Begin()
	if !begun {
		return
	}
	c.modelMu.Lock()
	if c.modelCancel != nil {
		c.modelCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	c.modelCancel, c.modelRequest = cancel, generation
	c.modelMu.Unlock()
	c.invalidate()
	if c.ports.Provider == nil {
		c.completeModels(generation, nil, locales.AiServiceRequestFailed(0))
		return
	}
	credentials := applicationagent.ProviderCredentials{BaseURL: settings.BaseURL, APIKey: settings.APIKey}
	go func() {
		result, providerErr := c.ports.Provider.ListModels(ctx, credentials)
		complete := func() {
			c.modelMu.Lock()
			if c.modelRequest == generation {
				cancel()
				c.modelCancel = nil
			}
			c.modelMu.Unlock()
			var models []applicationagent.ProviderModel
			message := ""
			if providerErr == nil {
				models = result.Models
				if len(models) == 0 {
					message = locales.AiNoModelsReturned()
				}
			} else {
				message = providerErrorMessage(providerErr)
			}
			if c.catalog.Complete(generation, models, message) {
				if providerErr == nil && strings.TrimSpace(settings.Model) == "" && len(models) > 0 && c.ports.SetDefaultModel != nil {
					c.ports.SetDefaultModel(models[0].ID)
				}
				c.invalidate()
			}
		}
		if c.ports.RunOnMain != nil {
			c.ports.RunOnMain(complete)
		} else {
			complete()
		}
	}()
}

func (c *Controller) completeModels(generation uint64, models []applicationagent.ProviderModel, message string) {
	if c.catalog.Complete(generation, models, message) {
		c.invalidate()
	}
}

func (c *Controller) cancelModelRequest() {
	if c == nil {
		return
	}
	c.modelMu.Lock()
	if c.modelCancel != nil {
		c.modelCancel()
		c.modelCancel = nil
	}
	c.modelRequest = 0
	c.modelMu.Unlock()
}

func (c *Controller) InsertAtStart(text string) {
	if c == nil || c.ports.InsertAtStart == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	c.ports.InsertAtStart(text)
	if c.ports.Notify != nil {
		c.ports.Notify(locales.DocumentInserted())
	}
	c.invalidate()
}

func (c *Controller) InsertAtEnd(text string) {
	if c == nil || c.ports.InsertAtEnd == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	c.ports.InsertAtEnd(text)
	if c.ports.Notify != nil {
		c.ports.Notify(locales.DocumentInserted())
	}
	c.invalidate()
}

func (c *Controller) invalidate() {
	if c != nil && c.ports.Invalidate != nil {
		c.ports.Invalidate()
	}
}

func providerErrorMessage(err error) string {
	var providerErr *applicationagent.ProviderError
	if !errors.As(err, &providerErr) {
		return err.Error()
	}
	switch providerErr.Kind {
	case applicationagent.ProviderErrorInvalidConfiguration:
		return locales.AiServiceFillBaseURL()
	case applicationagent.ProviderErrorTimeout:
		return locales.AiServiceTimeout(10)
	case applicationagent.ProviderErrorNetwork:
		return locales.AiServiceUnreachable()
	case applicationagent.ProviderErrorUnauthorized:
		return locales.AiServiceAuthFailure()
	case applicationagent.ProviderErrorRequest:
		if providerErr.Detail != "" {
			return providerErr.Detail
		}
		return locales.AiServiceRequestFailed(providerErr.StatusCode)
	case applicationagent.ProviderErrorReadResponse:
		return locales.AiServiceReadModelsFailed()
	case applicationagent.ProviderErrorInvalidModelList:
		return locales.AiServiceInvalidModelList()
	default:
		return err.Error()
	}
}

func (c *Controller) applyEffect(part transcript.Part) {
	if c == nil || part.Kind != transcript.ToolPart || part.Tool.Status != transcript.PartSucceeded || part.Tool.Result == "" {
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
	if _, exists := c.effects[key]; exists {
		return
	}
	c.effects[key] = struct{}{}
	switch effect.Kind {
	case applicationagent.OpenDocumentEffect:
		if types.IsImagePath(effect.Path) {
			if c.ports.OpenImageDocument != nil {
				c.ports.OpenImageDocument(effect.Path)
			}
		} else if c.ports.OpenDocument != nil {
			c.ports.OpenDocument(effect.Path)
		}
	case applicationagent.InsertDocumentEffect:
		if effect.Position == "start" {
			if c.ports.InsertAtStart != nil {
				c.ports.InsertAtStart(effect.Text)
			}
		} else if c.ports.InsertAtEnd != nil {
			c.ports.InsertAtEnd(effect.Text)
		}
	}
}
