package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVaultFixtureImagesResolveAndDecode(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "vault-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(root, "Projects", "Atlas")
	cases := []struct {
		name, src, wantPath string
		minWidth, minHeight int
	}{
		{"WebP", "../../Attachments/landscape.webp", filepath.Join(root, "Attachments", "landscape.webp"), 640, 459},
		{"PNG relative", "../../Attachments/diagram.png", filepath.Join(root, "Attachments", "diagram.png"), 685, 510},
		{"JPEG vault-root", "Attachments/photo.jpg", filepath.Join(root, "Attachments", "photo.jpg"), 4096, 2304},
		{"GIF bare attachment", "animation.gif", filepath.Join(root, "Attachments", "animation.gif"), 82, 82},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolved := ResolveImageSource(tc.src, base, root)
			if resolved != tc.wantPath {
				t.Fatalf("ResolveImageSource(%q) = %q, want %q", tc.src, resolved, tc.wantPath)
			}
			bmp := LoadImage(resolved, "", nil)
			if bmp == nil {
				t.Fatalf("image did not decode: %s", resolved)
			}
			w, h := bmp.Size()
			if int(w) < tc.minWidth || int(h) < tc.minHeight {
				t.Fatalf("decoded size = %dx%d, want at least %dx%d", w, h, tc.minWidth, tc.minHeight)
			}
		})
	}
}

func TestResolveImageSourceWithSharedLookup(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "Notes")
	assets := filepath.Join(root, "Attachments")
	for _, dir := range []string{base, assets} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	image := filepath.Join(assets, "shared.png")
	if err := os.WriteFile(image, []byte("catalog metadata only"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	lookup := func(name string) string {
		calls++
		if name == "shared.png" {
			return image
		}
		return ""
	}
	if got := ResolveImageSourceWithLookup("shared.png", base, root, lookup); got != image {
		t.Fatalf("resolved = %q, want %q", got, image)
	}
	if calls != 1 {
		t.Fatalf("lookup calls = %d, want 1", calls)
	}
	remote := "https://example.test/image.png"
	if got := ResolveImageSourceWithLookup(remote, base, root, lookup); got != remote {
		t.Fatalf("remote = %q", got)
	}
	if calls != 1 {
		t.Fatalf("remote invoked lookup: calls=%d", calls)
	}
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ResolveImageSourceWithLookup("outside.png", base, root, func(string) string { return outside }); got == outside {
		t.Fatalf("outside resolver escaped root: %q", got)
	}
}

func TestVaultFixtureBareDuplicateAttachmentHasStableResolution(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "vault-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(root, "Journal")
	want := filepath.Join(root, "Assets", "diagram.png")
	for attempt := 0; attempt < 3; attempt++ {
		if got := ResolveImageSource("diagram.png", base, root); got != want {
			t.Fatalf("duplicate bare attachment resolution = %q, want deterministic %q", got, want)
		}
	}
}

func TestImageResolutionCannotEscapeVaultRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "vault")
	base := filepath.Join(root, "Notes")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside.png")
	if err := os.WriteFile(outside, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ResolveImageSource("../../outside.png", base, root); got != "" {
		t.Fatalf("outside relative path resolved to %q", got)
	}
	link := filepath.Join(root, "linked-outside.png")
	if err := os.Symlink(outside, link); err == nil {
		if got := ResolveImageSource("linked-outside.png", root, root); got != "" {
			t.Fatalf("symlink to outside file resolved to %q", got)
		}
	}
}

func TestImageResolutionKeepsMissingFallbackUnderTmpSymlinkRoot(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "muse-image-root-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	base := filepath.Join(root, "Notes")
	attachments := filepath.Join(root, "Attachments")
	for _, dir := range []string{base, attachments} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fixtureRoot := filepath.Join("..", "..", "..", "testdata", "vault-fixture")
	image, err := os.ReadFile(filepath.Join(fixtureRoot, "Attachments", "photo.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attachments, "fallback.jpg"), image, 0o600); err != nil {
		t.Fatal(err)
	}

	got := ResolveImageSource("fallback.jpg", base, root)
	want := filepath.Join(attachments, "fallback.jpg")
	if got != want {
		t.Fatalf("missing local image did not fall back to vault attachment: got %q, want %q", got, want)
	}
	if LoadImage(got, "", nil) == nil {
		t.Fatal("resolved JPEG fallback did not decode")
	}
}

func TestVaultFixtureGalleryRendersKnownAndMissingAssets(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "vault-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(root, "Home.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := Parse(string(source))
	images := 0
	for _, block := range doc.Blocks {
		if block.Kind != Image {
			continue
		}
		images++
		resolved := ResolveImageSource(block.Src, root, root)
		if strings.Contains(block.Src, "not-present") {
			if LoadImage(resolved, "", nil) != nil {
				t.Errorf("missing fixture image unexpectedly loaded: %s", block.Src)
			}
			if got := MissingImageMessage(block.Src); !strings.Contains(got, block.Src) {
				t.Errorf("missing image status %q does not name source %q", got, block.Src)
			}
			continue
		}
		if LoadImage(resolved, "", nil) == nil {
			t.Errorf("gallery image %q failed to render from %q", block.Src, resolved)
		}
	}
	if images != 7 {
		t.Fatalf("parsed Home gallery has %d images, want 7", images)
	}
}
