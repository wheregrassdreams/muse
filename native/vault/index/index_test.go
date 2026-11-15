package vaultindex

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func writeNote(t *testing.T, root, name, text string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fullScanResolve is the pre-optimization resolver kept as a semantic oracle.
func fullScanResolve(i *Index, current, target string) (Location, bool) {
	target = strings.TrimSpace(strings.Trim(target, "<>"))
	if target == "" {
		return Location{}, false
	}
	filePart, anchor, block := splitTarget(target)
	i.mu.RLock()
	defer i.mu.RUnlock()
	paths := sortedPaths(i.docs)
	var candidates []string
	if filePart == "" {
		candidates = append(candidates, cleanPath(current))
	} else {
		name := filepath.FromSlash(filePart)
		if filepath.Ext(name) == "" {
			name += ".md"
		}
		cur := filepath.Dir(cleanPath(current))
		for _, base := range []string{cur, "."} {
			p := cleanPath(filepath.Join(base, name))
			if _, ok := i.docs[p]; ok {
				candidates = appendUnique(candidates, p)
			}
		}
		want := strings.TrimSuffix(strings.ToLower(filepath.Base(name)), strings.ToLower(filepath.Ext(name)))
		for _, p := range paths {
			d := i.docs[p]
			if strings.EqualFold(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)), want) || equalAny(d.Title, want) || hasAnyFold(d.Aliases, want) {
				candidates = appendUnique(candidates, p)
			}
		}
	}
	if len(candidates) == 0 && filePart != "" {
		for _, p := range paths {
			d := i.docs[p]
			if equalAny(d.Title, filePart) || hasAnyFold(d.Aliases, filePart) {
				candidates = append(candidates, p)
			}
		}
	}
	if len(candidates) == 0 {
		return Location{}, false
	}
	d := i.docs[candidates[0]]
	if block != "" {
		for _, id := range d.Blocks {
			if strings.EqualFold(id.ID, block) {
				return Location{Path: candidates[0], Offset: id.Offset}, true
			}
		}
		return Location{}, false
	}
	if anchor != "" {
		for _, heading := range d.Headings {
			if strings.EqualFold(heading.Slug, slugify(anchor)) || strings.EqualFold(heading.Text, anchor) {
				return Location{Path: candidates[0], Offset: heading.Offset}, true
			}
		}
		return Location{}, false
	}
	return Location{Path: candidates[0]}, true
}

