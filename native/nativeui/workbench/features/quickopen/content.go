package quickopen

import (
	"github.com/egoist/mygo/ui"
	workspaceui "muse/nativeui/workspace"
)

type SurfaceOptions struct {
	Candidates CandidateSource
	FileIcon   *ui.SVG
	IconTint   func(*ui.Theme) ui.Color
	OnOpen     func(path string)
	OnClose    func()
}

// Content keeps query and selection private to the transient Quick Open
// surface. Candidate construction and ranking remain with the caller.
type Content struct {
	state    state
	options  SurfaceOptions
	controls workspaceui.Controls
	closed   bool
	rows     Rows
	rowsKey  Revision
	rowsFor  string
	hasRows  bool
}

func NewContent(options SurfaceOptions) *Content {
	return &Content{state: state{Open: true}, options: options}
}

func (s *Content) Bind(controls workspaceui.Controls) { s.controls = controls }

func (s *Content) Render(c *ui.Context) {
	if s == nil || s.closed {
		return
	}
	rowsKey := Revision{}
	if s.options.Candidates != nil {
		rowsKey = s.options.Candidates.Revision()
	}
	if !s.hasRows || s.rowsFor != s.state.Query || s.rowsKey != rowsKey {
		s.rowsFor, s.rowsKey, s.hasRows = s.state.Query, rowsKey, true
		s.rows = nil
		if s.options.Candidates != nil {
			s.rows = s.options.Candidates.Rows(s.state.Query)
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

var _ workspaceui.Content = (*Content)(nil)
var _ workspaceui.Bindable = (*Content)(nil)
var _ workspaceui.FocusLifecycle = (*Content)(nil)
