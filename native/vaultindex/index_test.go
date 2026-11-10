package vaultindex

import (
	"os"
	"path/filepath"
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
	if d.Title != "Guide Note" || strings.Join(d.Aliases, ",") != "Handbook,Quick Start" || strings.Join(d.Tags, ",") != "docs,onboarding" {
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