func TestWorkspaceIndexFeatures(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "notes/guide.md", `---
title: Guide Note
aliases:
  - Handbook
  - "Quick Start"
tags: [docs, onboarding]
---
# Getting Started
正文 contains İSTANBUL and 搜索词.
Target paragraph ^target-block
`)
	writeNote(t, root, "daily/today.md", "See [[Handbook#Getting Started|the guide]] and [local](../notes/guide.md).\n`code-only-search-token [[fake]] [no](x)`\n\n    indented-only-search-token [[indented fake]]\n<!-- comment-only-search-token [[hidden]] [x](y) -->\ninline %% inline-comment-search-token [[inline-fake]] [x](y) %% end\n%%\nmulti-comment-search-token [[multi-fake]] [x](y)\n%%\n")
	writeNote(t, root, ".hidden/secret.md", "secret searchable")
	idx := New()
	if err := idx.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	if _, ok := idx.Resolve("daily/today.md", "Handbook#Getting Started"); !ok {
		t.Fatal("alias and heading did not resolve")
	}
	loc, ok := idx.Resolve("daily/today.md", "notes/guide#^target-block")
	if !ok || loc.Path != "notes/guide.md" {
		t.Fatalf("block resolution = %#v, %v", loc, ok)
	}
	loc, ok = idx.Resolve("daily/today.md", "../notes/guide.md#Getting-Started")
	if !ok || loc.Path != "notes/guide.md" {
		t.Fatalf("relative path resolution = %#v, %v", loc, ok)
	}
	d := idx.docs["notes/guide.md"]
	if d.Title != "Getting Started" || strings.Join(d.Aliases, ",") != "Handbook,Quick Start" || strings.Join(d.Tags, ",") != "docs,onboarding" {
		t.Fatalf("metadata: %#v", d)
	}
	if len(d.Headings) != 1 || d.Headings[0].Slug != "getting-started" || len(d.Blocks) != 1 {
		t.Fatalf("structure: %#v %#v", d.Headings, d.Blocks)
	}
	links := idx.Outgoing("daily/today.md")
	if len(links) != 2 || links[0].Kind != "wiki" || links[1].Kind != "markdown" {
		t.Fatalf("links include masked fake links or wrong order: %#v", links)
	}
	if got := idx.Backlinks("notes/guide.md"); len(got) != 2 {
		t.Fatalf("backlinks: %#v", got)
	}
	if got := idx.Backlinks("Handbook#Getting Started"); len(got) != 1 || got[0].Kind != "wiki" {
		t.Fatalf("alias and heading backlinks: %#v", got)
	}
	if got := idx.Query("搜索词"); len(got) != 1 || got[0].Path != "notes/guide.md" || d.Source[got[0].Start:got[0].End] != "搜索词" {
		t.Fatalf("CJK query offsets: %#v", got)
	}
	if got := idx.Query("istanbul"); len(got) != 1 || d.Source[got[0].Start:got[0].End] != "İSTANBUL" {
		t.Fatalf("Unicode fold offsets: %#v", got)
	}
	for _, token := range []string{"code-only-search-token", "indented-only-search-token", "comment-only-search-token", "inline-comment-search-token", "multi-comment-search-token"} {
		if got := idx.Query(token); len(got) != 1 || got[0].Path != "daily/today.md" {
			t.Errorf("full-text search missed %q: %#v", token, got)
		}
	}
	if got := idx.Query("secret"); len(got) != 0 {
		t.Fatalf("hidden directory indexed: %#v", got)
	}
}

func TestOverlayRemoveAndRootBoundary(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "a.md", "disk value")
	idx := New()
	if err := idx.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	if err := idx.UpdateText("a.md", "unsaved buffer ^live\n"); err != nil {
		t.Fatal(err)
	}
	if got := idx.Query("unsaved"); len(got) != 1 {
		t.Fatalf("overlay not searchable: %#v", got)
	}
	if err := idx.UpdateText("../outside.md", "x"); err == nil {
		t.Fatal("accepted path outside workspace")
	}
	if err := idx.UpdateText("b.md", "temporary"); err != nil {
		t.Fatal(err)
	}
	idx.Remove("b.md")
	if got := idx.Query("temporary"); len(got) != 0 {
		t.Fatalf("remove left indexed content: %#v", got)
	}
	if err := idx.UpdateText("folder/a.md", "tree-removal-token"); err != nil {
		t.Fatal(err)
	}
	if err := idx.UpdateText("folder/sub/b.md", "tree-removal-token"); err != nil {
		t.Fatal(err)
	}
	if err := idx.UpdateText("folderish/c.md", "tree-removal-token"); err != nil {
		t.Fatal(err)
	}
	idx.Remove("folder")
	if got := idx.Query("tree-removal-token"); len(got) != 1 || got[0].Path != "folderish/c.md" {
		t.Fatalf("directory remove must prune descendants but preserve prefix siblings: %#v", got)
	}
}

func TestBacklinkIndexTracksSnapshotAndRemovalEvents(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "Target.md", "# Target\n")
	writeNote(t, root, "Other.md", "# Other\n")
	writeNote(t, root, "Source.md", "[[Target]]\n")
	idx := New()
	if err := idx.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	if got := idx.Backlinks("Target.md"); len(got) != 1 || got[0].Source != "Source.md" {
		t.Fatalf("initial backlinks = %+v", got)
	}
	if err := idx.UpdateText("Source.md", "[[Other]]\n"); err != nil {
		t.Fatal(err)
	}
	if got := idx.Backlinks("Target.md"); len(got) != 0 {
		t.Fatalf("stale backlink after source edit = %+v", got)
	}
	if got := idx.Backlinks("Other.md"); len(got) != 1 || got[0].Source != "Source.md" {
		t.Fatalf("updated backlink = %+v", got)
	}
	idx.Remove("Source.md")
	if got := idx.Backlinks("Other.md"); len(got) != 0 {
		t.Fatalf("stale backlink after source removal = %+v", got)
	}
}

