//go:build darwin || linux

package mdedit

import (
	"syscall"
	"time"
)

// cpuTime returns how much CPU this process has used. A keystroke's budget
// is about the work it does, not about how long the scheduler kept it off a
// core: the whole test suite running its packages side by side would
// otherwise report a slow editor on a busy machine.
func cpuTime() (time.Duration, bool) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0, false
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano()), true
}
