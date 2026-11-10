// Package scrollui provides the application's scroll-only indicator policy.
package scrollui

import "time"

const VisibleFor = 700 * time.Millisecond

type Indicator struct {
	initialized bool
	x, y        float32
	until       time.Time
}

// Visible never reacts to hover, wheel intent at an edge, or an initial offset.
// It only extends the indicator lifetime when the actual offset changes.
func (i *Indicator) Visible(now time.Time, x, y float32) bool {
	if i.initialized && (x != i.x || y != i.y) {
		i.until = now.Add(VisibleFor)
	}
	i.initialized = true
	i.x = x
	i.y = y
	return now.Before(i.until)
}
func (i *Indicator) Remaining(now time.Time) time.Duration { return max(i.until.Sub(now), 0) }
