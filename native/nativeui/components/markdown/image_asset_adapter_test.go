package markdown

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImageAssetAdaptsFeatureFrameToBitmap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "adapter.png")
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 24, 12))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded := make(chan struct{}, 1)
	asset := LoadImageAsset(path, "", func() { loaded <- struct{}{} })
	select {
	case <-loaded:
	case <-time.After(5 * time.Second):
		t.Fatal("image asset did not finish loading")
	}
	if !asset.Loaded() {
		t.Fatal("asset is not loaded")
	}
	if got, ok := asset.IntrinsicSize(); !ok || got != image.Pt(24, 12) {
		t.Fatalf("intrinsic size=(%v,%v), want (24,12)", got, ok)
	}
	if _, changed := asset.Advance(time.Now()); !changed {
		t.Fatal("static image did not publish its first frame")
	}
	bitmap := asset.Bitmap()
	if bitmap == nil {
		t.Fatal("renderer adapter did not create a bitmap")
	}
	if w, h := bitmap.Size(); w != 24 || h != 12 {
		t.Fatalf("bitmap size=(%d,%d), want (24,12)", w, h)
	}
	if got := LoadImageAsset(path, "", nil); got != asset {
		t.Fatal("same feature Asset did not reuse its live UI adapter")
	}
}
