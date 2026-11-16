package editor

import "time"

const vimYankFlashDuration = 320 * time.Millisecond

func (e *Editor) startVimYankFlash(ranges []Range) {
	if e.Keymap() != "vim" || !e.opts.VimYankFlash || len(ranges) == 0 {
		return
	}
	valid := make([]Range, 0, len(ranges))
	for _, r := range ranges {
		r.Start = max(0, min(r.Start, len(e.src)))
		r.End = max(r.Start, min(r.End, len(e.src)))
		if r.End > r.Start {
			valid = append(valid, r)
		}
	}
	if len(valid) == 0 {
		return
	}
	e.vimYankRanges = valid
	e.vimYankStart = time.Now()
	e.vimYankActive = true
	e.invalidate()
}

// vimYankFlashOpacity fades the yanked range out quickly, like the brief
// highlight used by common Vim yank-flash configurations.
func (e *Editor) vimYankFlashOpacity(now time.Time) (float32, bool) {
	if e.Keymap() != "vim" || !e.opts.VimYankFlash || !e.vimYankActive {
		return 0, false
	}
	progress := float32(now.Sub(e.vimYankStart)) / float32(vimYankFlashDuration)
	if progress >= 1 {
		e.vimYankActive = false
		e.vimYankRanges = nil
		return 0, false
	}
	progress = min(max(progress, 0), 1)
	return 1 - progress*progress, true
}
