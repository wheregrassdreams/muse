package nativeui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreshFixtureInitialCaretAlignsWithHeadingInAppView(t *testing.T) {
	fixture := filepath.Join("..", "testdata", "vault-fixture")
	source, err := os.ReadFile(filepath.Join(fixture, "Home.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(source), "# Home") {
		t.Fatalf("Home fixture no longer starts with its heading: %q", source[:min(20, len(source))])
	}

	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	a, tt, path := runtimeFixtureAt(t, root, filepath.Join(root, "Home.md"))
	if got := a.documentPath(); got != path {
		t.Fatalf("active path = %q, want %q", got, path)
	}
	for range 2 {
		tt.Frame()
	}

	commands := a.noteEditor().NavigationCommands()
	caretOffset := commands.Caret()
	headingTextOffset := strings.Index(string(source), "Home")
	_, caretY, caretOK := commands.WindowPoint(caretOffset)
	_, headingY, headingOK := commands.WindowPoint(headingTextOffset)
	if !caretOK || !headingOK {
		t.Fatalf("fresh app geometry missing: caret=%v heading=%v", caretOK, headingOK)
	}
	if delta := caretY - headingY; delta < -1 || delta > 1 {
		t.Fatalf("fresh fixture caret y=%.1f is %.1f DIPs from first heading text y=%.1f; want same line", caretY, delta, headingY)
	}
}
