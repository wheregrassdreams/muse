package feedback

import (
	"time"
)

// YankFlashInput contains only host snapshots needed to start a Vim yank
// flash; the source remains owned by the editor session.
type YankFlashInput struct {
	Ranges       []TextRange
	SourceLength int
	Enabled      bool
	VimKeymap    bool
	Now          time.Time
}

// StartVimYankFlash validates and clamps source ranges before recording the
// transient visual feedback state.
func (h *Handle) StartVimYankFlash(input YankFlashInput) bool {
	if h == nil || !input.Enabled || !input.VimKeymap || input.SourceLength < 0 || len(input.Ranges) == 0 {
		return false
	}
	valid := make([]TextRange, 0, len(input.Ranges))
	for _, r := range input.Ranges {
		r.Start = max(0, min(r.Start, input.SourceLength))
		r.End = max(r.Start, min(r.End, input.SourceLength))
		if r.End > r.Start {
			valid = append(valid, r)
		}
	}
	if len(valid) == 0 {
		return false
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	h.StartYankFlash(valid, now)
	return true
}

// VimYankFlashOpacity gates animation on the current host settings and
// keymap while delegating lifetime to the feedback state.
func (h *Handle) VimYankFlashOpacity(enabled, vimKeymap bool, now time.Time) (float32, bool) {
	if !enabled || !vimKeymap {
		return 0, false
	}
	opacity, active := h.YankFlashOpacity(now)
	return opacity, active
}
