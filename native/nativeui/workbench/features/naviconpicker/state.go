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
		Path: s.path, Query: s.query, Scroll: &s.scroll,
		Results: append([]Option(nil), s.results...), Loading: s.loading,
	}
}
