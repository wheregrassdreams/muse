// Package findreplace owns the current-document find/replace panel state and
// presentation. Matching and source range planning live in editor/findreplace.
package findreplace

import (
	"github.com/egoist/mygo/ui"
	editfind "muse/editor/findreplace"
	"muse/ui/scrollbar"
)

type ActionKind uint8

const (
	QueryChanged ActionKind = iota + 1
	ReplacementChanged
	OpenMatch
	ReplaceOne
	ReplaceAll
)

type Action struct {
	Kind  ActionKind
	Query string
	Text  string
	Index int
}

// state belongs to the panel and is scoped to the active document view.
type state struct {
	query       string
	replacement string
	matches     []editfind.Match
	matchSource string
	matchQuery  string
	valid       bool
	selected    int
	list        ui.ListState
	scroll      ui.ScrollState
	indicator   scrollbar.Indicator
}

// Snapshot is a detached read-only view of the current match projection.
// Matches is copied so callers cannot mutate panel-owned cached ranges.
type Snapshot struct {
	Query       string
	Replacement string
	Matches     []editfind.Match
	Selected    int
}

// Handle is the only stateful capability exposed to the composition root.
// The panel's concrete state and toolkit list/scroll state stay private.
type Handle struct{ state *state }

func NewHandle() *Handle { return &Handle{state: &state{selected: -1}} }

func (h *Handle) ensureState() *state {
	if h == nil {
		return nil
	}
	if h.state == nil {
		h.state = &state{selected: -1}
	}
	return h.state
}

func (h *Handle) Snapshot() Snapshot {
	s := h.ensureState()
	if s == nil {
		return Snapshot{Selected: -1}
	}
	return Snapshot{
		Query: s.query, Replacement: s.replacement,
		Matches: append([]editfind.Match(nil), s.matches...), Selected: s.selected,
	}
}

func (h *Handle) Sync(source string) bool {
	s := h.ensureState()
	return s != nil && s.sync(source)
}

func (h *Handle) Apply(action Action) {
	if s := h.ensureState(); s != nil {
		s.apply(action)
	}
}

func (h *Handle) Invalidate() {
	if s := h.ensureState(); s != nil {
		s.invalidate()
	}
}

// Sync refreshes the cached match projection when the query or document
// source changes. The caller should synchronize editor highlights when true.
func (s *state) sync(source string) bool {
	if s == nil || (s.valid && s.matchQuery == s.query && s.matchSource == source) {
		return false
	}
	s.matchQuery, s.matchSource = s.query, source
	s.matches = editfind.Find(source, s.query)
	s.valid = true
	if s.selected >= len(s.matches) {
		s.selected = -1
	}
	s.list = ui.ListState{}
	s.scroll = ui.ScrollState{}
	s.indicator = scrollbar.Indicator{}
	return true
}

func (s *state) setQuery(query string) {
	if s == nil || s.query == query {
		return
	}
	s.query = query
	s.selected = -1
	s.valid = false
	s.list = ui.ListState{}
	s.scroll = ui.ScrollState{}
	s.indicator = scrollbar.Indicator{}
}

// Invalidate forces the next Sync to refresh match ranges and editor
// highlights, such as after switching to another document with identical text.
func (s *state) invalidate() {
	if s == nil {
		return
	}
	s.valid = false
	s.matchSource = ""
}

func (s *state) setReplacement(text string) {
	if s != nil {
		s.replacement = text
	}
}

func (s *state) selectMatch(index int) {
	if s == nil || index < 0 || index >= len(s.matches) {
		return
	}
	s.selected = index
}

func (s *state) apply(action Action) {
	if s == nil {
		return
	}
	switch action.Kind {
	case QueryChanged:
		s.setQuery(action.Query)
	case ReplacementChanged:
		s.setReplacement(action.Text)
	case OpenMatch:
		s.selectMatch(action.Index)
	}
}
