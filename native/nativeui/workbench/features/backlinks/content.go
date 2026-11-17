// Package backlinks owns the transient incoming and outgoing links surface.
package backlinks

import (
	"github.com/egoist/mygo/ui"
	"muse/nativeui/workbench/hosting"
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

// Content owns the overlay's scroll presentation and delegates domain actions
// through the small Ports contract.
type Content struct {
	ports    Ports
	controls window.Controls
	state    state
	closed   bool
}

func NewContent(ports Ports) *Content { return &Content{ports: ports} }

func (s *Content) Bind(controls window.Controls) { s.controls = controls }

func (s *Content) Render(c *ui.Context) {
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

func (s *Content) Close() {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	s.state = state{}
	if s.ports.OnClose != nil {
		s.ports.OnClose()
	}
}

func (s *Content) Focused() {}
func (s *Content) Blurred() {}

var _ window.Content = (*Content)(nil)
var _ window.Bindable = (*Content)(nil)
var _ window.FocusLifecycle = (*Content)(nil)
