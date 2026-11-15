package markdown

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testImageCacheStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (s *testImageCacheStore) Read(key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.data[key]...), nil
}

func (s *testImageCacheStore) Write(key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = make(map[string][]byte)
	}
	s.data[key] = append([]byte(nil), data...)
	return nil
}

func (s *testImageCacheStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

func TestRemoteImageUsesInjectedCacheStore(t *testing.T) {
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
	decodedImageAssets.delete(imageAssetCacheKey{kind: imageAssetCacheRemote, value: srv.URL})
	t.Cleanup(func() { decodedImageAssets.delete(imageAssetCacheKey{kind: imageAssetCacheRemote, value: srv.URL}) })
	store := &testImageCacheStore{}

	loaded := make(chan struct{}, 1)
	asset := LoadImageAssetWithCacheStore(srv.URL, "", store, func() { loaded <- struct{}{} })
	waitImageLoad(t, loaded)
	if next, changed := asset.Advance(time.Now()); next != 0 || !changed {
		t.Fatalf("static image initial Advance = (%v, %v), want (0, true)", next, changed)
	}
	if bmp := asset.Bitmap(); bmp == nil {
		t.Fatal("downloaded ImageAsset has no bitmap")
	} else if w, h := bmp.Size(); w != 9 || h != 5 {
		t.Fatalf("downloaded bitmap dimensions = %dx%d, want 9x5", w, h)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("initial request count = %d, want 1", got)
	}

	// Simulate a fresh process by clearing decoded state and taking the server
	// offline. The injected store must provide the validated source bytes.
	decodedImageAssets.delete(imageAssetCacheKey{kind: imageAssetCacheRemote, value: srv.URL})
	unavailable.Store(true)
	loadedAgain := make(chan struct{}, 1)
	reloaded := LoadImageAssetWithCacheStore(srv.URL, "", store, func() { loadedAgain <- struct{}{} })
	waitImageLoad(t, loadedAgain)
	if next, changed := reloaded.Advance(time.Now()); next != 0 || !changed || reloaded.Bitmap() == nil {
		t.Fatalf("cached ImageAsset did not initialize: next=%v changed=%v bitmap=%v", next, changed, reloaded.Bitmap())
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("cache hit made %d total requests, want only initial download", got)
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
