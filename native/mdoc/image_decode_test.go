package mdoc

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"muse/cachepath"
)

func TestDocumentImageSVGFileAndDataURI(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="120" height="80"><rect width="120" height="80" fill="red"/></svg>`)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample image.svg"), svg, 0600); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{"sample%20image.svg", "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(svg)} {
		bmp := LoadImage(src, dir, nil)
		if bmp == nil {
			t.Fatalf("image %q did not load", src)
		}
		w, h := bmp.Size()
		if w <= 0 || h <= 0 || w*2 != h*3 {
			t.Fatalf("bad aspect ratio: %dx%d", w, h)
		}
	}
}

func TestDocumentImageRejectsMalformedSVG(t *testing.T) {
	if _, err := decodeDocumentImage([]byte(`<svg width="0" height="0"></svg>`)); err == nil {
		t.Fatal("accepted invalid dimensions")
	}
	if _, err := decodeDocumentImage([]byte(`not an image`)); err == nil {
		t.Fatal("accepted text")
	}
}

func TestRemoteImageNotifiesBothNoteAndPreview(t *testing.T) {
	t.Setenv(cachepath.RootEnv, t.TempDir())
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="10"><rect width="20" height="10" fill="blue"/></svg>`))
	}))
	defer srv.Close()
	note, preview := make(chan struct{}, 1), make(chan struct{}, 1)
	LoadImage(srv.URL, "", func() { note <- struct{}{} })
	LoadImage(srv.URL, "", func() { preview <- struct{}{} })
	close(release)
	for _, ch := range []chan struct{}{note, preview} {
		select {
		case <-ch:
		case <-time.After(3 * time.Second):
			t.Fatal("image subscriber was not notified")
		}
	}
	if LoadImage(srv.URL, "", nil) == nil {
		t.Fatal("remote SVG was not cached")
	}
}

func TestRemoteImageCachedFailureCompletesNewViewer(t *testing.T) {
	t.Setenv(cachepath.RootEnv, t.TempDir())
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	for i := 0; i < 2; i++ {
		done := make(chan struct{}, 1)
		if LoadImage(srv.URL, "", func() { done <- struct{}{} }) != nil {
			t.Fatal("404 loaded an image")
		}
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("failed remote image never completed")
		}
	}
}
