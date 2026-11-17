package imagedocument

import (
	"github.com/egoist/mygo/ui"
	"muse/markdown/images"
	"muse/nativeui/workbench/hosting"
)

type SurfaceOptions struct {
	LocalFiles    images.LocalFiles
	Header        func(*ui.Context)
	HeaderHeight  func(*ui.Context) float32
	BacklinkCount func() int
	OnBacklinks   func()
	OnFullscreen  func(string)
	OnCopy        func(string)
	OnClose       func()
}

// Content owns the active image document's zoom/pan state and rendering.
type Content struct {
	state    state
	options  SurfaceOptions
	controls window.Controls
	closed   bool
}

func NewContent(path string, cache images.CacheStore, options SurfaceOptions) (*Content, bool) {
	content := &Content{options: options}
	content.state.Open(path, cache, options.LocalFiles)
	return content, content.state.HasImage()
}

func (s *Content) Bind(controls window.Controls) { s.controls = controls }

func (s *Content) Render(c *ui.Context) {
	if s == nil || s.closed {
		return
	}
	count := 0
	if s.options.BacklinkCount != nil {
		count = s.options.BacklinkCount()
	}
	headerHeight := float32(0)
	if s.options.HeaderHeight != nil {
		headerHeight = s.options.HeaderHeight(c)
	}
	Render(c, s.state.ViewModel(count), Options{
		Header: func() {
			if s.options.Header != nil {
				s.options.Header(c)
			}
		},
		HeaderHeight: headerHeight,
		OnBacklinks:  s.options.OnBacklinks,
		OnFullscreen: func() {
			if s.options.OnFullscreen != nil {
				s.options.OnFullscreen(s.state.Path())
			}
		},
		OnCopy: func() {
			if s.options.OnCopy != nil {
				s.options.OnCopy(s.state.Path())
			}
		},
	}, func(action Action) {
		s.state.Apply(action)
	})
}

func (s *Content) Close() {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	s.state.Close()
	if s.options.OnClose != nil {
		s.options.OnClose()
	}
}

func (s *Content) Focused() {}
func (s *Content) Blurred() {}

func (s *Content) IsClosed() bool { return s == nil || s.closed }

var _ window.Content = (*Content)(nil)
var _ window.Bindable = (*Content)(nil)
var _ window.FocusLifecycle = (*Content)(nil)
