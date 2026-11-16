package editor

import (
	"time"
)

func (e *Editor) searchMatchPulse(now time.Time) (float32, bool) {
	if !e.opts.SearchMatchPulse {
		return 0, false
	}
	return e.feedbackState.SearchPulseOpacity(now)
}
