package quickopen

import (
	"github.com/egoist/mygo/ui"
	"muse/nativeui/window"
)

type SurfaceOptions struct {
	Rows func(query string) Rows
	// RowsRevision identifies changes in the candidate source independent of
	// the query. The content refreshes its row snapshot when either this value
	// or the query changes.
	RowsRevision func() [2]uint64
	FileIcon     *ui.SVG
	IconTint     func(*ui.Theme) ui.Color
	OnOpen       func(path string)
	OnClose      func()
}

// Content keeps query and selection private to the transient Quick Open
// surface. Candidate construction and ranking remain with the caller.
type Content struct {
	state    state
	options  SurfaceOptions
	controls window.Controls
	closed   bool
	rows     Rows
	rowsKey  [2]uint64
	rowsFor  string
	hasRows  bool
}

func NewContent(options SurfaceOptions) *Content {
	return &Content{state: state{Open: true}, options: options}
}

func (s *Content) Bind(controls window.Controls) { s.controls = controls }

func (s *Content) Render(c *ui.Context) {
	if s == nil || s.closed {
		return
	}
	rowsKey := [2]uint64{}
	if s.options.RowsRevision != nil {
		rowsKey = s.options.RowsRevision()
	}
	if !s.hasRows || s.rowsFor != s.state.Query || s.rowsKey != rowsKey {
		s.rowsFor, s.rowsKey, s.hasRows = s.state.Query, rowsKey, true
		s.rows = nil
		if s.options.Rows != nil {
			s.rows = s.options.Rows(s.state.Query)
		}
	}
	rows := s.rows
	tint := ui.Color{}
	if s.options.IconTint != nil {
		tint = s.options.IconTint(c.Theme())
	}
	Render(c, ViewModel{State: s.state, Rows: rows, FileIcon: s.options.FileIcon, IconTint: tint}, func(action Action) {
		switch action.Kind {
		case QueryChanged:
			s.state.Query = action.Query
			s.state.Selected = 0
		case MoveSelection:
			s.state.Selected = action.Index
		case OpenSelection:
			if rows != nil && action.Index >= 0 && action.Index < rows.Len() && s.options.OnOpen != nil {
				s.options.OnOpen(rows.At(action.Index).Path)
				if s.controls != nil {
					s.controls.CloseSelf()
				}
			}
		case Dismiss:
			if s.controls != nil {
				s.controls.CloseSelf()
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
	s.rows = nil
	s.hasRows = false
	if s.options.OnClose != nil {
		s.options.OnClose()
	}
}

func (s *Content) Focused() {}
func (s *Content) Blurred() {}

var _ window.Content = (*Content)(nil)
var _ window.Bindable = (*Content)(nil)
var _ window.FocusLifecycle = (*Content)(nil)
