// Package watcher adapts fsnotify to the backend-neutral vault watch port.
package watcher

import (
	"sync"

	"github.com/fsnotify/fsnotify"
	vaultwatch "muse/vault/watch"
)

// Backend translates operating-system notifications into the vault watch
// contract while keeping fsnotify types out of feature modules.
type Backend struct {
	watcher *fsnotify.Watcher
	events  chan vaultwatch.RawEvent
	errors  chan error
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
}

func New() (*Backend, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	b := &Backend{watcher: fw, events: make(chan vaultwatch.RawEvent, 64), errors: make(chan error, 16), stop: make(chan struct{}), done: make(chan struct{})}
	go b.forward()
	return b, nil
}

func (b *Backend) Add(path string) error              { return b.watcher.Add(path) }
func (b *Backend) Remove(path string) error           { return b.watcher.Remove(path) }
func (b *Backend) Events() <-chan vaultwatch.RawEvent { return b.events }
func (b *Backend) Errors() <-chan error               { return b.errors }

func (b *Backend) Close() error {
	var err error
	b.once.Do(func() { close(b.stop); err = b.watcher.Close() })
	<-b.done
	return err
}

func (b *Backend) forward() {
	defer close(b.done)
	defer close(b.events)
	defer close(b.errors)
	for {
		select {
		case <-b.stop:
			return
		case event, ok := <-b.watcher.Events:
			if !ok {
				return
			}
			var op vaultwatch.Operation
			if event.Op&fsnotify.Create != 0 {
				op |= vaultwatch.Create
			}
			if event.Op&fsnotify.Write != 0 {
				op |= vaultwatch.Write
			}
			if event.Op&fsnotify.Remove != 0 {
				op |= vaultwatch.Remove
			}
			if event.Op&fsnotify.Rename != 0 {
				op |= vaultwatch.Rename
			}
			select {
			case b.events <- vaultwatch.RawEvent{Path: event.Name, Op: op}:
			case <-b.stop:
				return
			}
		case err, ok := <-b.watcher.Errors:
			if !ok {
				return
			}
			select {
			case b.errors <- err:
			case <-b.stop:
				return
			}
		}
	}
}
