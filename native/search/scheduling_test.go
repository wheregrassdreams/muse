package search

import (
	"testing"
	"time"
)

func TestCoalesceDelaySelectsMeasuredCadence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cost     time.Duration
		busy     bool
		wantWait time.Duration
	}{
		{name: "fast immediate", cost: 5 * time.Millisecond, wantWait: 0},
		{name: "one frame balanced", cost: 10 * time.Millisecond, wantWait: balancedWait},
		{name: "slow device low load", cost: 28 * time.Millisecond, wantWait: lowLoadWait},
		{name: "coalesce while prior query runs", cost: 5 * time.Millisecond, busy: true, wantWait: balancedWait},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CoalesceDelay(tc.cost, tc.busy); got != tc.wantWait {
				t.Fatalf("CoalesceDelay(%s, %v) = %s, want %s", tc.cost, tc.busy, got, tc.wantWait)
			}
		})
	}
}

func TestQuerySchedulerSubmitsImmediateRequestAndDeduplicates(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	var scheduler QueryScheduler
	request := QueryRequest{Text: "alpha"}

	if wait := scheduler.Schedule(request, 0, false, now); wait != 0 {
		t.Fatalf("immediate request wait = %s, want 0", wait)
	}
	if due, ok := scheduler.Due(); !ok || !due.Equal(now) {
		t.Fatalf("due = %v, %v; want %v, true", due, ok, now)
	}
	if got, ok := scheduler.Take(now); !ok || got != request {
		t.Fatalf("Take() = %+v, %v; want %+v, true", got, ok, request)
	}

	if wait := scheduler.Schedule(request, 0, false, now); wait != 0 {
		t.Fatalf("duplicate request wait = %s, want 0", wait)
	}
	if _, ok := scheduler.Due(); ok {
		t.Fatal("duplicate request scheduled unnecessary work")
	}
	if got, ok := scheduler.Take(now); ok {
		t.Fatalf("duplicate Take() = %+v, true; want no request", got)
	}
}

func TestQuerySchedulerTreatsTextAndLiteralModeAsRequestIdentity(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	var scheduler QueryScheduler
	first := QueryRequest{Text: "tag:work"}
	scheduler.Schedule(first, 0, false, now)
	if got, ok := scheduler.Take(now); !ok || got != first {
		t.Fatalf("first Take() = %+v, %v; want %+v, true", got, ok, first)
	}

	for _, next := range []QueryRequest{
		{Text: "tag:personal"},
		{Text: "tag:work", Literal: true},
	} {
		if wait := scheduler.Schedule(next, 0, false, now); wait != 0 {
			t.Fatalf("changed request %+v wait = %s, want 0", next, wait)
		}
		if got, ok := scheduler.Take(now); !ok || got != next {
			t.Fatalf("changed request Take() = %+v, %v; want %+v, true", got, ok, next)
		}
	}
}

func TestQuerySchedulerCoalescesToLatestRequestAtOriginalDeadline(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	var scheduler QueryScheduler
	first := QueryRequest{Text: "first"}
	latest := QueryRequest{Text: "latest", Literal: true}
	cost := 10 * time.Millisecond

	if wait := scheduler.Schedule(first, cost, false, now); wait != balancedWait {
		t.Fatalf("first wait = %s, want %s", wait, balancedWait)
	}
	firstDue, ok := scheduler.Due()
	if !ok || !firstDue.Equal(now.Add(balancedWait)) {
		t.Fatalf("first due = %v, %v", firstDue, ok)
	}
	if wait := scheduler.Schedule(latest, cost, false, now.Add(5*time.Millisecond)); wait != 11*time.Millisecond {
		t.Fatalf("coalesced wait = %s, want 11ms", wait)
	}
	if due, ok := scheduler.Due(); !ok || !due.Equal(firstDue) {
		t.Fatalf("coalescing moved deadline: due = %v, %v; want %v", due, ok, firstDue)
	}
	if _, ok := scheduler.Take(firstDue.Add(-time.Nanosecond)); ok {
		t.Fatal("request was submitted before its deadline")
	}
	if got, ok := scheduler.Take(firstDue); !ok || got != latest {
		t.Fatalf("Take() = %+v, %v; want latest request %+v", got, ok, latest)
	}
}

func TestQuerySchedulerForceRefreshAndReset(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	var scheduler QueryScheduler
	request := QueryRequest{Text: "same"}
	scheduler.Schedule(request, 0, false, now)
	if _, ok := scheduler.Take(now); !ok {
		t.Fatal("initial request was not submitted")
	}

	scheduler.Force(now, 120*time.Millisecond)
	if wait := scheduler.Schedule(request, 0, false, now); wait != 120*time.Millisecond {
		t.Fatalf("forced wait = %s, want 120ms", wait)
	}
	if _, ok := scheduler.Take(now.Add(119 * time.Millisecond)); ok {
		t.Fatal("forced request was submitted before its deadline")
	}
	if got, ok := scheduler.Take(now.Add(120 * time.Millisecond)); !ok || got != request {
		t.Fatalf("forced Take() = %+v, %v; want %+v, true", got, ok, request)
	}

	scheduler.Schedule(QueryRequest{Text: "pending"}, 10*time.Millisecond, false, now)
	scheduler.Reset()
	if _, ok := scheduler.Due(); ok {
		t.Fatal("Reset retained a pending deadline")
	}
	if _, ok := scheduler.Take(now.Add(time.Second)); ok {
		t.Fatal("Reset retained a pending request")
	}
	if wait := scheduler.Schedule(request, 0, false, now); wait != 0 {
		t.Fatalf("request after Reset wait = %s, want 0", wait)
	}
	if got, ok := scheduler.Take(now); !ok || got != request {
		t.Fatalf("request after Reset Take() = %+v, %v", got, ok)
	}
}
