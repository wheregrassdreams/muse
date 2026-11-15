package markdown

import "muse/markdown/images"

// imageDecodeGate keeps the existing package-local test seam while the worker
// queue itself is owned by the Markdown feature package.
type imageDecodeGate struct{ queue *images.DecodeQueue }

func newImageDecodeGate(limit int) *imageDecodeGate {
	return &imageDecodeGate{queue: images.NewDecodeQueue(limit)}
}
func (g *imageDecodeGate) submit(job func()) {
	if g.queue == nil {
		images.SubmitDecode(job)
		return
	}
	g.queue.Submit(job)
}
func (g *imageDecodeGate) pendingCount() int {
	if g.queue == nil {
		pending, _ := images.QueueStats()
		return pending
	}
	pending, _ := g.queue.Stats()
	return pending
}
func (g *imageDecodeGate) activeCount() int {
	if g.queue == nil {
		_, active := images.QueueStats()
		return active
	}
	_, active := g.queue.Stats()
	return active
}

var imageDecodes = &imageDecodeGate{}

// ImageDecodeQueueStats reports feature-owned decode and frame queue status.
func ImageDecodeQueueStats() (pending, active int) { return images.QueueStats() }

// Remote fetching has its own bounded concurrency pool and runs outside decode
// workers so slow connections do not block local images.
func submitRemoteImageLoad(job func()) { images.SubmitRemote(job) }
