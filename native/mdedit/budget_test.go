package mdedit

import (
	"os"
	"sort"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestTypingFrameBudget keeps a keystroke inside a frame budget: a 1 MB
// document, the edit, the parse, the layout and a frame of the window — the
// slowest thing the editor does while the user types.
//
// 8.3 ms is 120 fps, the target on this machine. CI runs on shared, slower
// hardware, so there it takes the 60 fps budget instead; the gate is meant
// to catch a whole-document operation slipping into the keystroke path, not
// to measure the machine.
func TestTypingFrameBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("the frame budget is a timing test")
	}
	src := benchmarkSource(1 << 20)
	e := New(src, Options{FontSize: 16})
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			e.View(c).Grow(1)
		})
	}, 1000, 700)
	tt.Frame()
	tt.Frame()
	e.Focus()
	tt.Frame()
	tt.Frame()
	mid := benchmarkProse(t, src)
	e.anchor, e.caret = mid, mid
	e.wantReveal = true
	tt.Frame()

	// Warm up, so the first sample pays no cold caches.
	for i := 0; i < 5; i++ {
		e.begin(stepTyping)
		if i%2 == 0 {
			e.replace(mid, mid, "x")
		} else {
			e.replace(mid, mid+1, "")
		}
		tt.Frame()
	}

	const runs = 21
	samples := make([]time.Duration, 0, runs)
	for i := 0; i < runs; i++ {
		start := time.Now()
		e.begin(stepTyping)
		if i%2 == 0 {
			e.replace(mid, mid, "x")
		} else {
			e.replace(mid, mid+1, "")
		}
		tt.Frame()
		samples = append(samples, time.Since(start))
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	median := samples[len(samples)/2]

	budget := 8*time.Millisecond + 300*time.Microsecond // 120 fps
	if os.Getenv("CI") != "" {
		budget = 16 * time.Millisecond // 60 fps on shared machines
	}
	if median > budget {
		t.Fatalf("a keystroke in a 1 MB document takes %v (median of %d), budget %v", median, runs, budget)
	}
	t.Logf("keystroke %v, budget %v", median, budget)
}
