package links_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"muse/vault/links"
)

func TestResolutionAndDerivedBacklinks(t *testing.T) {
	root := t.TempDir()
	idx := links.New()
	err := idx.Rebuild(root, []links.Document{
		{Path: "notes/guide.md", Title: "Getting Started", Aliases: []string{"Handbook"},
			Headings: []links.Heading{{Text: "Installation", Slug: "installation", Offset: 21}},
			Blocks:   []links.BlockID{{ID: "target", Offset: 52}}},
		{Path: "daily/today.md", Links: []links.Link{
			{Source: "daily/today.md", Target: "Handbook#Installation", Kind: "wiki", Start: 2, End: 12},
			{Source: "daily/today.md", Target: "../notes/guide.md#^target", Kind: "wiki", Start: 20, End: 32},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		target string
		offset int
	}{
		{"Handbook#Installation", 21},
		{"../notes/guide.md#^target", 52},
	} {
		loc, ok := idx.Resolve("daily/today.md", tc.target)
		if !ok || loc.Path != "notes/guide.md" || loc.Offset != tc.offset {
			t.Errorf("Resolve(%q) = %+v, %v", tc.target, loc, ok)
		}
	}
	if got := idx.Outgoing("daily/today.md"); len(got) != 2 || got[0].Target != "Handbook#Installation" {
		t.Fatalf("Outgoing = %+v", got)
	}
	got := idx.Backlinks("Handbook#Installation")
	if len(got) != 1 || got[0].Source != "daily/today.md" {
		t.Fatalf("Backlinks = %+v", got)
	}
	sources := idx.BacklinkSources("notes/guide.md")
	if len(sources) != 1 || sources[0] != (links.BacklinkSource{Source: "daily/today.md", Start: 2, Count: 2}) {
		t.Fatalf("BacklinkSources = %+v", sources)
	}
}

func TestApplySnapshotAndRemoveUpdateLinkGraph(t *testing.T) {
	root := t.TempDir()
	idx := links.New()
	if err := idx.Rebuild(root, []links.Document{
		{Path: "Target.md"},
		{Path: "Other.md"},
		{Path: "Source.md", Links: []links.Link{{Source: "Source.md", Target: "Target", Kind: "wiki"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := idx.Backlinks("Target"); len(got) != 1 {
		t.Fatalf("initial backlinks = %+v", got)
	}
	if err := idx.ApplySnapshot(links.Document{Path: "Source.md", Links: []links.Link{{Source: "Source.md", Target: "Other", Kind: "wiki"}}}); err != nil {
		t.Fatal(err)
	}
	if got := idx.Backlinks("Target"); len(got) != 0 {
		t.Fatalf("stale backlinks after source update = %+v", got)
	}
	if got := idx.Backlinks("Other"); len(got) != 1 {
		t.Fatalf("updated backlinks = %+v", got)
	}
	idx.Remove(filepath.Join(root, "Source.md"))
	if got := idx.Backlinks("Other"); len(got) != 0 {
		t.Fatalf("stale backlinks after source removal = %+v", got)
	}
}

func TestAssetBacklinksAndBoundedCache(t *testing.T) {
	root := t.TempDir()
	idx := links.New()
	if err := idx.Rebuild(root, []links.Document{{Path: "Notes/Source.md", Links: []links.Link{
		{Source: "Notes/Source.md", Target: "../Attachments/diagram.png", Kind: "image"},
		{Source: "Notes/Source.md", Target: "../Attachments/photo.jpg", Kind: "markdown"},
	}}}); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"Attachments/diagram.png", filepath.Join("Notes", "..", "Attachments", "photo.jpg"), filepath.Join(root, "Attachments", "diagram.png")} {
		if got := idx.Backlinks(target); len(got) != 1 {
			t.Errorf("Backlinks(%q) = %+v", target, got)
		}
	}
	for n := range 300 {
		idx.Backlinks("missing-" + string(rune('a'+n%26)) + ".md")
	}
	if got := idx.Backlinks("Attachments/diagram.png"); !reflect.DeepEqual(got, idx.Backlinks(filepath.Join(root, "Attachments", "diagram.png"))) {
		t.Fatalf("asset backlink results vary across path forms: %+v", got)
	}
}
