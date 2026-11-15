// Package outline renders caller-provided heading rows and translates a
// selection into a typed action. Heading extraction, active-heading policy,
// and navigation remain with the caller.
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

// SelectedRange is an inclusive range of row indices selected by the caller.
// A single active heading is represented by Start == End.
type SelectedRange struct {
	Start, End int
}

func (r SelectedRange) Contains(row, count int) bool {
	return r.Start >= 0 && r.End >= r.Start && r.End < count && row >= r.Start && row <= r.End
}

// ScrollPort gives the owner control over scroll position and indicator
// lifetime while allowing this view to bind the toolkit list to that state.
type ScrollPort struct {
	State     *ui.ScrollState
	Indicator *scrollbar.Indicator
}

// ViewModel contains only values/capabilities required to render the panel.
// The caller owns heading extraction, active-row selection, and scroll state.
type ViewModel struct {
	Headings      []Heading
	SelectedRange SelectedRange
	Scroll        ScrollPort
}

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

func Render(c *ui.Context, model ViewModel, dispatch func(Action)) {
	t := c.Theme()
	if len(model.Headings) == 0 {
		ui.Column(c).Fill().Center().Padding(20).Children(func() {
			ui.Text(c, locales.DocumentNoHeading()).FontSize(12).TextColor(t.TextMuted)
		})
		return
	}

	showScrollbar := false
	if model.Scroll.State != nil && model.Scroll.Indicator != nil {
		showScrollbar = model.Scroll.Indicator.Visible(c.Now(), model.Scroll.State.X, model.Scroll.State.Y)
		if showScrollbar {
			c.After(model.Scroll.Indicator.Remaining(c.Now()))
		}
	}

	var fallbackScroll ui.ScrollState
	scroll := model.Scroll.State
	if scroll == nil {
		scroll = &fallbackScroll
	}
	ui.List(c, nil, len(model.Headings), func(i int) {
		heading := model.Headings[i]
		selected := model.SelectedRange.Contains(i, len(model.Headings))
		row := ui.Row(c).Height(26).Radius(6).Padding(0, 8, 0, 6+float32(max(heading.Level-1, 0))*14).
			Gap(7).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		if selected || row.Hovered() {
			row.Background(t.SurfaceHover)
		}
		row.Children(func() {
			if selected {
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
			if dispatch != nil {
				dispatch(Action{Kind: SelectHeading, Offset: heading.Offset})
			}
		})
	}).Grow(1).Padding(0, 8).Gap(0).TrackScroll(scroll).Scrollbars(showScrollbar)
}
