package imagepreview

import (
	"github.com/egoist/mygo/ui"
	"muse/markdown/images"
	hosting "muse/nativeui/workbench/hosting"
)

// Content owns the full-screen image preview's private view state and joins it
// to the generic logical window host.
type Content struct {
	state      state
	cache      images.CacheStore
	localFiles images.LocalFiles
	source     string
	label      string
	onMain     func(func())
	invalidate func()
	onCopy     func(string)
	onClose    func()
	controls   hosting.Controls
	closed     bool
}

func NewContent(source, label string, cache images.CacheStore, onMain func(func()), invalidate func(), onCopy func(string), onClose func()) *Content {
	return &Content{source: source, label: label, cache: cache, onMain: onMain, invalidate: invalidate, onCopy: onCopy, onClose: onClose}
}

func (s *Content) Bind(controls hosting.Controls) { s.controls = controls }

// SetLocalFiles injects the platform file adapter before the first render.
func (s *Content) SetLocalFiles(files images.LocalFiles) {
	if s != nil && !s.state.open {
		s.localFiles = files
	}
}

func (s *Content) Render(c *ui.Context) {
	if s == nil || s.closed {
		return
	}
	if s.state.asset == nil && !s.state.open {
		s.state.Open(s.source, s.label, s.cache, s.localFiles, s.onMain, s.invalidate)
	}
	if next := s.state.Advance(c.Now()); next > 0 {
		c.After(next)
	}
	Render(c, s.state.ViewModel(), func(action Action) {
		switch action.Kind {
		case Close:
			if s.controls != nil {
				s.controls.CloseSelf()
			}
		case Copy:
			if s.onCopy != nil {
				s.onCopy(action.Source)
			}
		default:
			s.state.Apply(action)
			if s.invalidate != nil {
				s.invalidate()
			}
		}
	})
}

func (s *Content) Close() {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	s.state.Close()
	if s.onClose != nil {
		s.onClose()
	}
}

func (s *Content) Focused()       {}
func (s *Content) Blurred()       {}
func (s *Content) IsClosed() bool { return s == nil || s.closed }

var _ hosting.Content = (*Content)(nil)
var _ hosting.Bindable = (*Content)(nil)
var _ hosting.FocusLifecycle = (*Content)(nil)
