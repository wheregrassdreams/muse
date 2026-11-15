package images

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type testLocalFiles struct{}

func (testLocalFiles) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }
func (testLocalFiles) ReadFile(path string) ([]byte, error)  { return os.ReadFile(path) }
func (testLocalFiles) EvalSymlinks(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}

func TestResolveSourceDecodesEscapesAndConfinesVaultPaths(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Attachments", "photo space.png")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := testLocalFiles{}
	got := ResolveSource("photo%20space.png", SourceOptions{BaseDir: filepath.Dir(path), Root: root, LocalFiles: files})
	if got != path {
		t.Fatalf("escaped source resolved to %q, want %q", got, path)
	}
	if got := ResolveSource("../outside.png", SourceOptions{BaseDir: root, Root: root, LocalFiles: files}); got != "" {
		t.Fatalf("vault traversal resolved to %q", got)
	}
	const remote = "https://example.test/a%20b.png"
	if got := ResolveSource(remote, SourceOptions{Root: root}); got != remote {
		t.Fatalf("remote source changed to %q", got)
	}
	const data = "data:image/png;base64,abc"
	if got := ResolveSource(data, SourceOptions{Root: root}); got != data {
		t.Fatalf("data source changed")
	}
}

func TestResolveSourceUsesBareNameLookupOnly(t *testing.T) {
	root := t.TempDir()
	attachment := filepath.Join(root, "Attachments", "icon.svg")
	if err := os.MkdirAll(filepath.Dir(attachment), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(attachment, []byte("<svg/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	lookupCalls := 0
	lookup := func(name string) string {
		lookupCalls++
		if name == "icon.svg" {
			return attachment
		}
		return ""
	}
	files := testLocalFiles{}
	got := ResolveSource("icon.svg", SourceOptions{BaseDir: root, Root: root, Lookup: lookup, LocalFiles: files})
	if got != attachment || lookupCalls != 1 {
		t.Fatalf("bare attachment resolution=(%q,%d calls), want (%q,1)", got, lookupCalls, attachment)
	}
	lookupCalls = 0
	ResolveSource("nested/icon.svg", SourceOptions{BaseDir: root, Root: root, Lookup: lookup, LocalFiles: files})
	if lookupCalls != 0 {
		t.Fatalf("path reference invoked attachment lookup %d times", lookupCalls)
	}
}

func TestLocalSourceRequiresFileCapability(t *testing.T) {
	if _, err := ReadLocalSource("local.png", SourceOptions{}); err != ErrLocalFilesRequired {
		t.Fatalf("ReadLocalSource error = %v, want %v", err, ErrLocalFilesRequired)
	}
	asset := NewLoader(1<<20).Load("local.png", LoadOptions{}, nil, nil)
	if !asset.Failed() || asset.Failure() != ErrLocalFilesRequired {
		t.Fatalf("asset failure = %v, failed=%v; want LocalFiles-required error", asset.Failure(), asset.Failed())
	}
	loaded := make(chan error, 1)
	LoadSourceBytes("local.png", SourceOptions{}, nil, func(_ []byte, err error) { loaded <- err })
	select {
	case err := <-loaded:
		if err != ErrLocalFilesRequired {
			t.Fatalf("LoadSourceBytes error = %v, want %v", err, ErrLocalFilesRequired)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("LoadSourceBytes did not report the missing file capability")
	}
}

func TestResolveSourceFailsClosedAcrossSymlinkedMissingPath(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "vault")
	outside := filepath.Join(parent, "outside")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	files := testLocalFiles{}
	if got := ResolveSource("linked/not-created.png", SourceOptions{BaseDir: root, Root: root, LocalFiles: files}); got != "" {
		t.Fatalf("missing path through outside symlink resolved to %q", got)
	}
	if got := ResolveSource("nested/not-created.png", SourceOptions{BaseDir: root, Root: root, LocalFiles: files}); got != filepath.Join(root, "nested", "not-created.png") {
		t.Fatalf("safe missing path resolved to %q", got)
	}
}
