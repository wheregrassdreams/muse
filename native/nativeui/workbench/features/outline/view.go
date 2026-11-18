// Package outline renders caller-provided heading rows, tracks its active row
// and scrolling, and translates a selection into a typed action. Heading
// extraction and editor navigation remain with the caller.
package outline

import (
	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/ui/scrollbar"
)

// Heading is a presentation row projected from the document's heading data.
// Offset is an opaque source position returned with SelectHeading.
type Heading struct {
	Level  int
	Text   string
	Offset int
}

// Snapshot contains the immutable heading projection and active source
// position supplied by the editor feature.
type Snapshot struct {
	Headings []Heading
	Caret    int
}

// ViewModel contains the immutable heading projection supplied by the caller.
type ViewModel struct{ Headings []Heading }

type ActionKind uint8

const (
	SelectHeading ActionKind = iota + 1
)

// Action requests selection/navigation to a heading. The application decides
// how to interpret Offset and how to move/focus the editor.
type Action struct {
	Kind   ActionKind
	Offset int
}

type state struct {
	activeRow int
	scroll    ui.ScrollState
	indicator scrollbar.Indicator
}

// Handle keeps the outline's active row and toolkit scrolling private to the
// panel. The application supplies a fresh heading projection for each view.
type Handle struct{ state state }

func NewHandle() *Handle { return &Handle{state: state{activeRow: -1}} }

// SyncActiveHeading updates the displayed selection from the active editor
// caret. Heading extraction remains with the editor session.
func (h *Handle) SyncActiveHeading(headings []Heading, caret int) {
	if h == nil {
		return
	}
	active := -1
	for i, heading := range headings {
		if heading.Offset <= caret {
			active = i
		}
	}
	h.state.activeRow = active
}

// Render draws the supplied immutable heading projection and owns the local
// selection and scrolling state. Selecting a heading returns its source offset
// to the application through the typed action callback.
func (h *Handle) Render(c *ui.Context, model ViewModel, dispatch func(Action)) {
	if h == nil {
		return
	}
	render(c, &h.state, model, dispatch)
}

// RenderSnapshot synchronizes the outline's active row with the editor caret,
// then renders the supplied immutable snapshot. The caller routes the typed
// selection action back to the editor navigation API.
func (h *Handle) RenderSnapshot(c *ui.Context, snapshot Snapshot, dispatch func(Action)) {
	if h == nil {
		return
	}
	h.SyncActiveHeading(snapshot.Headings, snapshot.Caret)
	h.Render(c, ViewModel{Headings: snapshot.Headings}, dispatch)
}

func render(c *ui.Context, s *state, model ViewModel, dispatch func(Action)) {
	if s == nil {
		fallback := state{activeRow: -1}
		s = &fallback
	}
	t := c.Theme()
	if len(model.Headings) == 0 {
		ui.Column(c).Fill().Center().Padding(20).Children(func() {
			ui.Text(c, locales.DocumentNoHeading()).FontSize(12).TextColor(t.TextMuted)
		})
		return
	}

	showScrollbar := s.indicator.Visible(c.Now(), s.scroll.X, s.scroll.Y)
	if showScrollbar {
		c.After(s.indicator.Remaining(c.Now()))
	}
	ui.List(c, nil, len(model.Headings), func(i int) {
		heading := model.Headings[i]
		isSelected := i == s.activeRow
		row := ui.Row(c).Height(26).Radius(6).Padding(0, 8, 0, 6+float32(max(heading.Level-1, 0))*14).
			Gap(7).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		if isSelected || row.Hovered() {
			row.Background(t.SurfaceHover)
		}
		row.Children(func() {
			if isSelected {
				ui.Box(c).Width(2.5).Height(14).Radius(1.5).Background(t.Accent)
			} else {
				ui.Box(c).Width(2.5).Height(14).Radius(1.5).Opacity(0)
			}
			label := ui.Text(c, heading.Text).SingleLine().Ellipsis("…")
			switch {
			case heading.Level == 1:
				label.FontSize(12.5).FontWeight(600).TextColor(t.Text)
			case heading.Level == 2:
				label.FontSize(12).FontWeight(500).TextColor(t.Text)
			default:
				label.FontSize(11.5).TextColor(t.TextMuted)
			}
		})
		row.OnClick(func() {
			s.activeRow = i
			if dispatch != nil {
				dispatch(Action{Kind: SelectHeading, Offset: heading.Offset})
			}
		})
	}).Grow(1).Padding(0, 8).Gap(0).TrackScroll(&s.scroll).Scrollbars(showScrollbar)
}
