package searchui

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"muse/ui/scrollbar"
)

// state owns only transient presentation state for the workspace-search
// surface. The application supplies recent notes and matched results in View.
type state struct {
	open            bool
	query           string
	mode            Mode
	selected        int
	revealSelection bool
	scroll          ui.ScrollState
	results         ui.ListState
	indicator       scrollbar.Indicator
	inputFocus      ui.Handle
}

func newState() state { return state{mode: FullText} }

func (s *state) show() {
	if s == nil {
		return
	}
	s.open = true
	s.query = ""
	s.selected = 0
	s.revealSelection = false
	s.scroll = ui.ScrollState{}
	s.results = ui.ListState{}
	s.indicator = scrollbar.Indicator{}
}

func (s *state) close() {
	if s != nil {
		s.open = false
	}
}
func (s state) isOpen() bool       { return s.open }
func (s state) queryValue() string { return s.query }
func (s state) modeValue() Mode    { return s.mode }
func (s state) selection() int     { return s.selected }
func (s *state) setQuery(query string) {
	if s == nil || s.query == query {
		return
	}
	s.query = query
	s.selected = 0
	s.revealSelection = false
	s.results = ui.ListState{}
	s.scroll = ui.ScrollState{}
	s.indicator = scrollbar.Indicator{}
}

func (s *state) focusInput(c *ui.Context) {
	if s != nil {
		s.inputFocus.Focus(c)
	}
}

func (s state) visibleResults() (first, last int) { return s.results.Visible() }

func (s *state) setMode(mode Mode) {
	if s == nil || (mode != FullText && mode != Literal) {
		return
	}
	s.mode = mode
	s.selected = 0
	s.revealSelection = false
	s.results = ui.ListState{}
	s.scroll = ui.ScrollState{}
	s.indicator = scrollbar.Indicator{}
}
func (s *state) selectResult(index int, reveal bool) {
	if s == nil {
		return
	}
	s.selected = max(index, 0)
	s.revealSelection = reveal
}

// Apply commits view-state transitions from typed actions. The application
// still owns query execution and performs navigation for OpenResult/OpenRecent.
func (s *state) apply(action Action) {
	if s == nil {
		return
	}
	switch action.Kind {
	case QueryChanged:
		s.setQuery(action.Query)
	case ModeChanged:
		s.setMode(action.Mode)
	case MoveSelection:
		s.selectResult(action.Index, true)
	case Dismiss:
		s.close()
	}
}

func (s state) viewCount(view View) int {
	if strings.TrimSpace(s.query) == "" {
		return len(view.Recent)
	}
	if view.Availability != Ready {
		return 0
	}
	return len(view.Results)
}

// Handle owns one workspace-search panel state instance. The application may
// query the current intent and submit typed transitions, while the state and
// toolkit list/scroll internals remain private to this package.
type Handle struct{ state state }

func NewHandle() *Handle { return &Handle{state: newState()} }

func (h *Handle) Open() {
	if h != nil {
		h.state.show()
	}
}

func (h *Handle) Close() {
	if h != nil {
		h.state.close()
	}
}

func (h *Handle) IsOpen() bool { return h != nil && h.state.isOpen() }
func (h *Handle) Query() string {
	if h == nil {
		return ""
	}
	return h.state.queryValue()
}
func (h *Handle) Mode() Mode {
	if h == nil {
		return FullText
	}
	return h.state.modeValue()
}
func (h *Handle) Selection() int {
	if h == nil {
		return 0
	}
	return h.state.selection()
}

// Apply lets the composition root request panel state transitions through the
// same typed action contract used by rendered controls.
func (h *Handle) Apply(action Action) {
	if h != nil {
		h.state.apply(action)
	}
}

func (h *Handle) FocusInput(c *ui.Context) {
	if h != nil {
		h.state.focusInput(c)
	}
}

// VisibleResults returns the row range currently visible in the list. The
// toolkit's scroll state and list implementation remain private to this panel.
func (h *Handle) VisibleResults() (first, last int) {
	if h == nil {
		return 0, -1
	}
	return h.state.visibleResults()
}

func (h *Handle) Render(c *ui.Context, view View, dispatch func(Action)) {
	if h != nil {
		render(c, &h.state, view, dispatch)
	}
}
