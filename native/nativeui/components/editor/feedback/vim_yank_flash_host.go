package editor

import (
	"time"

	"muse/nativeui/components/editor/feedback"
)

func (e *Editor) startVimYankFlash(ranges []Range) {
	if e.Keymap() != "vim" || !e.opts.VimYankFlash || len(ranges) == 0 {
		return
	}
	valid := make([]feedback.TextRange, 0, len(ranges))
	for _, r := range ranges {
		r.Start = max(0, min(r.Start, len(e.Source())))
		r.End = max(r.Start, min(r.End, len(e.Source())))
		if r.End > r.Start {
			valid = append(valid, feedback.TextRange{Start: r.Start, End: r.End})
		}
	}
	if len(valid) == 0 {
		return
	}
	e.feedbackState.StartYankFlash(valid, time.Now())
	e.invalidate()
}

func (e *Editor) vimYankFlashOpacity(now time.Time) (float32, bool) {
	if e.Keymap() != "vim" || !e.opts.VimYankFlash {
		return 0, false
	}
	opacity, _, active := e.feedbackState.YankFlashOpacity(now)
	return opacity, active
}
