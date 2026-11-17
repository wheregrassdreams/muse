package watch_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	filesystemwatcher "muse/platform/filesystem/watcher"
	vaultwatch "muse/vault/watch"
)

type Event = vaultwatch.Event
type Watcher = vaultwatch.Watcher

const (
	Removed     = vaultwatch.Removed
	eventBuffer = 64
)

func waitEvent(t *testing.T, events <-chan Event, match func(Event) bool) Event {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("event channel closed before matching event")
			}
			if match(ev) {
				return ev
			}
		case <-deadline.C:
			t.Fatalf("timed out waiting for filesystem event (queued=%d)", len(events))
		}
	}
}

func newTempWatcher(t *testing.T) (string, *Watcher) {
	t.Helper()
	root := t.TempDir()
	backend, err := filesystemwatcher.New()
	if err != nil {
		t.Fatal(err)
	}
	w, err := vaultwatch.New(root, backend, filesystemwatcher.NewDirectoryAccess())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return root, w
}

func TestWatchCreateWriteRemoveAndNewDirectories(t *testing.T) {
	root, w := newTempWatcher(t)
	events := w.Events()
	file := filepath.Join(root, "first.md")
	if err := os.WriteFile(file, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, events, func(ev Event) bool { return ev.Path == file && !ev.Deleted })

	if err := os.WriteFile(file, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, events, func(ev Event) bool { return ev.Path == file && !ev.Deleted })

	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	ev := waitEvent(t, events, func(ev Event) bool { return ev.Path == file && ev.Deleted })
	if ev.Kind != Removed {
		t.Fatalf("remove kind = %q, want %q", ev.Kind, Removed)
	}

	dir := filepath.Join(root, "new", "nested")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, events, func(ev Event) bool { return ev.Path == filepath.Join(root, "new") })
	nestedFile := filepath.Join(dir, "created.md")
	if err := os.WriteFile(nestedFile, []byte("nested"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, events, func(ev Event) bool { return ev.Path == nestedFile && !ev.Deleted })
}

func TestWatchFileOutsideWorkspaceAndAtomicRenameSave(t *testing.T) {
	_, w := newTempWatcher(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "doc.md")
	if err := os.WriteFile(target, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := w.WatchFile(target); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(outside, ".doc.md.tmp")
	if err := os.WriteFile(tmp, []byte("after"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, target); err != nil {
		t.Fatal(err)
	}
	ev := waitEvent(t, w.Events(), func(ev Event) bool { return ev.Path == target })
	if ev.Deleted {
		t.Fatalf("atomic replacement reported deletion: %+v", ev)
	}
}

func TestCloseClosesEventsAndWatchFileReturnsClosed(t *testing.T) {
	root, w := newTempWatcher(t)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	select {
	case _, ok := <-w.Events():
		if ok { // Drain any events queued before Close until channel closure.
			deadline := time.NewTimer(time.Second)
			defer deadline.Stop()
			for {
				select {
				case _, ok := <-w.Events():
					if !ok {
						goto closed
					}
				case <-deadline.C:
					t.Fatal("event channel remained open after Close")
				}
			}
		}
	case <-time.After(time.Second):
		t.Fatal("event channel did not close")
	}
closed:
	if err := w.WatchFile(filepath.Join(root, "doc.md")); err != vaultwatch.ErrClosed {
		t.Fatalf("WatchFile after Close = %v, want ErrClosed", err)
	}
}

func TestEventStormProducesBoundedQueueAndRescanSignal(t *testing.T) {
	root, w := newTempWatcher(t)
	for i := 0; i < eventBuffer*4; i++ {
		path := filepath.Join(root, "storm-"+strconv.Itoa(i)+".md")
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	waitEvent(t, w.Events(), func(ev Event) bool { return ev.Overflow })
	if len(w.Events()) > eventBuffer {
		t.Fatalf("event queue exceeded capacity: %d", len(w.Events()))
	}
}
