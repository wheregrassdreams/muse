// Package events provides instance-scoped, typed in-process event dispatch.
// A dispatcher handles one event type; applications create one dispatcher per
// independently subscribed event contract.
package events

import (
	"sort"
	"sync"
)

type Dispatcher[T any] struct {
	mu       sync.RWMutex
	nextID   uint64
	closed   bool
	handlers map[uint64]func(T)
}

func New[T any]() *Dispatcher[T] {
	return &Dispatcher[T]{handlers: make(map[uint64]func(T))}
}

// Subscription removes one handler. Close is idempotent and may run while an
// event is being dispatched.
type Subscription struct {
	once   sync.Once
	remove func()
}

func (s *Subscription) Close() {
	if s != nil {
		s.once.Do(func() {
			if s.remove != nil {
				s.remove()
			}
		})
	}
}

func (d *Dispatcher[T]) Subscribe(handler func(T)) *Subscription {
	if d == nil || handler == nil {
		return &Subscription{}
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return &Subscription{}
	}
	d.nextID++
	id := d.nextID
	d.handlers[id] = handler
	d.mu.Unlock()
	return &Subscription{remove: func() {
		d.mu.Lock()
		delete(d.handlers, id)
		d.mu.Unlock()
	}}
}

// Publish dispatches synchronously in registration order. Handlers run
// without the dispatcher lock, so they may close subscriptions or publish
// other events without deadlocking.
func (d *Dispatcher[T]) Publish(event T) {
	if d == nil {
		return
	}
	d.mu.RLock()
	if d.closed {
		d.mu.RUnlock()
		return
	}
	ids := make([]uint64, 0, len(d.handlers))
	for id := range d.handlers {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	handlers := make([]func(T), 0, len(ids))
	for _, id := range ids {
		handlers = append(handlers, d.handlers[id])
	}
	d.mu.RUnlock()
	for _, handler := range handlers {
		handler(event)
	}
}

func (d *Dispatcher[T]) Close() {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.closed = true
	d.handlers = nil
	d.mu.Unlock()
}
