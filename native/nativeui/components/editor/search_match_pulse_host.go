package editor

import (
	"time"

	"muse/nativeui/components/editor/feedback"
)

func (e *Editor) searchMatchPulse(now time.Time) (float32, bool) {
	if !e.opts.SearchMatchPulse {
		return 0, false
	}
	return e.feedbackState.SearchPulseOpacity(now)
}
