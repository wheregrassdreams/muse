package nativeui

import (
	"path/filepath"
	"testing"
)

func TestPropertyBacklinksUseSharedSourceProjection(t *testing.T) {
	a, tt, _ := runtimeFixtureWithNotes(t, map[string]string{
		"Source.md": "first [[note]] and later [[note]]",
	})
	a.openProperties()
	tt.Frame()

	sources := a.documentBacklinkSources()
	if len(sources) != 1 {
		t.Fatalf("source projection has %d rows, want 1: %+v", len(sources), sources)
	}
	if got := sources[0]; got.Source != "Source.md" || got.Count != 2 || got.Start <= 0 {
		t.Fatalf("unexpected grouped source: %+v", got)
	}
	if !tt.HasText("Source（2 处）") {
		t.Fatalf("property backlinks card did not display grouped count: %q", tt.Texts())
	}
	targetPath := a.path
	if err := tt.Click("属性反向链接：Source.md"); err != nil {
		t.Fatalf("click grouped property backlink: %v", err)
	}
	if filepath.Clean(a.path) != filepath.Join(a.root, "Source.md") || a.indexedOffset != sources[0].Start {
		t.Fatalf("grouped backlink did not navigate to first link: path=%q offset=%d want path=%q offset=%d", a.path, a.indexedOffset, filepath.Join(a.root, "Source.md"), sources[0].Start)
	}
	// Keep the original target selected while checking live index updates below.
	a.path = targetPath
	a.openProperties()
	tt.Frame()

	if err := a.vault.index.UpdateText("Source.md", "link changed to [[another-note]]"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := a.documentBacklinkSources(); len(got) != 0 {
		t.Fatalf("property projection retained stale links after target changed: %+v", got)
	}
	if !tt.HasText("无数值") {
		t.Fatalf("empty backlinks state was not rendered after index update: %q", tt.Texts())
	}

	if err := a.vault.index.UpdateText("Source.md", "[[note]]"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := a.documentBacklinkSources(); len(got) != 1 || got[0].Count != 1 {
		t.Fatalf("property projection did not reflect changed link count: %+v", got)
	}
	if tt.HasText("Source（2 处）") {
		t.Fatalf("property card retained stale grouped count after index update: %q", tt.Texts())
	}
}
