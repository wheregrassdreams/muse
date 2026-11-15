package mdedit

import "time"

const searchMatchPulseDuration = 220 * time.Millisecond

// searchMatchPulse returns a brief eased expansion for the newly selected
// search result. It starts and ends at rest so frames do not jump.
func (e *Editor) searchMatchPulse(now time.Time) (float32, bool) {
	if !e.opts.SearchMatchPulse || !e.searchPulseActive {
		return 0, false
	}
	progress := float32(now.Sub(e.searchPulseStart)) / float32(searchMatchPulseDuration)
	if progress >= 1 {
		e.searchPulseActive = false
		return 0, false
	}
	progress = min(max(progress, 0), 1)
	return 4 * progress * (1 - progress), true
}
