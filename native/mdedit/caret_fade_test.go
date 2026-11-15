package mdedit

import (
	"math"
	"testing"
	"time"
)

func TestCaretFadeOpacity(t *testing.T) {
	tests := []struct {
		name  string
		phase time.Duration
		alpha float32
		fades bool
	}{
		{name: "initial caret remains visible", phase: 0, alpha: 1},
		{name: "caret remains visible after navigation reset", phase: 60 * time.Millisecond, alpha: 1},
		{name: "initial phase ends without a fade restart", phase: 120 * time.Millisecond, alpha: 1},
		{name: "steady visible", phase: 200 * time.Millisecond, alpha: 1},
		{name: "fade out start", phase: blink, alpha: 1, fades: true},
		{name: "fade out midpoint", phase: blink + 60*time.Millisecond, alpha: 0.5, fades: true},
		{name: "steady hidden", phase: blink + 200*time.Millisecond, alpha: 0},
		{name: "next cycle fade in", phase: 2 * blink, alpha: 0, fades: true},
		{name: "subsequent fade in midpoint", phase: 2*blink + 60*time.Millisecond, alpha: 0.5, fades: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alpha, fading := caretFadeOpacity(tt.phase)
			if math.Abs(float64(alpha-tt.alpha)) > 0.001 || fading != tt.fades {
				t.Fatalf("caretFadeOpacity(%s) = (%v, %v), want (%v, %v)", tt.phase, alpha, fading, tt.alpha, tt.fades)
			}
		})
	}
}
