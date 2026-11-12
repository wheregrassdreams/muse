package mdoc

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"muse/cachepath"
)

func TestRemoteImagePersistsAndReloadsFromDisk(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv(cachepath.RootEnv, cacheRoot)
	var requests atomic.Int32
	var unavailable atomic.Bool
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 9, 5))); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if unavailable.Load() {
			http.Error(w, "offline", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes.Bytes())
	}))
	defer srv.Close()
	remoteImageAssets.Delete(srv.URL)
	remoteImages.Delete(srv.URL)
	t.Cleanup(func() {
		remoteImageAssets.Delete(srv.URL)
		remoteImages.Delete(srv.URL)
	})

	loaded := make(chan struct{}, 1)
	asset := LoadImageAsset(srv.URL, "", func() { loaded <- struct{}{} })
	waitImageLoad(t, loaded)
	if next, changed := asset.Advance(time.Now()); next != 0 || !changed {
		t.Fatalf("static image initial Advance = (%v, %v), want (0, true)", next, changed)
	}
	if bmp := asset.Bitmap(); bmp == nil {
		t.Fatal("downloaded ImageAsset has no bitmap")
	} else if w, h := bmp.Size(); w != 9 || h != 5 {
		t.Fatalf("downloaded bitmap dimensions = %dx%d, want 9x5", w, h)
	}

	cacheFile, err := remoteImageCachePath(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(filepath.Dir(cacheFile)) != "images" || filepath.Dir(filepath.Dir(cacheFile)) != cacheRoot {
		t.Fatalf("remote cache path = %q, want under %q/images", cacheFile, cacheRoot)
	}
	cacheBytes, err := os.ReadFile(cacheFile)
	if err != nil {
		t.Fatalf("remote image cache was not written: %v", err)
	}
	if !bytes.Equal(cacheBytes, pngBytes.Bytes()) {
		t.Fatal("disk cache contents differ from downloaded bytes")
	}
	info, err := os.Stat(cacheFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("cache file permissions = %o, want 600", info.Mode().Perm())
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("initial request count = %d, want 1", got)
	}

	// Simulate a fresh process: discard all in-process entries, then make the
	// endpoint return 404. The on-disk copy must serve both loaders without a
	// second request.
	remoteImageAssets.Delete(srv.URL)
	remoteImages.Delete(srv.URL)
	unavailable.Store(true)

	loadedAgain := make(chan struct{}, 1)
	reloaded := LoadImageAsset(srv.URL, "", func() { loadedAgain <- struct{}{} })
	waitImageLoad(t, loadedAgain)
	if next, changed := reloaded.Advance(time.Now()); next != 0 || !changed || reloaded.Bitmap() == nil {
		t.Fatalf("disk-backed ImageAsset did not initialize: next=%v changed=%v bitmap=%v", next, changed, reloaded.Bitmap())
	}

	remoteImages.Delete(srv.URL)
	legacyDone := make(chan struct{}, 1)
	if bmp := LoadImage(srv.URL, "", func() { legacyDone <- struct{}{} }); bmp != nil {
		t.Fatal("legacy LoadImage should notify asynchronously after process cache reset")
	}
	waitImageLoad(t, legacyDone)
	if bmp := LoadImage(srv.URL, "", nil); bmp == nil {
		t.Fatal("legacy LoadImage did not reload the disk cache")
	} else if w, h := bmp.Size(); w != 9 || h != 5 {
		t.Fatalf("legacy disk-backed image dimensions = %dx%d, want 9x5", w, h)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("disk cache miss made %d total requests, want only the initial download", got)
	}
}

func waitImageLoad(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("image load did not complete")
	}
}