func TestBacklinkQueryCacheHasBoundedCapacity(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "Target.md", "# Target\n")
	idx := New()
	if err := idx.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	for n := range backlinksCacheLimit + 10 {
		idx.Backlinks(fmt.Sprintf("missing-%d.md", n))
	}
	if len(idx.backlinksCache) != backlinksCacheLimit || len(idx.backlinksOrder) != backlinksCacheLimit {
		t.Fatalf("backlink cache sizes = %d/%d, limit %d", len(idx.backlinksCache), len(idx.backlinksOrder), backlinksCacheLimit)
	}
}

func TestResolveNameIndexMatchesFullScanAcrossMutations(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "z/Guide.md", "---\ntitle: Shared Title\naliases: [Common]\n---\n# Alpha\nA ^alpha\n")
	writeNote(t, root, "a/Guide.md", "---\ntitle: Other Title\naliases: [Common]\n---\n# Beta\nB ^beta\n")
	writeNote(t, root, "Guide.md", "# Root Guide\n")
	writeNote(t, root, "daily/Source.md", "")
	idx := New()
	if err := idx.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	check := func(phase string) {
		t.Helper()
		for _, current := range []string{"daily/Source.md", "z/Guide.md"} {
			for _, target := range []string{
				"Guide", "Guide.md", "../z/Guide.md", "a/Guide#Beta", "Shared Title#Alpha",
				"Common#Alpha", "Common#^alpha", "Guide#Root-Guide", "Other Title#Beta",
				"Missing", "Guide#not-a-heading", "Common#^missing",
			} {
				got, gotOK := idx.Resolve(current, target)
				want, wantOK := fullScanResolve(idx, current, target)
				if gotOK != wantOK || got != want {
					t.Errorf("%s Resolve(%q, %q) = %#v, %v; full scan = %#v, %v", phase, current, target, got, gotOK, want, wantOK)
				}
			}
		}
	}
	check("rebuilt")
	if err := idx.UpdateText("m/Guide.md", "---\ntitle: Shared Title\naliases: [Common, Fresh]\n---\n# Gamma\nC ^gamma\n"); err != nil {
		t.Fatal(err)
	}
	check("added")
	if err := idx.UpdateText("z/Guide.md", "---\ntitle: Renamed\naliases: [Changed]\n---\n# Delta\nD ^delta\n"); err != nil {
		t.Fatal(err)
	}
	check("modified")
	idx.Remove("a/Guide.md")
	check("removed")
	for key, paths := range idx.docPathsByName {
		if !sort.StringsAreSorted(paths) {
			t.Errorf("docPathsByName[%q] not sorted: %v", key, paths)
		}
	}
}

