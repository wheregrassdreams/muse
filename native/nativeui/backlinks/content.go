// Package backlinks owns the transient incoming and outgoing links surface.
package backlinks

import (
	"github.com/egoist/mygo/ui"
	workspaceui "muse/nativeui/workspace"
	"muse/ui/scrollbar"
	"muse/vault/links"
)

// Model is the immutable data needed to render one backlinks frame.
type Model struct {
	Ready    bool
	Busy     bool
	Incoming []links.BacklinkSource
	Outgoing []links.Link
}

// Ports are the narrow application capabilities used by the surface.
type Ports struct {
	Data       func() Model
	OpenSource func(path string, offset int)
	OpenTarget func(target string)
	OnClose    func()
}

// state belongs to the surface and survives frames while the overlay is open.
type state struct {
	Scroll    ui.ScrollState
	Indicator scrollbar.Indicator
}

// Host owns the overlay's scroll presentation and delegates domain actions
// through the small Ports contract.
type Host struct {
	ports    Ports
	controls workspaceui.Controls
	state    state
	closed   bool
}

// Content is kept as a feature-local name for callers that compose the host
// directly with the generic window manager.
type Content = Host

func NewHost(ports Ports) *Host { return &Host{ports: ports} }

func NewContent(ports Ports) *Host { return NewHost(ports) }

func (s *Host) Bind(controls workspaceui.Controls) { s.controls = controls }

func (s *Host) Render(c *ui.Context) {
	if s == nil || s.closed {
		return
	}
	model := Model{}
	if s.ports.Data != nil {
		model = s.ports.Data()
	}
	Render(c, model, &s.state.Scroll, &s.state.Indicator, func(action Action) {
		switch action.Kind {
		case Dismiss:
			if s.controls != nil {
				s.controls.CloseSelf()
			}
		case OpenSource:
			if s.controls != nil {
				s.controls.CloseSelf()
			}
			if s.ports.OpenSource != nil {
				s.ports.OpenSource(action.Path, action.Offset)
			}
		case OpenTarget:
			if s.controls != nil {
				s.controls.CloseSelf()
			}
			if s.ports.OpenTarget != nil {
				s.ports.OpenTarget(action.Target)
			}
		}
	})
}

func (s *Host) Close() {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	s.state = state{}
	if s.ports.OnClose != nil {
		s.ports.OnClose()
	}
}

func (s *Host) Focused() {}
func (s *Host) Blurred() {}

var _ workspaceui.Content = (*Host)(nil)
var _ workspaceui.Bindable = (*Host)(nil)
var _ workspaceui.FocusLifecycle = (*Host)(nil)
