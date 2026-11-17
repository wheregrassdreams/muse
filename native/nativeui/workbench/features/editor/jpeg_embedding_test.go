package editor

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"muse/markdown/images"
)

func TestJPEGEmbedsResolveDocumentVaultAndBareAttachmentPaths(t *testing.T) {
	root := t.TempDir()
	notes := filepath.Join(root, "notes")
	assets := filepath.Join(root, "assets")
	os.MkdirAll(notes, 0700)
	os.MkdirAll(assets, 0700)
	for _, name := range []string{"photo.jpg", "photo.jpeg", "photo space.JPG"} {
		f, err := os.Create(filepath.Join(assets, name))
		if err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 120, 60))
		img.Set(0, 0, color.White)
		if err = jpeg.Encode(f, img, nil); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	for _, src := range []string{"![jpg](../assets/photo.jpg)", "![jpeg](assets/photo.jpeg)", "![[photo.jpg]]", "![[photo space.JPG]]", "![encoded](../assets/photo%20space.JPG)"} {
		h := newHarness(t, "before\n\n"+src+"\n")
		opts := h.e.Options()
		opts.BaseDir = notes
		opts.AssetRoot = root
		opts.ResolveImage = func(src, base, assetRoot string) string {
			return images.ResolveSource(src, images.SourceOptions{
				BaseDir: base, Root: assetRoot, LocalFiles: opts.ImageLocalFiles,
				Lookup: func(name string) string {
					return filepath.Join(assets, name)
				},
			})
		}
		h.e.SetOptions(opts)
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			h.frame()
			if len(h.e.imageHits) > 0 && h.e.imageHits[0].obj.bmp != nil {
				break
			}
			time.Sleep(time.Millisecond)
		}
		if len(h.e.imageHits) == 0 || h.e.imageHits[0].obj.bmp == nil {
			var assetState string
			if len(h.e.imageHits) > 0 && h.e.imageHits[0].obj.asset != nil {
				asset := h.e.imageHits[0].obj.asset
				assetState = fmt.Sprintf(" loaded=%v failed=%v frame=%v version=%d", asset.Loaded(), asset.Failed(), asset.CurrentFrame() != nil, asset.FrameVersion())
			}
			t.Fatalf("JPEG did not render: %s%s", src, assetState)
		}
		if got := h.e.imageHits[0].obj; got.w != 120 || got.h != 60 {
			t.Fatalf("decoded JPEG dimensions were not laid out for %s: got %.1fx%.1f, want 120x60", src, got.w, got.h)
		}
	}
}
