package vault

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestResolveSearchHitPathKeepsPathsInsideVault(t *testing.T) {
	root := t.TempDir()
	session := &Session{root: root}
	got, err := session.ResolveSearchHitPath("notes/a.md")
	if err != nil || got != filepath.Join(root, "notes", "a.md") {
		t.Fatalf("ResolveSearchHitPath() = %q, %v", got, err)
	}
	for _, path := range []string{"../outside.md", filepath.Join(root, "absolute.md"), "."} {
		if _, err := session.ResolveSearchHitPath(path); !errors.Is(err, ErrInvalidSearchHitPath) {
			t.Fatalf("ResolveSearchHitPath(%q) error = %v, want invalid hit path", path, err)
		}
	}
}
