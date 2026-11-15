package links_test

import (
	"path/filepath"
	"testing"

	vaultindex "muse/vault/index"
	"muse/vault/links"
)

func fixtureLinkIndex(t *testing.T) (*vaultindex.Index, *links.Index, string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "vault-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	parsed := vaultindex.New()
	if err := parsed.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	graph := links.New()
	if err := graph.Rebuild(root, parsed.LinkSnapshot()); err != nil {
		t.Fatal(err)
	}
	return parsed, graph, root
}

func TestNestedVaultFixtureLinksAndBacklinks(t *testing.T) {
	_, graph, _ := fixtureLinkIndex(t)
	for _, tc := range []struct {
		current, target, path string
		offsetRequired        bool
	}{
		{"Home.md", "Projects/Atlas/Overview", "Projects/Atlas/Overview.md", false},
		{"Home.md", "Projects/Atlas/Rendering Notes", "Projects/Atlas/Rendering Notes.md", false},
		{"Journal/2026-11-12.md", "../Projects/Atlas/Overview#Architecture", "Projects/Atlas/Overview.md", true},
		{"Journal/2026-11-12.md", "Atlas Overview", "Projects/Atlas/Overview.md", false},
		{"Projects/Atlas/Design Notes.md", "Atlas Overview#Architecture", "Projects/Atlas/Overview.md", true},
		{"Projects/Atlas/Rendering Notes.md", "Atlas Overview#Architecture", "Projects/Atlas/Overview.md", true},
	} {
		loc, ok := graph.Resolve(tc.current, tc.target)
		if !ok || loc.Path != tc.path {
			t.Errorf("Resolve(%q, %q) = %+v, %v; want %q", tc.current, tc.target, loc, ok, tc.path)
			continue
		}
		if tc.offsetRequired && loc.Offset <= 0 {
			t.Errorf("Resolve(%q) returned invalid heading offset %d", tc.target, loc.Offset)
		}
	}
	if _, ok := graph.Resolve("Home.md", "Not Yet Written"); ok {
		t.Fatal("unresolved fixture link unexpectedly resolved")
	}
	backlinks := graph.Backlinks("Projects/Atlas/Overview.md")
	if len(backlinks) != 6 {
		t.Fatalf("Overview backlinks = %#v, want six", backlinks)
	}
	wantSources := map[string]bool{"Home.md": false, "Journal/2026-11-12.md": false, "Projects/Atlas/Design Notes.md": false, "Projects/Atlas/Rendering Notes.md": false}
	for _, link := range backlinks {
		if _, ok := wantSources[link.Source]; !ok {
			t.Errorf("unexpected backlink source %q", link.Source)
		}
		wantSources[link.Source] = true
	}
	for source, found := range wantSources {
		if !found {
			t.Errorf("missing backlink source %q", source)
		}
	}
	if got := graph.Backlinks("Home"); len(got) != 1 || got[0].Source != "Projects/Atlas/Overview.md" {
		t.Fatalf("Home backlinks = %#v", got)
	}
	if got := graph.Backlinks("Atlas Overview#Architecture"); len(got) != 3 {
		t.Fatalf("heading backlinks through alias = %#v", got)
	}
}

func TestVaultFixtureGraphUpdatesAfterDeletion(t *testing.T) {
	parsed, graph, _ := fixtureLinkIndex(t)
	if got := graph.Backlinks("Projects/Atlas/Overview"); len(got) != 6 {
		t.Fatalf("initial backlinks = %d, want 6", len(got))
	}
	parsed.Remove("Journal/2026-11-12.md")
	graph.Remove("Journal/2026-11-12.md")
	if got := graph.Backlinks("Projects/Atlas/Overview"); len(got) != 4 {
		t.Fatalf("backlinks after removal = %d, want 4", len(got))
	}
}
