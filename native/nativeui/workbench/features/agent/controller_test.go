package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	transcript "muse/agent"
	applicationagent "muse/application/agent"
	"muse/config"
)

type fakeSession struct {
	snapshot  applicationagent.Snapshot
	settings  config.AssistantSettings
	client    applicationagent.StreamClient
	sent      []string
	decisions []applicationagent.Decision
	toggled   [][2]int
	stops     int
	clears    int
	closed    int
	clearErr  error
}

func (s *fakeSession) Snapshot() applicationagent.Snapshot           { return s.snapshot }
func (s *fakeSession) SetSettings(settings config.AssistantSettings) { s.settings = settings }
func (s *fakeSession) SetClient(client applicationagent.StreamClient) error {
	s.client = client
	return nil
}
func (s *fakeSession) Send(text string) error { s.sent = append(s.sent, text); return nil }
func (s *fakeSession) Stop()                  { s.stops++ }
func (s *fakeSession) Clear() error           { s.clears++; return s.clearErr }
func (s *fakeSession) DecideApproval(_ string, decision applicationagent.Decision) error {
	s.decisions = append(s.decisions, decision)
	return nil
}
func (s *fakeSession) TogglePart(message, part int) {
	s.toggled = append(s.toggled, [2]int{message, part})
}
func (s *fakeSession) Close() { s.closed++ }

func TestControllerDispatchesPanelActionsToSessionAndPorts(t *testing.T) {
	session := &fakeSession{}
	settingsOpened := 0
	controller := NewController(ControllerOptions{Session: session, Ports: Ports{
		OpenSettings: func() { settingsOpened++ },
	}})
	controller.Dispatch(Action{Kind: Send, Text: "summarize this"})
	controller.Dispatch(Action{Kind: Stop})
	controller.Dispatch(Action{Kind: OpenSettings})
	controller.Dispatch(Action{Kind: ApproveTool, ToolID: "tool-1"})
	controller.Dispatch(Action{Kind: AlwaysAllowTool, ToolID: "tool-2"})
	controller.Dispatch(Action{Kind: RejectTool, ToolID: "tool-3"})
	controller.Dispatch(Action{Kind: TogglePart, MessageIndex: 2, PartIndex: 4})
	if len(session.sent) != 1 || session.sent[0] != "summarize this" || session.stops != 1 || settingsOpened != 1 {
		t.Fatalf("actions not routed: sent=%v stops=%d settings=%d", session.sent, session.stops, settingsOpened)
	}
	if len(session.decisions) != 3 || session.decisions[0] != applicationagent.Approve || session.decisions[1] != applicationagent.AlwaysApprove || session.decisions[2] != applicationagent.Reject {
		t.Fatalf("approval decisions=%v", session.decisions)
	}
	if len(session.toggled) != 1 || session.toggled[0] != [2]int{2, 4} {
		t.Fatalf("toggled parts=%v", session.toggled)
	}
}

func TestControllerAppliesToolEffectsOnceAndClearsEffectHistory(t *testing.T) {
	var openedDocs, openedImages, inserted []string
	session := &fakeSession{}
	controller := NewController(ControllerOptions{Session: session, Ports: Ports{
		OpenDocument:      func(path string) { openedDocs = append(openedDocs, path) },
		OpenImageDocument: func(path string) { openedImages = append(openedImages, path) },
		InsertAtStart:     func(text string) { inserted = append(inserted, "start:"+text) },
		InsertAtEnd:       func(text string) { inserted = append(inserted, "end:"+text) },
	}})
	parts := []transcript.Part{
		{Kind: transcript.ToolPart, Tool: transcript.ToolPartState{ID: "doc", Status: transcript.PartSucceeded, Result: `{"kind":"open_document","path":"notes.md"}`}},
		{Kind: transcript.ToolPart, Tool: transcript.ToolPartState{ID: "image", Status: transcript.PartSucceeded, Result: `{"kind":"open_document","path":"diagram.png"}`}},
		{Kind: transcript.ToolPart, Tool: transcript.ToolPartState{ID: "insert", Status: transcript.PartSucceeded, Result: `{"kind":"insert_document","position":"start","text":"  new block  "}`}},
	}
	for _, part := range parts {
		controller.applyEffect(part)
		controller.applyEffect(part)
	}
	if len(openedDocs) != 1 || openedDocs[0] != "notes.md" || len(openedImages) != 1 || openedImages[0] != "diagram.png" {
		t.Fatalf("open effects docs=%v images=%v", openedDocs, openedImages)
	}
	if len(inserted) != 1 || inserted[0] != "start:  new block  " {
		t.Fatalf("insert effects=%v", inserted)
	}
	controller.Dispatch(Action{Kind: ClearConversation})
	controller.applyEffect(parts[0])
	if len(openedDocs) != 2 {
		t.Fatalf("clear did not reset one-time effect history: %v", openedDocs)
	}
}

func TestControllerOwnsAssistantSettingsProviderAndModelCatalog(t *testing.T) {
	saved := config.AssistantSettings{}
	provider := &fakeProvider{models: applicationagent.ProviderModelList{Models: []applicationagent.ProviderModel{{ID: "model-a"}}}}
	selected := ""
	invalidations := 0
	controller := NewController(ControllerOptions{Session: &fakeSession{}, Ports: Ports{
		SaveSettings:    func(settings config.AssistantSettings) error { saved = settings; return nil },
		Provider:        provider,
		RunOnMain:       func(fn func()) { fn() },
		SetDefaultModel: func(model string) { selected = model },
		Invalidate:      func() { invalidations++ },
	}})
	settings := controller.SelectProvider(config.AssistantSettings{Provider: config.AssistantProviderMagpie, BaseURL: config.AssistantMagpieBaseURL}, false)
	if settings.Provider != config.AssistantProviderCustom || settings.BaseURL != "" || settings.Model != "" {
		t.Fatalf("custom provider selection=%+v", settings)
	}
	if err := controller.SaveSettings(settings); err != nil || saved != settings {
		t.Fatalf("settings save error=%v value=%+v", err, saved)
	}
	controller.FetchModels(config.AssistantSettings{BaseURL: "https://example.test", APIKey: "key"})
	deadline := time.Now().Add(time.Second)
	for controller.ModelCatalogSnapshot().Loading && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	catalog := controller.ModelCatalogSnapshot()
	if catalog.Loading || catalog.Error != "" || len(catalog.Models) != 1 || catalog.Models[0].ID != "model-a" || selected != "model-a" {
		t.Fatalf("model catalog=%+v selected=%q", catalog, selected)
	}
	controller.Close()
	if !provider.called || invalidations == 0 {
		t.Fatalf("provider called=%v invalidations=%d", provider.called, invalidations)
	}
}

type fakeProvider struct {
	models applicationagent.ProviderModelList
	err    error
	called bool
}

func (p *fakeProvider) TestConnection(context.Context, applicationagent.ProviderCredentials) (applicationagent.ProviderCheck, error) {
	return applicationagent.ProviderCheck{}, nil
}

func (p *fakeProvider) ListModels(context.Context, applicationagent.ProviderCredentials) (applicationagent.ProviderModelList, error) {
	p.called = true
	return p.models, p.err
}

func TestControllerProviderErrorsUseLocalizedModelMessage(t *testing.T) {
	if got := providerErrorMessage(errors.New("network unavailable")); got != "network unavailable" {
		t.Fatalf("unclassified provider error=%q", got)
	}
}
