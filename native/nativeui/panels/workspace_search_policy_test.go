package nativeui

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestWorkspaceSearchCoalesceSelectsMeasuredCadence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cost     time.Duration
		busy     bool
		wantWait time.Duration
	}{
		{name: "fast immediate", cost: 5 * time.Millisecond, wantWait: 0},
		{name: "one frame balanced", cost: 10 * time.Millisecond, wantWait: workspaceSearchBalancedWait},
		{name: "slow device low load", cost: 28 * time.Millisecond, wantWait: workspaceSearchLowLoadWait},
		{name: "coalesce while prior query runs", cost: 5 * time.Millisecond, busy: true, wantWait: workspaceSearchBalancedWait},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := workspaceSearchCoalesce(tc.cost, tc.busy); got != tc.wantWait {
				t.Fatalf("workspaceSearchCoalesce(%s, %v) = %s, want %s", tc.cost, tc.busy, got, tc.wantWait)
			}
		})
	}
}

func TestSmoothWorkspaceSearchCostUsesRecentMeasurements(t *testing.T) {
	if got := smoothWorkspaceSearchCost(0, 40*time.Millisecond); got != 40*time.Millisecond {
		t.Fatalf("first sample = %s", got)
	}
	if got := smoothWorkspaceSearchCost(40*time.Millisecond, 8*time.Millisecond); got != 32*time.Millisecond {
		t.Fatalf("smoothed cost = %s, want 32ms", got)
	}
	if got := smoothWorkspaceSearchCost(40*time.Millisecond, 0); got != 40*time.Millisecond {
		t.Fatalf("zero sample erased cost: %s", got)
	}
}

func TestLockWorkspaceSearchCanStopWaitingWhenCanceled(t *testing.T) {
	var mu sync.Mutex
	mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() { done <- lockWorkspaceSearch(ctx, &mu) }()
	time.Sleep(5 * time.Millisecond)
	cancel()
	select {
	case acquired := <-done:
		if acquired {
			t.Fatal("canceled waiter acquired the search lock")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("canceled search remained queued behind the lock")
	}
	mu.Unlock()
}
