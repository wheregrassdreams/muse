package markdown

import "sync"

// Image decoding creates both compressed input buffers and full-size pixel
// planes. Bound the number of those jobs that can overlap during fast scrolls.
// A fixed worker pool also avoids one blocked goroutine per queued image.
const maxConcurrentImageDecodes = 2

type imageDecodeGate struct {
	mu      sync.Mutex
	pending []func()
	head    int
	wake    chan struct{}
	active  int
}

func newImageDecodeGate(limit int) *imageDecodeGate {
	if limit < 1 {
		limit = 1
	}
	g := &imageDecodeGate{wake: make(chan struct{}, 1)}
	for range limit {
		go g.worker()
	}
	return g
}

var imageDecodes = newImageDecodeGate(maxConcurrentImageDecodes)

// Remote fetches wait outside the decode queue so a few slow servers cannot
// prevent local files and data URLs from being decoded.
const maxConcurrentRemoteImageLoads = 4

var remoteImageLoadSlots = make(chan struct{}, maxConcurrentRemoteImageLoads)

func (g *imageDecodeGate) submit(job func()) {
	if job == nil {
		return
	}
	g.mu.Lock()
	g.pending = append(g.pending, job)
	g.mu.Unlock()
	select {
	case g.wake <- struct{}{}:
	default:
	}
}

func (g *imageDecodeGate) pendingCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.pending) - g.head
}

func (g *imageDecodeGate) activeCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.active
}

// ImageDecodeQueueStats reports queued and active image decode or frame jobs.
func ImageDecodeQueueStats() (pending, active int) {
	imageDecodes.mu.Lock()
	defer imageDecodes.mu.Unlock()
	return len(imageDecodes.pending) - imageDecodes.head, imageDecodes.active
}

func (g *imageDecodeGate) worker() {
	for {
		g.mu.Lock()
		if g.head < len(g.pending) {
			job := g.pending[g.head]
			g.pending[g.head] = nil
			g.head++
			g.active++
			if g.head == len(g.pending) {
				g.pending = nil
				g.head = 0
			} else if g.head >= 64 && g.head*2 >= len(g.pending) {
				g.pending = append(g.pending[:0], g.pending[g.head:]...)
				g.head = 0
			}
			g.mu.Unlock()
			job()
			g.mu.Lock()
			g.active--
			g.mu.Unlock()
			continue
		}
		g.mu.Unlock()
		<-g.wake
	}
}
