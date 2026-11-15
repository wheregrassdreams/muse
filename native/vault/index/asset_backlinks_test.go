package vaultindex

import (
	"path/filepath"
	"testing"
)

func TestBacklinksResolveImageAssetsFromWikiAndMarkdownEmbeds(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "Notes/Current.md", "![[../Attachments/diagram.png]]\n\n![photo](../Attachments/photo.jpg)\n")
	writeNote(t, root, "Notes/Other.md", "![[Attachments/diagram.png]]\n")
	idx := New()
	if err := idx.Rebuild(root); err != nil {
		t.Fatal(err)
	}

	got := idx.Backlinks("Attachments/diagram.png")
	if len(got) != 2 || got[0].Source != "Notes/Current.md" || got[0].Kind != "wiki" || got[1].Source != "Notes/Other.md" {
		t.Fatalf("wiki image backlinks = %#v", got)
	}
	got = idx.Backlinks(filepath.Join("Notes", "..", "Attachments", "photo.jpg"))
	if len(got) != 1 || got[0].Source != "Notes/Current.md" || got[0].Kind != "image" {
		t.Fatalf("Markdown image backlinks = %#v", got)
	}
	got = idx.Backlinks(filepath.Join(root, "Attachments", "diagram.png"))
	if len(got) != 2 || got[0].Source != "Notes/Current.md" || got[1].Source != "Notes/Other.md" {
		t.Fatalf("absolute image backlinks = %#v", got)
	}
}
