package agent

import (
	"github.com/egoist/mygo/ui"
	applicationagent "muse/application/agent"
	"muse/config"
)

// HostOptions supplies the application session and the small set of host
// capabilities needed by the assistant feature. The feature owns its
// transient presentation state, settings snapshot, provider selection, and
// model discovery lifecycle; the caller owns construction of the session and
// adapters for documents, settings storage, and notifications.
type HostOptions struct {
	Session  Session
	Settings config.AssistantSettings
	Ports    Ports
}

// Host is the public boundary for embedding the assistant feature in a
// workbench. It has no dependency on the workbench shell or its App type.
type Host struct {
	controller *Controller
	settings   config.AssistantSettings
}

func NewHost(options HostOptions) *Host {
	controller := NewController(ControllerOptions{
		Session: options.Session, Settings: options.Settings, Ports: options.Ports,
	})
	controller.SetSettings(options.Settings)
	return &Host{controller: controller, settings: options.Settings}
}

func (h *Host) Render(ctx *ui.Context) {
	if h == nil || h.controller == nil {
		return
	}
	h.controller.Render(ctx, h.settings)
}

func (h *Host) Dispatch(action Action) {
	if h != nil && h.controller != nil {
		h.controller.Dispatch(action)
	}
}

func (h *Host) Settings() config.AssistantSettings {
	if h == nil {
		return config.AssistantSettings{}
	}
	return h.settings
}

func (h *Host) SetSettings(settings config.AssistantSettings) {
	if h == nil {
		return
	}
	h.settings = settings
	if h.controller != nil {
		h.controller.SetSettings(settings)
	}
}

func (h *Host) SelectProvider(magpie bool) config.AssistantSettings {
	if h == nil || h.controller == nil {
		return config.AssistantSettings{}
	}
	h.settings = h.controller.SelectProvider(h.settings, magpie)
	h.controller.SetSettings(h.settings)
	return h.settings
}

func (h *Host) SaveSettings() error {
	if h == nil || h.controller == nil {
		return applicationagent.ErrUnavailable
	}
	return h.controller.SaveSettings(h.settings)
}

func (h *Host) SetClient(client applicationagent.StreamClient) error {
	if h == nil || h.controller == nil {
		return applicationagent.ErrUnavailable
	}
	return h.controller.SetClient(client)
}

func (h *Host) SetModelProvider(provider ModelProvider) {
	if h != nil && h.controller != nil {
		h.controller.SetModelProvider(provider)
	}
}

func (h *Host) FetchModels() {
	if h != nil && h.controller != nil {
		h.controller.FetchModels(h.settings)
	}
}

func (h *Host) ModelCatalogSnapshot() applicationagent.ModelCatalogSnapshot {
	if h == nil || h.controller == nil {
		return applicationagent.ModelCatalogSnapshot{}
	}
	return h.controller.ModelCatalogSnapshot()
}

func (h *Host) Close() {
	if h != nil && h.controller != nil {
		h.controller.Close()
	}
}
