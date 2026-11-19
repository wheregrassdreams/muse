package agent

import (
	"testing"
	"time"

	applicationagent "muse/application/agent"
	"muse/config"
)

func TestHostOwnsSettingsAndProviderSelection(t *testing.T) {
	settings := config.AssistantSettings{
		Provider: config.AssistantProviderMagpie,
		BaseURL:  config.AssistantMagpieBaseURL,
		Model:    "old-model",
	}
	session := &fakeSession{}
	var saved config.AssistantSettings
	host := NewHost(HostOptions{Session: session, Settings: settings, Ports: Ports{
		SaveSettings: func(value config.AssistantSettings) error {
			saved = value
			return nil
		},
	}})

	if session.settings != settings {
		t.Fatalf("initial settings were not provided to the session: %+v", session.settings)
	}
	selected := host.SelectProvider(false)
	if selected.Provider != config.AssistantProviderCustom || selected.BaseURL != "" || selected.Model != "" {
		t.Fatalf("provider selection did not update host settings: %+v", selected)
	}
	if session.settings != selected {
		t.Fatalf("session settings were not synchronized: %+v", session.settings)
	}
	if err := host.SaveSettings(); err != nil {
		t.Fatal(err)
	}
	if saved != selected || host.Settings() != selected {
		t.Fatalf("saved=%+v host=%+v", saved, host.Settings())
	}

	host.Close()
	if session.closed != 1 {
		t.Fatalf("session close count=%d", session.closed)
	}
}

func TestHostForwardsActionsAndModelLoading(t *testing.T) {
	provider := &fakeProvider{models: applicationagent.ProviderModelList{Models: []applicationagent.ProviderModel{{ID: "model-a"}}}}
	session := &fakeSession{}
	settings := config.AssistantSettings{
		BaseURL: "https://example.test", APIKey: "secret",
	}
	var host *Host
	host = NewHost(HostOptions{Session: session, Settings: settings, Ports: Ports{
		RunOnMain: func(fn func()) { fn() },
		SetDefaultModel: func(model string) {
			settings.Model = model
			host.SetSettings(settings)
		},
	}})
	host.SetModelProvider(provider)
	host.Dispatch(Action{Kind: Send, Text: "hello"})
	host.FetchModels()

	if len(session.sent) != 1 || session.sent[0] != "hello" {
		t.Fatalf("sent messages=%v", session.sent)
	}
	deadline := time.Now().Add(time.Second)
	for host.ModelCatalogSnapshot().Loading && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !provider.called {
		t.Fatal("provider was not called")
	}
	if got := host.ModelCatalogSnapshot(); got.Loading || got.Error != "" || len(got.Models) != 1 || got.Models[0].ID != "model-a" {
		t.Fatalf("catalog=%+v", got)
	}
}
