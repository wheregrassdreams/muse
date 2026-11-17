package links

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestWikiSuggestionsUseVaultRelativeCanonicalTargets(t *testing.T) {
	documents := []Document{
		{Path: "Notes/Second.md", Title: "Second title", Aliases: []string{"second", "two"}},
		{Path: "Notes/First.markdown", Title: "First title"},
		{Path: "Assets/ignored.txt", Title: "Not a note"},
	}
	got := WikiSuggestions(documents, []string{"Assets/diagram.svg", "Attachments/photo.png"})
	want := []Suggestion{
		{Target: "Assets/diagram.svg", Title: "diagram.svg"},
		{Target: "Attachments/photo.png", Title: "photo.png"},
		{Target: "Notes/First", Title: "First title"},
		{Target: "Notes/Second", Title: "Second title", Aliases: []string{"second", "two"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("WikiSuggestions() = %#v, want %#v", got, want)
	}
	documents[0].Aliases[0] = "mutated"
	if got[3].Aliases[0] != "second" {
		t.Fatalf("suggestion aliases retained caller memory: %#v", got[3].Aliases)
	}
}

func TestSuggestionCacheIsScopedToOwnerAndBothRevisions(t *testing.T) {
	cache := &SuggestionCache[string]{}
	items := []Suggestion{{Target: "Notes/A", Title: "A", Aliases: []string{"alias"}}}
	cache.Store("vault-a", "/vault", 3, 7, items)
	items[0].Aliases[0] = "caller mutation"

	got, ok := cache.Get("vault-a", "/vault", 3, 7)
	if !ok || len(got) != 1 || got[0].Aliases[0] != "alias" {
		t.Fatalf("cache hit = %#v, %v", got, ok)
	}
	got[0].Aliases[0] = "returned mutation"
	gotAgain, ok := cache.Get("vault-a", "/vault", 3, 7)
	if !ok || gotAgain[0].Aliases[0] != "alias" {
		t.Fatalf("cache exposed retained alias memory: %#v, %v", gotAgain, ok)
	}
	for _, key := range []struct {
		owner         string
		root          string
		indexRevision uint64
		filesRevision uint64
	}{
		{"vault-b", "/vault", 3, 7},
		{"vault-a", "/other", 3, 7},
		{"vault-a", "/vault", 4, 7},
		{"vault-a", "/vault", 3, 8},
	} {
		if _, ok := cache.Get(key.owner, key.root, key.indexRevision, key.filesRevision); ok {
			t.Errorf("cache unexpectedly hit for stale key %+v", key)
		}
	}
	cache.Clear()
	if _, ok := cache.Get("vault-a", "/vault", 3, 7); ok {
		t.Fatal("cleared suggestion cache still hit")
	}
}

func TestResolveImageTargetUsesCurrentThenVaultThenBareName(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "tmp", "vault")
	images := []string{
		"Attachments/photo.png",
		"Projects/photo.png",
		"Projects/Design/diagram space.svg",
		"B/same.png",
		"A/same.png",
	}
	for _, tc := range []struct{ current, target, want string }{
		{"Projects/Note.md", "photo.png", "Projects/photo.png"},
		{"Projects/Note.md", "Attachments/photo.png", "Attachments/photo.png"},
		{"Projects/Design/Note.md", "diagram%20space.svg#preview", "Projects/Design/diagram space.svg"},
		{"Notes/Note.md", "same.png", "A/same.png"},
		{"Notes/Note.md", "../Attachments/photo.png", "Attachments/photo.png"},
	} {
		got, ok := ResolveImageTarget(root, tc.current, tc.target, images)
		if !ok || got != tc.want {
			t.Errorf("ResolveImageTarget(%q, %q) = %q, %v; want %q", tc.current, tc.target, got, ok, tc.want)
		}
	}
}

func TestResolveImageTargetRejectsTraversalAndOutsideAbsolutePaths(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "tmp", "vault")
	images := []string{"Attachments/safe.png"}
	for _, target := range []string{"../../secret.png", "/tmp/outside.png", "https://example.test/image.png", "data:image/png;base64,abc"} {
		if got, ok := ResolveImageTarget(root, "Notes/Note.md", target, images); ok || got != "" {
			t.Errorf("ResolveImageTarget(%q) = %q, %v; want unresolved", target, got, ok)
		}
	}
	inside := filepath.Join(root, "Attachments", "safe.png")
	if got, ok := ResolveImageTarget(root, "Notes/Note.md", inside, images); !ok || got != "Attachments/safe.png" {
		t.Fatalf("inside absolute path = %q, %v", got, ok)
	}
	if got, ok := ResolveImageTarget(root, "Notes/Note.md", "file://"+inside, images); !ok || got != "Attachments/safe.png" {
		t.Fatalf("inside file URL = %q, %v", got, ok)
	}
}
