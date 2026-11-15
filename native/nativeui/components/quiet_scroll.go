package components

import (
	"github.com/egoist/mygo/ui"
	scrollbar "muse/ui/scrollbar"
)

func QuietScroll(c *ui.Context, state *ui.ScrollState, indicator *scrollbar.Indicator) ui.Element {
	show := indicator.Visible(c.Now(), state.X, state.Y)
	if show {
		c.After(indicator.Remaining(c.Now()))
	}
	return ui.Scroll(c).TrackScroll(state).Scrollbars(show)
}
