package naviconpicker

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	componenticons "muse/nativeui/workbench/framework/components/icons"
)

type memoryPreferences struct {
	mu    sync.Mutex
	items map[string]string
	err   error
}

func (p *memoryPreferences) Get(path string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.items[path]
}

func (p *memoryPreferences) Set(path, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	p.items[path] = id
	return nil
}

func (p *memoryPreferences) Delete(path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	delete(p.items, path)
	return nil
}

func TestHostSearchAndChooseUseVaultRelativePreferencePort(t *testing.T) {
	preferences := &memoryPreferences{items: map[string]string{}}
	var invalidations atomic.Int32
	notices := make(chan Notice, 2)
	searchQueries := make(chan string, 2)
	controller := NewHost(Ports{
		Preferences: preferences,
		RelativePath: func(path string) (string, bool) {
			if path == "/vault/Notes/Home.md" {
				return "Notes/Home.md", true
			}
			return "", false
		},
		Search: func(query string) []Option {
			searchQueries <- query
			return []Option{{ID: "bookmark", Label: "Bookmark"}}
		},
		Invalidate: func() { invalidations.Add(1) },
		Notify:     func(notice Notice) { notices <- notice },
	})

	if !controller.Open("/vault/Notes/Home.md") {
		t.Fatal("Open rejected a valid vault path")
	}
	awaitPicker(t, controller, func(model ViewModel) bool { return !model.Loading && len(model.Results) == 1 })
	if got := <-searchQueries; got != "" {
		t.Fatalf("initial catalog query = %q, want empty query", got)
	}

	controller.Dispatch(Action{Kind: QueryChanged, Query: "  book  "})
	awaitPicker(t, controller, func(model ViewModel) bool { return !model.Loading && model.Query == "  book  " })
	if got := <-searchQueries; got != "book" {
		t.Fatalf("catalog query = %q, want trimmed query", got)
	}
	controller.Dispatch(Action{Kind: Choose, ID: "bookmark"})
	if got := preferences.Get("Notes/Home.md"); got != "bookmark" {
		t.Fatalf("saved icon ID = %q, want bookmark", got)
	}
	if notice := <-notices; notice.Kind != PreferenceUpdated {
		t.Fatalf("save notice = %+v, want preference-updated", notice)
	}
	if controller.IsOpen() {
		t.Fatal("picker remained open after choosing an icon")
	}
	if invalidations.Load() < 2 {
		t.Fatalf("invalidation count = %d, want open and search updates", invalidations.Load())
	}
}

func TestHostReportsPreferenceFailureAndRestoresDefault(t *testing.T) {
	preferences := &memoryPreferences{items: map[string]string{"Notes/Home.md": "bookmark"}, err: errors.New("read only")}
	notices := make(chan Notice, 2)
	controller := NewHost(Ports{
		Preferences:  preferences,
		RelativePath: func(string) (string, bool) { return "Notes/Home.md", true },
		Search:       func(string) []Option { return nil },
		Notify:       func(notice Notice) { notices <- notice },
	})
	if !controller.Open("/vault/Notes/Home.md") {
		t.Fatal("Open rejected a valid vault path")
	}
	awaitPicker(t, controller, func(model ViewModel) bool { return !model.Loading })
	controller.Dispatch(Action{Kind: RestoreDefault})
	if notice := <-notices; notice.Kind != PreferenceSaveFailed || notice.Err == nil {
		t.Fatalf("restore notice = %+v, want typed save failure", notice)
	}
	if controller.IsOpen() {
		t.Fatal("picker remained open after restore-default action")
	}
}

func TestSearchOptionsUsesSharedSVGLookup(t *testing.T) {
	want, ok := componenticons.Lookup("bookmark")
	if !ok {
		t.Fatal("shared icon catalog is missing bookmark")
	}
	for _, option := range SearchOptions("bookmark") {
		if option.ID == "bookmark" {
			if option.Icon != want {
				t.Fatal("search option did not reuse the shared parsed SVG")
			}
			return
		}
	}
	t.Fatal("search options did not include bookmark")
}

func awaitPicker(t *testing.T, controller *Host, predicate func(ViewModel) bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if predicate(controller.Snapshot()) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for picker state: %+v", controller.Snapshot())
}
