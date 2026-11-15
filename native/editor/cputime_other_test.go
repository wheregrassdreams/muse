//go:build !darwin && !linux

package mdedit

import "time"

// cpuTime is not read on this system: the budget is measured against the
// clock instead, and a busy machine may call it late.
func cpuTime() (time.Duration, bool) { return 0, false }