func TestRevisionNotePathsAndNoteContent(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "z/note.md", "---\ntitle: Zed\naliases: [Alias]\n---\nbody\n")
	writeNote(t, root, "a/note.markdown", "alpha")
	idx := New()
	if got := idx.Revision(); got != 0 {
		t.Fatalf("initial revision = %d", got)
	}
	if err := idx.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	firstRev := idx.Revision()
	if firstRev != 1 {
		t.Fatalf("revision after rebuild = %d", firstRev)
	}
	if got := idx.NotePaths(); strings.Join(got, ",") != "a/note.markdown,z/note.md" {
		t.Fatalf("note paths = %v", got)
	}
	title, source, ok := idx.NoteContent(filepath.Join(root, "z", "note.md"))
	if !ok || title != "note" || !strings.Contains(source, "body") {
		t.Fatalf("canonical NoteContent = %q, %q, %v", title, source, ok)
	}
	if _, _, ok := idx.NoteContent("../outside.md"); ok {
		t.Fatal("NoteContent accepted a path outside the vault")
	}
	if err := idx.UpdateText("z/note.md", "---\ntitle: Zed\naliases: [Alias]\n---\nbody\n"); err != nil {
		t.Fatal(err)
	}
	if got := idx.Revision(); got != firstRev {
		t.Fatalf("identical snapshot advanced revision: %d -> %d", firstRev, got)
	}
	if err := idx.UpdateText("z/note.md", "changed"); err != nil {
		t.Fatal(err)
	}
	changedRev := idx.Revision()
	if changedRev != firstRev+1 {
		t.Fatalf("changed snapshot revision = %d, want %d", changedRev, firstRev+1)
	}
	idx.Remove("absent.md")
	if got := idx.Revision(); got != changedRev {
		t.Fatalf("no-op removal advanced revision: %d -> %d", changedRev, got)
	}
	idx.Remove("z/note.md")
	if got := idx.Revision(); got != changedRev+1 {
		t.Fatalf("removal revision = %d, want %d", got, changedRev+1)
	}
}

func TestBacklinkSourcesGroupsSourceOrder(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "Target.md", "# Target\n")
	writeNote(t, root, "z.md", "[[Target]] and [[Target]]\n")
	writeNote(t, root, "a.md", "[[Target]]\n")
	idx := New()
	if err := idx.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	got := idx.BacklinkSources("Target")
	want := []BacklinkSource{{Source: "a.md", Start: 0, Count: 1}, {Source: "z.md", Start: 0, Count: 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BacklinkSources = %#v, want %#v", got, want)
	}
	got[0].Source = "mutated"
	if next := idx.BacklinkSources("Target"); !reflect.DeepEqual(next, want) {
		t.Fatalf("caller mutation escaped into source cache: %#v", next)
	}
	if err := idx.UpdateText("z.md", "no backlink now\n"); err != nil {
		t.Fatal(err)
	}
	if next := idx.BacklinkSources("Target"); !reflect.DeepEqual(next, want[:1]) {
		t.Fatalf("source cache was not invalidated after index update: %#v", next)
	}
}

func TestQueryLimitStopsAtRequestedCount(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "b.md", "needle needle")
	writeNote(t, root, "a.md", "needle needle")
	idx := New()
	if err := idx.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	got := idx.QueryLimit("needle", 3)
	if len(got) != 3 {
		t.Fatalf("QueryLimit count = %d, want 3", len(got))
	}
	if got[0].Path != "a.md" || got[1].Path != "a.md" || got[2].Path != "b.md" {
		t.Fatalf("QueryLimit order: %#v", got)
	}
	if all := idx.Query("needle"); len(all) != 4 {
		t.Fatalf("Query should retain unbounded behavior, got %d matches", len(all))
	}
	if got := idx.QueryLimit("needle", 0); len(got) != 0 {
		t.Fatalf("nonpositive limit should return no matches: %#v", got)
	}
}

func TestSnapshotCopiesAndApplySnapshot(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "a.md", "disk source")
	idx := New()
	if err := idx.Rebuild(root); err != nil {
		t.Fatal(err)
	}
	d, err := idx.ParseSnapshot("a.md", "---\naliases: [Live]\n---\nnew source")
	if err != nil {
		t.Fatal(err)
	}
	idx.ApplySnapshot(d)
	d.Aliases[0] = "mutated input"
	got, ok := idx.Document("a.md")
	if !ok || got.Source == "" || got.Aliases[0] != "Live" {
		t.Fatalf("stored snapshot = %#v, %v", got, ok)
	}
	got.Aliases[0] = "mutated output"
	all := idx.Snapshot()
	if len(all) != 1 || all[0].Aliases[0] != "Live" || all[0].Source != "---\naliases: [Live]\n---\nnew source" {
		t.Fatalf("snapshot copy = %#v", all)
	}
	disk, err := os.ReadFile(filepath.Join(root, "a.md"))
	if err != nil || string(disk) != "disk source" {
		t.Fatalf("source file changed: %q, %v", disk, err)
	}
}
