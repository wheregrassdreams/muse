package nativeui

import (
	"os"
	"path/filepath"
	"testing"

	applicationvault "muse/application/vault"
	mdedit "muse/editor"
	vaultindexsource "muse/platform/filesystem/vaultindexsource"
	vaultindex "muse/vault/index"
)

func TestEditorOptionsResolveWikiLinksFromCurrentVaultContext(t *testing.T) {
	root := t.TempDir()
	for path, text := range map[string]string{
		"one/current.md": "# Local",
		"one/target.md":  "# Target",
		"two/current.md": "# Other",
	} {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	source, err := vaultindexsource.New(root)
	if err != nil {
		t.Fatal(err)
	}
	idx := vaultindex.New(source)
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}
	links, err := applicationvault.NewLinkGraph(root, idx)
	if err != nil {
		t.Fatal(err)
	}
	a := New()
	a.root = root
	a.path = filepath.Join(root, "one/current.md")
	a.source = "[[target]]"
	a.vault = &vaultRuntime{root: root, index: idx, links: links}
	opts := a.editorOptions()
	if got := opts.ResolveWikiLink(opts.CurrentPath, "target"); got != mdedit.LinkTargetValid {
		t.Fatalf("existing indexed target state=%v, want valid", got)
	}
	if got := opts.ResolveWikiLink(opts.CurrentPath, "missing"); got != mdedit.LinkTargetInvalid {
		t.Fatalf("missing indexed target state=%v, want invalid", got)
	}
	if got := opts.ResolveWikiLink(opts.CurrentPath, "#Local"); got != mdedit.LinkTargetValid {
		t.Fatalf("current-document heading state=%v, want valid", got)
	}
	firstContext, revision := opts.LinkContext, opts.LinkRevision

	// The source is identical and the index revision is unchanged, but the
	// relative-link resolution base changes when another note is opened.
	a.path = filepath.Join(root, "two/current.md")
	opts = a.editorOptions()
	if opts.LinkContext == firstContext || opts.LinkRevision != revision {
		t.Fatalf("document context did not change independently of index revision: %+v", opts)
	}
	// Bare wiki names deliberately retain vault-wide title fallback. Only
	// the local heading target is expected to change with current document.
	if got := opts.ResolveWikiLink(opts.CurrentPath, "target"); got != mdedit.LinkTargetValid {
		t.Fatalf("vault-wide wiki fallback changed: state=%v", got)
	}
	if got := opts.ResolveWikiLink(opts.CurrentPath, "#Local"); got != mdedit.LinkTargetInvalid {
		t.Fatalf("local heading in second document state=%v, want invalid", got)
	}

	// A watcher update changes the revision and resolver result without a disk
	// scan in the editor render path.
	if err := os.Remove(filepath.Join(root, "one/target.md")); err != nil {
		t.Fatal(err)
	}
	if err := idx.Update(filepath.Join(root, "one/target.md")); err != nil {
		t.Fatal(err)
	}
	if err := links.Rebuild(); err != nil {
		t.Fatal(err)
	}
	opts = a.editorOptions()
	if opts.LinkRevision != revision+1 || opts.ResolveWikiLink(opts.CurrentPath, "target") != mdedit.LinkTargetInvalid {
		t.Fatalf("updated index did not refresh link style: revision=%d state=%v", opts.LinkRevision, opts.ResolveWikiLink(opts.CurrentPath, "target"))
	}
	a.vault = nil
	if got := a.editorOptions().ResolveWikiLink(a.path, "target"); got != mdedit.LinkTargetUnknown {
		t.Fatalf("unavailable vault state=%v, want unknown", got)
	}
}
