package links

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	vaultapp "muse/application/vault"
	vaultfiles "muse/vault/files"
	vaultlinks "muse/vault/links"
)

type fakeLinkNavigationSession struct {
	snapshot vaultapp.Snapshot
	links    map[string]vaultlinks.Location
	images   map[string]string
	content  map[string]string
}

func (f *fakeLinkNavigationSession) Snapshot() vaultapp.Snapshot { return f.snapshot }
func (f *fakeLinkNavigationSession) ResolveLink(_, target string) (vaultlinks.Location, bool) {
	location, ok := f.links[target]
	return location, ok
}
func (f *fakeLinkNavigationSession) ResolveImageTarget(_, target string) (string, bool) {
	path, ok := f.images[target]
	return path, ok
}
func (f *fakeLinkNavigationSession) NoteContent(path string) (string, string, bool) {
	source, ok := f.content[path]
	return filepath.Base(path), source, ok
}
func (f *fakeLinkNavigationSession) FindImage(string) string { return "" }

type localImageFiles struct{}

func (localImageFiles) Stat(path string) (os.FileInfo, error)    { return os.Stat(path) }
func (localImageFiles) ReadFile(path string) ([]byte, error)     { return os.ReadFile(path) }
func (localImageFiles) EvalSymlinks(path string) (string, error) { return filepath.EvalSymlinks(path) }

type fakeLinkPathKinds map[string]vaultfiles.EntryKind

func (f fakeLinkPathKinds) Kind(path string) (vaultfiles.EntryKind, error) {
	kind, ok := f[path]
	if !ok {
		return 0, errors.New("missing")
	}
	return kind, nil
}

func TestLinkNavigationResolvesTypedDestinations(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "notes", "here.md")
	imagePath := filepath.Join(root, "assets", "image.png")
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imagePath, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	session := &fakeLinkNavigationSession{
		snapshot: vaultapp.Snapshot{Root: root, Ready: true},
		links: map[string]vaultlinks.Location{
			"Target#Section":  {Path: "notes/target.md", Offset: 42},
			"notes/target.md": {Path: "notes/target.md", Offset: 0},
		},
		images: map[string]string{"image.png": imagePath},
	}
	knownFile := filepath.Join(root, "notes", "other.md")
	navigation := NewLinkNavigation(fakeLinkPathKinds{knownFile: vaultfiles.EntryRegular}, localImageFiles{})

	for _, test := range []struct {
		name string
		href string
		want LinkDestination
	}{
		{"empty", "  ", LinkDestination{Kind: LinkDestinationIgnored}},
		{"heading", "#intro", LinkDestination{Kind: LinkDestinationHeading, Value: "intro"}},
		{"external", "https://example.test", LinkDestination{Kind: LinkDestinationExternal, Value: "https://example.test"}},
		{"wiki note", "wiki:Target#Section", LinkDestination{Kind: LinkDestinationNote, Path: "notes/target.md", Offset: 42}},
		{"wiki image", "wiki:image.png", LinkDestination{Kind: LinkDestinationImage, Path: imagePath}},
		{"local note", "other.md", LinkDestination{Kind: LinkDestinationFile, Path: knownFile}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := navigation.ResolveClick(session, current, test.href); got != test.want {
				t.Fatalf("ResolveClick(%q) = %+v, want %+v", test.href, got, test.want)
			}
		})
	}
}

func TestLinkNavigationKeepsWikiTargetPendingUntilIndexReady(t *testing.T) {
	root := t.TempDir()
	session := &fakeLinkNavigationSession{snapshot: vaultapp.Snapshot{Root: root}}
	got := NewLinkNavigation(nil, nil).ResolveClick(session, filepath.Join(root, "note.md"), "wiki:Target#Section")
	want := LinkDestination{Kind: LinkDestinationPending, Value: "Target#Section"}
	if got != want {
		t.Fatalf("ResolveClick() = %+v, want %+v", got, want)
	}
}

func TestLinkNavigationPreviewEmbedAndValidity(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "notes", "here.md")
	session := &fakeLinkNavigationSession{
		snapshot: vaultapp.Snapshot{Root: root, Ready: true},
		links:    map[string]vaultlinks.Location{"Target#Heading": {Path: "notes/target.md", Offset: 19}},
		content:  map[string]string{"notes/target.md": "# Target\nbody"},
	}
	navigation := NewLinkNavigation(nil, nil)
	preview, ok := navigation.ResolveWikiPreview(session, current, "Target#Heading")
	if !ok || preview != (WikiPreviewNote{Title: "target.md", Path: "notes/target.md", Source: "# Target\nbody", Offset: 19, Root: root}) {
		t.Fatalf("ResolveWikiPreview() = %+v, %v", preview, ok)
	}
	embed, ok := navigation.ResolveWikiEmbed(session, current, "Target#Heading")
	if !ok || embed != (WikiEmbed{Path: "notes/target.md", Source: "# Target\nbody"}) {
		t.Fatalf("ResolveWikiEmbed() = %+v, %v", embed, ok)
	}
	if got := navigation.WikiLinkValidity(session, current, "Target#Heading"); got != LinkValid {
		t.Fatalf("WikiLinkValidity() = %v, want valid", got)
	}
	if got := navigation.WikiLinkValidity(session, current, "Missing"); got != LinkInvalid {
		t.Fatalf("WikiLinkValidity() = %v, want invalid", got)
	}
	if got := navigation.WikiLinkValidity(nil, current, "Target"); got != LinkUnknown {
		t.Fatalf("WikiLinkValidity(nil) = %v, want unknown", got)
	}
}

func TestLinkNavigationRejectsWikiResolutionOutsideVault(t *testing.T) {
	root := t.TempDir()
	session := &fakeLinkNavigationSession{
		snapshot: vaultapp.Snapshot{Root: root, Ready: true},
		links:    map[string]vaultlinks.Location{"Target": {Path: "target.md"}},
	}
	got := NewLinkNavigation(nil, nil).ResolveClick(session, filepath.Join(filepath.Dir(root), "outside.md"), "wiki:Target")
	if got.Kind != LinkDestinationIgnored {
		t.Fatalf("outside-vault link resolved unexpectedly: %+v", got)
	}
}
