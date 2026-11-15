package images

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func pngFixture(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func waitLoaded(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("image load timed out")
	}
}

func TestLoaderSharesFileAssetAndInvalidatesChangedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(path, pngFixture(t, 40, 20), 0o600); err != nil {
		t.Fatal(err)
	}
	l := NewLoader(2 << 20)
	loaded := make(chan struct{}, 2)
	sizes := make(chan image.Point, 2)
	returned := make(chan struct{})
	var a *Asset
	options := LoadOptions{Source: SourceOptions{LocalFiles: testLocalFiles{}}}
	a = l.Load(path, options, func() {
		<-returned
		p, ok := a.IntrinsicSize()
		if !ok {
			sizes <- image.Point{}
		} else {
			sizes <- p
		}
	}, func() { loaded <- struct{}{} })
	close(returned)
	waitLoaded(t, loaded)
	if got := <-sizes; got != image.Pt(40, 20) {
		t.Fatalf("intrinsic size %v, want (40,20)", got)
	}
	if !a.Loaded() || a.Failed() {
		t.Fatal("valid PNG did not load")
	}
	if next, changed := a.Advance(time.Now()); next != 0 || !changed {
		t.Fatalf("static first frame=(%v,%v), want (0,true)", next, changed)
	}
	if _, ok := a.CurrentFrame().(*image.RGBA); !ok {
		t.Fatalf("static frame type %T, want *image.RGBA for zero-copy UI adaptation", a.CurrentFrame())
	}
	if got := l.Load(path, options, nil, nil); got != a {
		t.Fatal("same file source did not reuse its Asset")
	}
	if err := os.WriteFile(path, pngFixture(t, 60, 30), 0o600); err != nil {
		t.Fatal(err)
	}
	b := l.Load(path, options, nil, func() { loaded <- struct{}{} })
	if b == a {
		t.Fatal("changed file reused stale Asset")
	}
	waitLoaded(t, loaded)
	if p, ok := b.IntrinsicSize(); !ok || p != image.Pt(60, 30) {
		t.Fatalf("updated file size=(%v,%v), want (60,30)", p, ok)
	}
}

func TestLoaderDataURLContentCacheAndLargeKeyRelease(t *testing.T) {
	l := NewLoader(2 << 20)
	data := pngFixture(t, 8, 5)
	src := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	loaded := make(chan struct{}, 2)
	first := l.Load(src, LoadOptions{}, nil, func() { loaded <- struct{}{} })
	waitLoaded(t, loaded)
	clone := strings.Clone(src)
	second := l.Load(clone, LoadOptions{}, nil, nil)
	if second != first {
		t.Fatal("content-equal data URL did not reuse decoded Asset")
	}
	oversized := "data:image/png;base64," + strings.Repeat("A", MaxCachedDataSourceBytes)
	failed := l.Load(oversized, LoadOptions{}, nil, func() { loaded <- struct{}{} })
	waitLoaded(t, loaded)
	if !failed.Failed() {
		t.Fatal("invalid large data URL did not report failure")
	}
	if entries, weight := l.CacheStats(); entries != 1 || weight <= 0 {
		t.Fatalf("large noncacheable data URL remained in LRU: entries=%d weight=%d", entries, weight)
	}
}

type memoryImageStore struct{ values map[string][]byte }

func (s *memoryImageStore) Read(k string) ([]byte, error) {
	v, ok := s.values[k]
	if !ok {
		return nil, os.ErrNotExist
	}
	return v, nil
}
func (s *memoryImageStore) Write(k string, b []byte) error {
	if s.values == nil {
		s.values = map[string][]byte{}
	}
	s.values[k] = append([]byte(nil), b...)
	return nil
}
func (s *memoryImageStore) Delete(k string) error { delete(s.values, k); return nil }

func TestLoaderCoalescesRemoteFetchAndCallbacks(t *testing.T) {
	var requests atomic.Int32
	payload := pngFixture(t, 10, 6)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); <-release; _, _ = w.Write(payload) }))
	defer srv.Close()
	l := NewLoader(2 << 20)
	store := &memoryImageStore{values: map[string][]byte{}}
	completed := make(chan struct{}, 2)
	a := l.Load(srv.URL, LoadOptions{Store: store}, nil, func() { completed <- struct{}{} })
	b := l.Load(srv.URL, LoadOptions{Store: store}, nil, func() { completed <- struct{}{} })
	if a != b {
		t.Fatal("in-flight remote request did not share feature Asset")
	}
	close(release)
	waitLoaded(t, completed)
	waitLoaded(t, completed)
	if got := requests.Load(); got != 1 {
		t.Fatalf("remote requests=%d, want 1", got)
	}
	if entries := len(store.values); entries != 1 {
		t.Fatalf("remote disk-cache entries=%d, want 1", entries)
	}
}
