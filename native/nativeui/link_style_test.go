package nativeui

import (
	"os"
	"path/filepath"
	"testing"

	mdedit "muse/editor"
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
	a, tt, _ := runtimeFixtureAt(t, root, filepath.Join(root, "one/current.md"))
	waitDocument(t, tt, func() bool { return a.vaultSnapshot.Ready })
	opts := a.editorOptions()
	if got := opts.ResolveWikiLink(opts.CurrentPath, "target"); got != mdedit.LinkTargetValid {
		t.Fatalf("target state=%v", got)
	}
	if got := opts.ResolveWikiLink(opts.CurrentPath, "missing"); got != mdedit.LinkTargetInvalid {
		t.Fatalf("missing target state=%v", got)
	}
	if got := opts.ResolveWikiLink(opts.CurrentPath, "#Local"); got != mdedit.LinkTargetValid {
		t.Fatalf("local heading state=%v", got)
	}
	firstContext, revision := opts.LinkContext, opts.LinkRevision

	setDocumentPathForTest(a, filepath.Join(root, "two/current.md"))
	opts = a.editorOptions()
	if opts.LinkContext == firstContext || opts.LinkRevision != revision {
		t.Fatalf("document context changed incorrectly: %+v", opts)
	}
	if got := opts.ResolveWikiLink(opts.CurrentPath, "target"); got != mdedit.LinkTargetValid {
		t.Fatalf("vault-wide target state=%v", got)
	}
	if got := opts.ResolveWikiLink(opts.CurrentPath, "#Local"); got != mdedit.LinkTargetInvalid {
		t.Fatalf("second-document local heading state=%v", got)
	}

	if err := os.Remove(filepath.Join(root, "one/target.md")); err != nil {
		t.Fatal(err)
	}
	if err := a.vault.Refresh(filepath.Join(root, "one/target.md")); err != nil {
		t.Fatal(err)
	}
	waitDocument(t, tt, func() bool { _, ok := a.vault.Document("one/target.md"); return !ok })
	a.vaultSnapshot = a.vault.Snapshot()
	opts = a.editorOptions()
	if opts.LinkRevision <= revision || opts.ResolveWikiLink(opts.CurrentPath, "target") != mdedit.LinkTargetInvalid {
		t.Fatalf("updated vault snapshot did not refresh link style: revision=%d state=%v", opts.LinkRevision, opts.ResolveWikiLink(opts.CurrentPath, "target"))
	}
}
