package nativeui

import (
	"github.com/egoist/mygo/ui"
	"muse/scrollui"
)

func quietScroll(c *ui.Context, state *ui.ScrollState, indicator *scrollui.Indicator) ui.Element {
	show := indicator.Visible(c.Now(), state.X, state.Y)
	if show {
		c.After(indicator.Remaining(c.Now()))
	}
	return ui.Scroll(c).TrackScroll(state).Scrollbars(show)
}
