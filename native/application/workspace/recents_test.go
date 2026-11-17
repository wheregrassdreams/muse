package workspace

import (
	"path/filepath"
	"reflect"
	"testing"
)

type memoryRecentStore struct {
	entries []RecentEntry
	paths   map[string]bool
	err     error
}

func (s *memoryRecentStore) Load() ([]RecentEntry, error) {
	return append([]RecentEntry(nil), s.entries...), s.err
}
func (s *memoryRecentStore) Save(entries []RecentEntry) error {
	s.entries = append([]RecentEntry(nil), entries...)
	return s.err
}
func (s *memoryRecentStore) Exists(path string) bool { return s.paths[path] }

func TestRecentItemsDeduplicatesBoundsAndFiltersMissingPaths(t *testing.T) {
	store := &memoryRecentStore{
		entries: []RecentEntry{{Path: "a.md"}, {Path: "b.md"}, {Path: "missing.md"}},
		paths:   map[string]bool{"a.md": true, "b.md": true},
	}
	items := NewRecentItems(store)
	if err := items.Add("a.md", true); err != nil {
		t.Fatal(err)
	}
	entries := items.Entries()
	if entries[0] != (RecentEntry{Path: "a.md", IsDir: true}) {
		t.Fatalf("deduplicated recent = %+v", entries[0])
	}
	if got := items.Live(2); !reflect.DeepEqual(got, []RecentEntry{{Path: "a.md", IsDir: true}, {Path: "b.md"}}) {
		t.Fatalf("live entries = %+v", got)
	}
	for i := 0; i < MaxRecentEntries+3; i++ {
		path := string(rune('a'+i)) + ".md"
		if err := items.Add(path, false); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(items.Entries()); got != MaxRecentEntries {
		t.Fatalf("retained entries = %d, want %d", got, MaxRecentEntries)
	}
}

func TestRecentItemsWithoutRepositoryIsSafe(t *testing.T) {
	items := NewRecentItems(nil)
	if err := items.Add("note.md", false); err != ErrUnavailable {
		t.Fatalf("Add error = %v, want %v", err, ErrUnavailable)
	}
	if items.Entries() != nil || items.Live(MaxVisibleRecent) != nil {
		t.Fatal("unconfigured recent service returned entries")
	}
}

func TestRecentItemsLiveNotesAppliesDocumentAndWorkspacePolicy(t *testing.T) {
	root := filepath.Join("", "vault")
	inside := filepath.Join(root, "note.MD")
	outside := filepath.Join("other-vault", "note.md")
	store := &memoryRecentStore{
		entries: []RecentEntry{
			{Path: filepath.Join(root, "folder"), IsDir: true},
			{Path: filepath.Join(root, "readme.txt")},
			{Path: outside},
			{Path: inside},
			{Path: filepath.Join(root, "second.md")},
		},
		paths: map[string]bool{
			filepath.Join(root, "folder"):     true,
			filepath.Join(root, "readme.txt"): true,
			outside:                           true,
			inside:                            true,
			filepath.Join(root, "second.md"):  true,
		},
	}
	items := NewRecentItems(store)

	want := []RecentEntry{{Path: inside}, {Path: filepath.Join(root, "second.md")}}
	if got := items.LiveNotes(root, MaxVisibleRecent); !reflect.DeepEqual(got, want) {
		t.Fatalf("LiveNotes() = %+v, want %+v", got, want)
	}
	if got := items.LiveNotes(root, 1); !reflect.DeepEqual(got, want[:1]) {
		t.Fatalf("LiveNotes(limit=1) = %+v, want %+v", got, want[:1])
	}
	if got := items.LiveNotes(root, MaxRecentEntries+1); !reflect.DeepEqual(got, want) {
		t.Fatalf("LiveNotes(oversized limit) = %+v, want %+v", got, want)
	}
}
