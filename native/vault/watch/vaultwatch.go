// Package watch provides a native, recursive filesystem watcher.
//
// Events are invalidations: consumers should reread the affected path (or
// rescan the workspace when Event.Overflow is true). The package never reads
// or mutates application/editor state.
package watch

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	eventBuffer = 64
	debounce    = 80 * time.Millisecond
)

var ErrClosed = errors.New("vault/watch: watcher is closed")

// Kind describes the resulting state change for a path.
type Kind string

// Operation is the backend-neutral set of filesystem notifications.
type Operation uint8

const (
	Create Operation = 1 << iota
	Write
	Remove
	Rename
)

// RawEvent is one low-level notification from a platform filesystem adapter.
type RawEvent struct {
	Path string
	Op   Operation
}

// Backend performs filesystem watch registration and delivers raw events.
// Implementations belong to platform adapters; this package owns coalescing,
// recursive coverage, selected-file policy, and overflow signaling.
type Backend interface {
	Add(path string) error
	Remove(path string) error
	Events() <-chan RawEvent
	Errors() <-chan error
	Close() error
}

// FileSystem supplies only the directory queries needed to maintain
// recursive watch coverage. The concrete OS calls live in platform adapters;
// vault/watch retains the policy for which directories to watch.
type FileSystem interface {
	IsDirectory(path string) (bool, error)
	WalkDirectories(root string, visit func(path string, walkErr error) error) error
}

const (
	Changed Kind = "changed"
	Removed Kind = "removed"
)

// Event invalidates a path. Path is absolute and clean; it is empty for an
// Overflow event, which asks the consumer to rescan the watched workspace.
type Event struct {
	Path     string
	Kind     Kind
	Deleted  bool
	Overflow bool
}

type watchRequest struct {
	path  string
	reply chan error
}

// Watcher recursively monitors root and optionally one separately selected
// file. The event channel is bounded; overflow is reported as a rescan event.
type Watcher struct {
	root       string
	backend    Backend
	fileSystem FileSystem
	events     chan Event
	requests   chan watchRequest
	stop       chan struct{}
	done       chan struct{}
	closeOnce  sync.Once
	wakeMu     sync.RWMutex
	wake       func()
}

// New starts watching root recursively. Newly created directories are added
// automatically. root must be an existing directory.
func New(root string, backend Backend, fileSystem FileSystem) (*Watcher, error) {
	if backend == nil {
		return nil, errors.New("vault/watch: filesystem backend is required")
	}
	if fileSystem == nil {
		_ = backend.Close()
		return nil, errors.New("vault/watch: filesystem access port is required")
	}
	started := false
	defer func() {
		if !started {
			_ = backend.Close()
		}
	}()
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("vault/watch: resolve root: %w", err)
	}
	isDirectory, err := fileSystem.IsDirectory(abs)
	if err != nil {
		return nil, fmt.Errorf("vault/watch: stat root: %w", err)
	}
	if !isDirectory {
		return nil, fmt.Errorf("vault/watch: root is not a directory: %s", abs)
	}
	w := &Watcher{
		root: abs, backend: backend, fileSystem: fileSystem, events: make(chan Event, eventBuffer),
		requests: make(chan watchRequest), stop: make(chan struct{}), done: make(chan struct{}),
	}
	if err := w.addTree(abs, nil); err != nil {
		return nil, err
	}
	go w.run()
	started = true
	return w, nil
}

// Events returns the bounded stream of filesystem invalidations. The channel
// is closed after Close has stopped the watcher.
func (w *Watcher) Events() <-chan Event { return w.events }

// SetWake installs an optional non-blocking, thread-safe wake callback. It is
// called after an event is queued; callbacks must not call back into UI state.
func (w *Watcher) SetWake(fn func()) {
	w.wakeMu.Lock()
	w.wake = fn
	w.wakeMu.Unlock()
}

// WatchFile additionally watches the parent of path so atomic rename saves
// remain observable. The selected file may be outside the workspace. Calling
// it again replaces the previously selected external file.
func (w *Watcher) WatchFile(path string) error {
	if path == "" {
		return errors.New("vault/watch: empty file path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("vault/watch: resolve file: %w", err)
	}
	parent := filepath.Dir(abs)
	isDirectory, err := w.fileSystem.IsDirectory(parent)
	if err != nil {
		return fmt.Errorf("vault/watch: stat file parent: %w", err)
	}
	if !isDirectory {
		return fmt.Errorf("vault/watch: file parent is not a directory: %s", parent)
	}
	req := watchRequest{path: abs, reply: make(chan error, 1)}
	select {
	case <-w.done:
		return ErrClosed
	case <-w.stop:
		return ErrClosed
	case w.requests <- req:
	}
	select {
	case err := <-req.reply:
		return err
	case <-w.done:
		return ErrClosed
	}
}

// Close stops all watcher goroutines and closes Events. It is safe to call
// more than once and waits until the event loop has exited.
func (w *Watcher) Close() error {
	w.closeOnce.Do(func() {
		close(w.stop)
		_ = w.backend.Close()
	})
	<-w.done
	return nil
}

func (w *Watcher) addTree(root string, known map[string]struct{}) error {
	return w.fileSystem.WalkDirectories(root, func(path string, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("vault/watch: walk %s: %w", path, walkErr)
		}
		path, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if known != nil {
			if _, ok := known[path]; ok {
				return nil
			}
		}
		if err := w.backend.Add(path); err != nil {
			return fmt.Errorf("vault/watch: watch %s: %w", path, err)
		}
		if known != nil {
			known[path] = struct{}{}
		}
		return nil
	})
}

