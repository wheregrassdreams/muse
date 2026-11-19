// Package feedback contains backend-neutral animation policies. It consumes
// value snapshots and never retains editor, renderer, or widget state.
package feedback

import (
	"math"
	"time"
)

const (
	CaretBlinkPeriod         = 530 * time.Millisecond
	CaretFadeDuration        = 120 * time.Millisecond
	SmoothCursorDuration     = 110 * time.Millisecond
	SmoothCursorFrameRate    = float32(60)
	SearchMatchPulseDuration = 220 * time.Millisecond
	VimYankFlashDuration     = 320 * time.Millisecond
)

// CaretFadeOpacity returns the caret opacity during the short fade windows at
// the beginning and middle of a blink cycle. Away from those boundaries the
// caret is fully visible or hidden.
func CaretFadeOpacity(phase time.Duration) (float32, bool) {
	if phase < CaretFadeDuration {
		return 1, false
	}
	cycle := phase % (2 * CaretBlinkPeriod)
	if cycle < CaretFadeDuration {
		t := float32(cycle) / float32(CaretFadeDuration)
		return t * t * (3 - 2*t), true
	}
	if cycle >= CaretBlinkPeriod && cycle < CaretBlinkPeriod+CaretFadeDuration {
		t := float32(cycle-CaretBlinkPeriod) / float32(CaretFadeDuration)
		t = 1 - t*t*(3-2*t)
		return t, true
	}
	if cycle >= CaretBlinkPeriod+CaretFadeDuration {
		return 0, false
	}
	return 1, false
}

// Rect is a value-only rectangle used by cursor interpolation.
type Rect struct{ X, Y, W, H float32 }

func SmoothCursorNeedsFrames(enabled, active bool) bool { return enabled && active }

func InterpolateSmoothCursor(start, now time.Time, from, to Rect) (Rect, bool) {
	progress := float64(now.Sub(start)) / float64(SmoothCursorDuration)
	if progress >= 1 {
		return to, false
	}
	if progress < 0 {
		progress = 0
	}
	t := float32(progress * progress * (3 - 2*progress))
	return Rect{X: lerp(from.X, to.X, t), Y: lerp(from.Y, to.Y, t), W: lerp(from.W, to.W, t), H: lerp(from.H, to.H, t)}, true
}

func SmoothCursorProgress(elapsed time.Duration) float32 {
	p := float64(elapsed) / float64(SmoothCursorDuration)
	p = math.Max(0, math.Min(1, p))
	return float32(p * p * (3 - 2*p))
}

func SearchMatchPulse(elapsed time.Duration) (float32, bool) {
	progress := float32(elapsed) / float32(SearchMatchPulseDuration)
	if progress >= 1 {
		return 0, false
	}
	progress = min(max(progress, 0), 1)
	return 4 * progress * (1 - progress), true
}

func VimYankFlashOpacity(elapsed time.Duration) (float32, bool) {
	progress := float32(elapsed) / float32(VimYankFlashDuration)
	if progress >= 1 {
		return 0, false
	}
	progress = min(max(progress, 0), 1)
	return 1 - progress*progress, true
}

func lerp(a, b, t float32) float32 { return a + (b-a)*t }
