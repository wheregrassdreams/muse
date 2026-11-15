package nativeui

import (
	"context"
	"sync"
	"time"
)

// Workspace search adapts only after observing real work on this vault. A
// quick index query stays immediate; slower or overlapping queries briefly
// coalesce keystrokes so weak CPUs and cold disks do not run every prefix.
const (
	workspaceSearchBalancedAfter = 8 * time.Millisecond
	workspaceSearchSlowAfter     = 24 * time.Millisecond
	workspaceSearchBalancedWait  = 16 * time.Millisecond
	workspaceSearchLowLoadWait   = 40 * time.Millisecond
)

func workspaceSearchCoalesce(estimatedCost time.Duration, queryInFlight bool) time.Duration {
	if estimatedCost >= workspaceSearchSlowAfter {
		return workspaceSearchLowLoadWait
	}
	if estimatedCost >= workspaceSearchBalancedAfter || queryInFlight {
		return workspaceSearchBalancedWait
	}
	return 0
}

func smoothWorkspaceSearchCost(previous, sample time.Duration) time.Duration {
	if sample <= 0 {
		return previous
	}
	if previous <= 0 {
		return sample
	}
	return (previous*3 + sample) / 4
}

// lockWorkspaceSearch waits for watcher/index work without leaving canceled
// per-keystroke searches queued behind it. The uncontended path is one TryLock.
func lockWorkspaceSearch(ctx context.Context, mu *sync.Mutex) bool {
	if ctx.Err() != nil {
		return false
	}
	if mu.TryLock() {
		return true
	}
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return false
		}
		if mu.TryLock() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}
