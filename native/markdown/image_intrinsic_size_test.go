package markdown

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImageAssetPublishesIntrinsicSizeBeforePixelDecode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wide.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 640, 320))); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	ready := make(chan struct{})
	sized := make(chan struct {
		size   image.Point
		loaded bool
	}, 1)
	completed := make(chan struct{}, 1)
	var asset *ImageAsset
	asset = LoadImageAssetWithSize(path, "", func() {
		// Wait until the API returned the shared asset before re-entering it from
		// the worker callback. This also verifies the callback does not hold mu.
		<-ready
		size, ok := asset.IntrinsicSize()
		if !ok {
			sized <- struct {
				size   image.Point
				loaded bool
			}{image.Point{}, true}
			return
		}
		sized <- struct {
			size   image.Point
			loaded bool
		}{size, asset.Loaded()}
	}, func() { completed <- struct{}{} })
	close(ready)

	select {
	case got := <-sized:
		if got.size != image.Pt(640, 320) {
			t.Fatalf("intrinsic size = %v, want (640,320)", got.size)
		}
		if got.loaded {
			t.Fatal("asset was already loaded when intrinsic size callback ran")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("intrinsic size callback did not run")
	}
	select {
	case <-completed:
	case <-time.After(5 * time.Second):
		t.Fatal("completion callback did not run")
	}
	if !asset.Loaded() {
		t.Fatal("asset not loaded after completion callback")
	}
	if got, ok := asset.IntrinsicSize(); !ok || got != image.Pt(640, 320) {
		t.Fatalf("stored intrinsic size = (%v, %v), want (640,320), true", got, ok)
	}
	var cacheCallbacks int
	cached := LoadImageAssetWithSize(path, "", func() { cacheCallbacks++ }, func() { cacheCallbacks++ })
	if cached != asset {
		t.Fatal("completed load did not reuse its cached asset")
	}
	if size, ok := cached.IntrinsicSize(); !ok || size != image.Pt(640, 320) {
		t.Fatalf("cached intrinsic size = (%v, %v), want (640,320), true", size, ok)
	}
	if cacheCallbacks != 0 {
		t.Fatalf("completed cache hit replayed %d callbacks, want none", cacheCallbacks)
	}
}

func TestImageAssetSizeWaitersCoverInflightAndCompletedCacheEntries(t *testing.T) {
	asset := &ImageAsset{}
	sizeCalls, loadCalls := 0, 0
	asset.addWaiters(func() { sizeCalls++ }, func() { loadCalls++ })
	asset.addWaiters(func() { sizeCalls++ }, func() { loadCalls++ })
	asset.publishIntrinsicSize(image.Pt(12, 6))
	if sizeCalls != 2 || loadCalls != 0 {
		t.Fatalf("after size publication callbacks = (size %d, load %d), want (2,0)", sizeCalls, loadCalls)
	}

	// A caller joining after the hint reads it immediately and only needs the
	// regular completion notification.
	asset.addWaiters(func() { sizeCalls++ }, func() { loadCalls++ })
	if sizeCalls != 2 {
		t.Fatalf("late size waiter ran unexpectedly: %d calls", sizeCalls)
	}
	asset.finishDecoded([]image.Image{image.NewRGBA(image.Rect(0, 0, 12, 6))}, nil, nil, nil, nil)
	if loadCalls != 3 {
		t.Fatalf("completion callbacks = %d, want 3", loadCalls)
	}

	// A completed cache hit has its size available synchronously and preserves
	// the existing behavior of not replaying the completion callback.
	asset.addWaiters(func() { sizeCalls++ }, func() { loadCalls++ })
	if size, ok := asset.IntrinsicSize(); !ok || size != image.Pt(12, 6) {
		t.Fatalf("cached intrinsic size = (%v, %v), want (12,6), true", size, ok)
	}
	if sizeCalls != 2 || loadCalls != 3 {
		t.Fatalf("completed cache callbacks = (size %d, load %d), want (2,3)", sizeCalls, loadCalls)
	}
}

func TestImageIntrinsicSizeHandlesUnknownAndSVGData(t *testing.T) {
	if size, ok := imageIntrinsicSize([]byte("not an image")); ok || size != (image.Point{}) {
		t.Fatalf("unknown image size = (%v, %v), want zero,false", size, ok)
	}
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="120" height="80"><rect width="120" height="80"/></svg>`)
	if size, ok := imageIntrinsicSize(svg); !ok || size != image.Pt(120, 80) {
		t.Fatalf("SVG size = (%v, %v), want (120,80),true", size, ok)
	}
}

func TestSVGIntrinsicSizeMatchesDecodedBitmapRounding(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="105.3" height="42.7"><rect width="105.3" height="42.7"/></svg>`)
	hint, ok := imageIntrinsicSize(svg)
	if !ok {
		t.Fatal("fractional SVG dimensions were not available as a hint")
	}
	bmp, err := decodeDocumentImage(svg)
	if err != nil {
		t.Fatal(err)
	}
	w, h := bmp.Size()
	if hint != image.Pt(w, h) {
		t.Fatalf("SVG hint = %v, decoded bitmap = (%d,%d); dimensions must match exactly", hint, w, h)
	}
}
