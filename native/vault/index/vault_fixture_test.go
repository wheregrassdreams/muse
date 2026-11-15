package vaultindex

import (
	"path/filepath"
	"testing"
)

func TestNestedVaultFixtureLinksAndBacklinks(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "testdata", "vault-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	idx := New()
	if err := idx.Rebuild(root); err != nil {
		t.Fatal(err)
	}

	wantPaths := []string{
		"ATTRIBUTION.md",
		"Home.md",
		"Journal/2026-11-12.md",
		"Projects/Atlas/Design Notes.md",
		"Projects/Atlas/Overview.md",
		"Projects/Atlas/Rendering Notes.md",
	}
	snapshot := idx.Snapshot()
	if len(snapshot) != len(wantPaths) {
		t.Fatalf("vault fixture indexed %d notes, want %d: %#v", len(snapshot), len(wantPaths), snapshot)
	}
	for n, want := range wantPaths {
		if snapshot[n].Path != want {
			t.Errorf("snapshot[%d].Path = %q, want %q", n, snapshot[n].Path, want)
		}
	}

	for _, tc := range []struct {
		current, target, path string
		wantHeading           string
	}{
		{"Home.md", "Projects/Atlas/Overview", "Projects/Atlas/Overview.md", ""},
		{"Home.md", "Projects/Atlas/Rendering Notes", "Projects/Atlas/Rendering Notes.md", ""},
		{"Journal/2026-11-12.md", "../Projects/Atlas/Overview#Architecture", "Projects/Atlas/Overview.md", "architecture"},
		{"Journal/2026-11-12.md", "Atlas Overview", "Projects/Atlas/Overview.md", ""},
		{"Projects/Atlas/Design Notes.md", "Atlas Overview#Architecture", "Projects/Atlas/Overview.md", "architecture"},
		{"Projects/Atlas/Rendering Notes.md", "Atlas Overview#Architecture", "Projects/Atlas/Overview.md", "architecture"},
	} {
		loc, ok := idx.Resolve(tc.current, tc.target)
		if !ok || loc.Path != tc.path {
			t.Errorf("Resolve(%q, %q) = %#v, %v; want %q", tc.current, tc.target, loc, ok, tc.path)
			continue
		}
		if tc.wantHeading != "" {
			doc, _ := idx.Document(loc.Path)
			found := false
			for _, heading := range doc.Headings {
				if heading.Slug == tc.wantHeading && heading.Offset == loc.Offset {
					found = true
				}
			}
			if !found {
				t.Errorf("Resolve(%q) offset %d does not identify heading %q", tc.target, loc.Offset, tc.wantHeading)
			}
		}
	}

	if _, ok := idx.Resolve("Home.md", "Not Yet Written"); ok {
		t.Fatal("unresolved fixture link unexpectedly resolved")
	}
	backlinks := idx.Backlinks("Projects/Atlas/Overview.md")
	if len(backlinks) != 6 {
		t.Fatalf("Overview backlinks = %#v, want Home's link and embed, two Atlas notes, and two links from Journal", backlinks)
	}
	wantSources := map[string]bool{
		"Home.md":                           false,
		"Journal/2026-11-12.md":             false,
		"Projects/Atlas/Design Notes.md":    false,
		"Projects/Atlas/Rendering Notes.md": false,
	}
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
	if got := idx.Backlinks("Home"); len(got) != 1 || got[0].Source != "Projects/Atlas/Overview.md" {
		t.Fatalf("Home backlinks = %#v, want Overview", got)
	}
	if got := idx.Backlinks("Atlas Overview#Architecture"); len(got) != 3 {
		t.Fatalf("heading backlinks through alias = %#v, want both Atlas notes and journal", got)
	}
}

func TestVaultFixtureIndexUpdatesLinksAfterDeletion(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "testdata", "vault-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	idx := New()
	if err := idx.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	if got := idx.Backlinks("Projects/Atlas/Overview"); len(got) != 6 {
		t.Fatalf("initial backlinks = %d, want 6", len(got))
	}
	idx.Remove("Journal/2026-11-12.md")
	if got := idx.Backlinks("Projects/Atlas/Overview"); len(got) != 4 {
		t.Fatalf("backlinks after journal removal = %d, want Home's link and embed plus two Atlas notes, got %d", len(got), len(got))
	}
}
