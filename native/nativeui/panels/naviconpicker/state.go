package naviconpicker

import (
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

// state owns the picker lifecycle, query, scroll, and asynchronous result
// snapshot. The host starts icon-library searches and supplies their results.
type state struct {
	mu      sync.Mutex
	open    bool
	path    string
	query   string
	scroll  ui.ScrollState
	results []Option
	loading bool
	request uint64
}

func (s *state) Open(path string) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.open = true
	s.path = path
	s.query = ""
	s.scroll = ui.ScrollState{}
	return s.beginSearchLocked()
}

func (s *state) IsOpen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.open
}

func (s *state) BeginSearch() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.beginSearchLocked()
}

func (s *state) beginSearchLocked() uint64 {
	s.request++
	s.loading = true
	s.results = nil
	return s.request
}

func (s *state) CompleteSearch(request uint64, results []Option) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if request != s.request {
		return false
	}
	s.results = append(s.results[:0], results...)
	s.loading = false
	return true
}

func (s *state) CancelSearch() {
	s.mu.Lock()
	s.request++
	s.loading = false
	s.mu.Unlock()
}

func (s *state) Close() {
	s.mu.Lock()
	s.open = false
	s.request++
	s.loading = false
	s.mu.Unlock()
}

func (s *state) Apply(action Action) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch action.Kind {
	case QueryChanged:
		s.query = action.Query
		s.scroll.Y = 0
		return strings.TrimSpace(action.Query)
	case ScrollChanged:
		s.scroll = action.Scroll
	case Close:
		s.open = false
		s.request++
		s.loading = false
	}
	return ""
}

func (s *state) Snapshot() ViewModel {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ViewModel{
		Path: s.path, Query: s.query, Scroll: s.scroll,
		Results: append([]Option(nil), s.results...), Loading: s.loading,
	}
}

// Handle is the panel's opaque host capability. The concrete view state is
// private to this package so the composition root cannot couple to its fields.
type Handle struct{ state *state }

func NewHandle() *Handle { return &Handle{state: &state{}} }

func (h *Handle) ensureState() *state {
	if h == nil {
		return nil
	}
	if h.state == nil {
		h.state = &state{}
	}
	return h.state
}

func (h *Handle) Open(path string) uint64 {
	if s := h.ensureState(); s != nil {
		return s.Open(path)
	}
	return 0
}
func (h *Handle) IsOpen() bool {
	if s := h.ensureState(); s != nil {
		return s.IsOpen()
	}
	return false
}
func (h *Handle) BeginSearch() uint64 {
	if s := h.ensureState(); s != nil {
		return s.BeginSearch()
	}
	return 0
}
func (h *Handle) CompleteSearch(request uint64, results []Option) bool {
	if s := h.ensureState(); s != nil {
		return s.CompleteSearch(request, results)
	}
	return false
}
func (h *Handle) CancelSearch() {
	if s := h.ensureState(); s != nil {
		s.CancelSearch()
	}
}
func (h *Handle) Close() {
	if s := h.ensureState(); s != nil {
		s.Close()
	}
}
func (h *Handle) Apply(action Action) string {
	if s := h.ensureState(); s != nil {
		return s.Apply(action)
	}
	return ""
}
func (h *Handle) Snapshot() ViewModel {
	if s := h.ensureState(); s != nil {
		return s.Snapshot()
	}
	return ViewModel{}
}

// RenderPopover keeps the toolkit's open pointer private to the owning panel.
func RenderPopover(c *ui.Context, anchor ui.Element, handle *Handle, dispatch func(Action)) {
	state := handle.ensureState()
	if state == nil {
		return
	}
	state.mu.Lock()
	open := state.open
	model := ViewModel{
		Path: state.path, Query: state.query, Scroll: state.scroll,
		Results: append([]Option(nil), state.results...), Loading: state.loading,
	}
	state.mu.Unlock()
	ui.Popover(c, anchor, &open, func() {
		Render(c, model, func(action Action) {
			if action.Kind == Close || action.Kind == Choose || action.Kind == RestoreDefault {
				open = false
				state.Close()
			}
			if dispatch != nil {
				dispatch(action)
			}
		})
	})
	state.mu.Lock()
	if state.open && !open {
		state.request++
		state.loading = false
	}
	state.open = open
	state.mu.Unlock()
}
