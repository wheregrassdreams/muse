package editor

import "sync"

// paintMutex keeps callbacks from observing or re-entering an editor while a
// paint/mutation lock is held. A callback requested during a critical section
// is delivered synchronously by Unlock, after releasing the underlying lock.
type paintMutex struct {
	sync.Mutex
	owner *Editor
}

func (m *paintMutex) Unlock() {
	var callback func()
	if m.owner != nil && m.owner.changePending.Swap(false) {
		callback = m.owner.opts.OnChange
	}
	m.Mutex.Unlock()
	if callback != nil {
		callback()
	}
}