func (w *Watcher) run() {
	defer close(w.done)
	defer close(w.events)
	defer w.backend.Close()

	dirs := make(map[string]struct{})
	_ = w.fileSystem.WalkDirectories(w.root, func(path string, err error) error {
		if err == nil {
			abs, _ := filepath.Abs(path)
			dirs[abs] = struct{}{}
		}
		return err
	})
	var selectedFile, externalParent string
	pending := make(map[string]Event)
	var timer *time.Timer
	var timerC <-chan time.Time
	overflowPending := false

	resetTimer := func() {
		if timer == nil {
			timer = time.NewTimer(debounce)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(debounce)
		}
		timerC = timer.C
	}
	queue := func(ev Event) bool {
		select {
		case w.events <- ev:
			w.wakeMu.RLock()
			wake := w.wake
			w.wakeMu.RUnlock()
			if wake != nil {
				wake()
			}
			return true
		default:
			return false
		}
	}
	flush := func() {
		if overflowPending {
			if queue(Event{Kind: Changed, Overflow: true}) {
				overflowPending = false
				clear(pending)
			}
			return
		}
		for path, ev := range pending {
			if !queue(ev) {
				overflowPending = true
				break
			}
			delete(pending, path)
		}
		if overflowPending {
			clear(pending)
		}
	}
	addPath := func(path string, op Operation) {
		path, err := filepath.Abs(filepath.Clean(path))
		if err != nil {
			return
		}
		if !within(w.root, path) && path != selectedFile {
			return
		}
		deleted := op&(Remove|Rename) != 0
		kind := Changed
		if deleted {
			kind = Removed
		}
		prior, ok := pending[path]
		if ok && !deleted && prior.Deleted {
			// A remove followed by a recreate of the same name ends present.
			prior = Event{Path: path, Kind: Changed}
		} else if ok && deleted {
			prior = Event{Path: path, Kind: Removed, Deleted: true}
		} else {
			prior = Event{Path: path, Kind: kind, Deleted: deleted}
		}
		pending[path] = prior
		if len(pending) > eventBuffer {
			overflowPending = true
			clear(pending)
		}
		resetTimer()
	}

	for {
		select {
		case <-w.stop:
			if timer != nil {
				timer.Stop()
			}
			return
		case req := <-w.requests:
			parent := filepath.Dir(req.path)
			if within(w.root, req.path) {
				if externalParent != "" {
					_ = w.backend.Remove(externalParent)
					delete(dirs, externalParent)
					externalParent = ""
				}
				selectedFile = req.path
				req.reply <- nil
				continue
			}
			if externalParent != "" && externalParent != parent {
				_ = w.backend.Remove(externalParent)
				delete(dirs, externalParent)
			}
			if _, ok := dirs[parent]; !ok {
				if err := w.backend.Add(parent); err != nil {
					req.reply <- fmt.Errorf("vault/watch: watch file parent: %w", err)
					continue
				}
				dirs[parent] = struct{}{}
			}
			selectedFile, externalParent = req.path, parent
			req.reply <- nil
		case ev, ok := <-w.backend.Events():
			if !ok {
				return
			}
			path, err := filepath.Abs(filepath.Clean(ev.Path))
			if err != nil {
				continue
			}
			if ev.Op&Create != 0 {
				if isDirectory, err := w.fileSystem.IsDirectory(path); err == nil && isDirectory {
					if err := w.addTree(path, dirs); err != nil {
						overflowPending = true
						clear(pending)
					}
				}
			}
			if ev.Op&(Remove|Rename) != 0 {
				for dir := range dirs {
					if dir == path || within(path, dir) {
						_ = w.backend.Remove(dir)
						delete(dirs, dir)
					}
				}
			}
			addPath(path, ev.Op)
		case err, ok := <-w.backend.Errors():
			if !ok {
				return
			}
			if err != nil {
				overflowPending = true
				clear(pending)
				resetTimer()
			}
		case <-timerC:
			timerC = nil
			flush()
			if overflowPending {
				resetTimer()
			}
		}
	}
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}
