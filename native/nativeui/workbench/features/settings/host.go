package settings

import "github.com/egoist/mygo/ui"

// SnapshotPort supplies current preference and assistant-service values for
// projection. The feature retains only its local page, scroll, and dialog state.
type SnapshotPort func() ViewModel

// ActionPort routes a typed preference or assistant request to the root-owned
// application/service adapters.
type ActionPort func(Action)

type Ports struct {
	Snapshot SnapshotPort
	Dispatch ActionPort
	Native   *NativeAdapter
}

// Host owns settings navigation and rendering while the root provides current
// values and routes preference/API requests through narrow typed ports.
type Host struct {
	controller *controller
	ports      Ports
}

func NewHost(ports Ports) *Host {
	return &Host{controller: newController(), ports: ports}
}

// NewNativeHost builds the settings feature with its application config and
// MyGo preference adapter. The adapter owns persisted settings; the workbench
// only holds the feature Host and routes typed user actions to it.
func NewNativeHost(ports NativePorts) *Host {
	return NewHost(Ports{Native: NewNativeAdapter(ports)})
}

func (h *Host) OpenAt(page Page) {
	if h != nil {
		h.ensureController().OpenAt(page)
	}
}

func (h *Host) Close() {
	if h != nil {
		h.ensureController().Close()
	}
}

func (h *Host) IsOpen() bool {
	return h != nil && h.controller != nil && h.controller.IsOpen()
}

func (h *Host) Page() Page {
	if h == nil || h.controller == nil {
		return AppearancePage
	}
	return h.controller.Page()
}

func (h *Host) SetPage(page Page) {
	if h != nil {
		h.ensureController().SetPage(page)
	}
}

func (h *Host) SetScrollOffset(y float32) {
	if h != nil {
		h.ensureController().SetScrollOffset(y)
	}
}

func (h *Host) ScrollOffset() float32 {
	if h == nil || h.controller == nil {
		return 0
	}
	return h.controller.ScrollOffset()
}

// Dispatch consumes local navigation and close actions; all preference and
// assistant requests are forwarded as typed actions to the composition root.
func (h *Host) Dispatch(action Action) {
	if h == nil {
		return
	}
	h.ensureController().Apply(action)
	if action.Kind == SelectPage || action.Kind == Close {
		return
	}
	if h.ports.Native != nil {
		h.ports.Native.Dispatch(action)
		return
	}
	if h.ports.Dispatch != nil {
		h.ports.Dispatch(action)
	}
}

func (h *Host) RenderDialog(c *ui.Context) {
	if h == nil || h.controller == nil {
		return
	}
	model := ViewModel{}
	if h.ports.Native != nil {
		model = h.ports.Native.ViewModel()
	} else if h.ports.Snapshot != nil {
		model = h.ports.Snapshot()
	}
	h.controller.renderDialog(c, model, h.Dispatch)
}

// Runtime returns the current settings values used by shell/editor adapters.
// Values are copied so callers cannot mutate feature-owned settings state.
func (h *Host) Runtime() RuntimeSettings {
	if h == nil || h.ports.Native == nil {
		return RuntimeSettings{}
	}
	return h.ports.Native.Runtime()
}

// Load reads application preferences once and applies locale/editor settings
// through the feature's injected platform ports.
func (h *Host) Load() error {
	if h == nil || h.ports.Native == nil {
		return nil
	}
	return h.ports.Native.Load()
}

func (h *Host) ensureController() *controller {
	if h.controller == nil {
		h.controller = newController()
	}
	return h.controller
}
