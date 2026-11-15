package images

import "sync"

const (
	maxConcurrentDecodes = 2
	maxConcurrentRemote  = 4
)

// QueueStats reports queued and active feature-owned decode/frame jobs.
func QueueStats() (pending, active int) { return decodes.Stats() }

// SubmitDecode schedules bounded pixel decoding or GIF frame composition.
func SubmitDecode(job func()) { decodes.Submit(job) }

// SubmitRemote schedules bounded source fetching independently of decode work,
// so slow servers cannot starve local files and embedded data URLs.
func SubmitRemote(job func()) {
	if job == nil {
		return
	}
	go func() { remoteSlots <- struct{}{}; defer func() { <-remoteSlots }(); job() }()
}

var decodes = NewDecodeQueue(maxConcurrentDecodes)
var remoteSlots = make(chan struct{}, maxConcurrentRemote)

// DecodeQueue is a fixed worker pool with a compacting FIFO for image decode
// and animation preparation jobs.
type DecodeQueue struct {
	mu      sync.Mutex
	pending []func()
	head    int
	wake    chan struct{}
	active  int
}

func NewDecodeQueue(limit int) *DecodeQueue {
	if limit < 1 {
		limit = 1
	}
	q := &DecodeQueue{wake: make(chan struct{}, 1)}
	for range limit {
		go q.worker()
	}
	return q
}

func (q *DecodeQueue) Submit(job func()) {
	if job == nil {
		return
	}
	q.mu.Lock()
	q.pending = append(q.pending, job)
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *DecodeQueue) Stats() (pending, active int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending) - q.head, q.active
}

func (q *DecodeQueue) worker() {
	for {
		q.mu.Lock()
		if q.head < len(q.pending) {
			job := q.pending[q.head]
			q.pending[q.head] = nil
			q.head++
			q.active++
			if q.head == len(q.pending) {
				q.pending = nil
				q.head = 0
			} else if q.head >= 64 && q.head*2 >= len(q.pending) {
				q.pending = append(q.pending[:0], q.pending[q.head:]...)
				q.head = 0
			}
			q.mu.Unlock()
			job()
			q.mu.Lock()
			q.active--
			q.mu.Unlock()
			continue
		}
		q.mu.Unlock()
		<-q.wake
	}
}
