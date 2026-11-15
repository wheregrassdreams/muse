package markdown

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestImageDecodeGateBoundsConcurrentWork(t *testing.T) {
	const limit, workers = 3, 12
	gate := newImageDecodeGate(limit)
	started := make(chan struct{}, workers)
	release := make(chan struct{})
	var active, peak atomic.Int32
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gate.submit(func() {
				n := active.Add(1)
				for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
				}
				started <- struct{}{}
				<-release
				active.Add(-1)
			})
		}()
	}
	wg.Wait()
	for range limit {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("decode gate did not admit its configured concurrency")
		}
	}
	select {
	case <-started:
		close(release)
		t.Fatal("decode gate admitted work beyond its limit")
	case <-time.After(10 * time.Millisecond):
	}
	if got := gate.pendingCount(); got != workers-limit {
		close(release)
		t.Fatalf("pending decode jobs = %d, want %d", got, workers-limit)
	}
	close(release)
	deadline := time.After(time.Second)
	for gate.pendingCount() > 0 || gate.activeCount() > 0 {
		select {
		case <-deadline:
			t.Fatal("decode gate did not drain queued jobs")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if got := peak.Load(); got != limit {
		t.Fatalf("peak concurrent decode work = %d, want %d", got, limit)
	}
}
