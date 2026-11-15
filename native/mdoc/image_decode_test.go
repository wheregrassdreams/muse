package mdoc

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
	"testing"
	"time"
	"unsafe"

	"muse/cachepath"
)

func TestMissingImageMessageDoesNotRenderDataURIPayload(t *testing.T) {
	const payloadSize = 32 << 20
	src := "data:image/png;base64," + strings.Repeat("A", payloadSize)
	got := MissingImageMessage(src)
	want := MissingImageMessage("")
	if got != want {
		t.Fatalf("data URI error message should use generic fallback; got %q want %q", got, want)
	}
	if strings.Contains(got, "base64") || strings.Contains(got, "AAAA") {
		t.Fatalf("data URI payload leaked into error message: %q", got)
	}
}

func TestDataImageBytesRejectsMismatchedLargePngBeforeDecode(t *testing.T) {
	src := "data:image/png;base64," + strings.Repeat("A", 1<<20)
	if data, err := dataImageBytes(src); err == nil || data != nil {
		t.Fatalf("dataImageBytes returned %d bytes, %v for a non-PNG payload", len(data), err)
	}
}

func TestDataImageAssetCacheDeduplicatesAcrossSourceStorage(t *testing.T) {
	firstSource := "data:image/png;base64," + strings.Repeat("A", 128)
	ready := make(chan struct{}, 1)
	first := LoadImageAsset(firstSource, "", func() { ready <- struct{}{} })
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("first data image asset did not finish")
	}
	clonedSource := strings.Clone(firstSource)
	if unsafe.StringData(firstSource) == unsafe.StringData(clonedSource) {
		t.Fatal("strings.Clone reused the source backing storage")
	}
	if got := LoadImageAsset(clonedSource, "", nil); got != first {
		t.Fatal("content-equal data URLs did not reuse the cached asset")
	}
	cloneKey := dataImageSourceKey{data: unsafe.StringData(clonedSource), len: len(clonedSource)}
	if _, ok := decodedImageAssets.loadSource(cloneKey); ok {
		t.Fatal("content-equal copy was retained by the source-identity cache")
	}
	if got := LoadImageAsset(firstSource, "", nil); got != first {
		t.Fatal("same-source data URL did not reuse the source-identity cache")
	}
}

func TestLargeDataImageAssetDropsCacheKeyAfterDecode(t *testing.T) {
	src := "data:image/png;base64," + strings.Repeat("A", maxCachedDataImageSourceBytes)
	ready := make(chan struct{}, 1)
	asset := LoadImageAsset(src, "", func() { ready <- struct{}{} })
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("large data image asset did not finish")
	}
	if asset.cacheKey.kind != 0 {
		t.Fatal("large data URI remains reachable through its ImageAsset cache key")
	}
	key := imageAssetCacheKey{kind: imageAssetCacheData, value: src}
	if _, ok := decodedImageAssets.load(key); ok {
		t.Fatal("large data URI remained in the shared cache after decode")
	}
}

func TestDataImageBytesAcceptsMatchingPngSignature(t *testing.T) {
	want := []byte("\x89PNG\r\n\x1a\nsmall payload")
	src := "data:image/png;base64," + base64.StdEncoding.EncodeToString(want)
	got, err := dataImageBytes(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("dataImageBytes = %q, want %q", got, want)
	}
}

func TestValidateImageDataChecksHeaderWithoutRasterizing(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 320, 240))); err != nil {
		t.Fatal(err)
	}
	if err := validateImageData(encoded.Bytes()); err != nil {
		t.Fatalf("validateImageData rejected a valid PNG: %v", err)
	}
	if err := validateImageData([]byte("not an image")); err == nil {
		t.Fatal("validateImageData accepted invalid image data")
	}
}

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

func TestSVGDocumentPrefixSkipsWhitespaceAndFoldsCase(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  bool
	}{
		{"<svg xmlns=\"http://www.w3.org/2000/svg\">", true},
		{" \n\t<?XML version=\"1.0\"?>", true},
		{"\u00a0<!-- generated -->", true},
		{"not an image", false},
		{"", false},
	} {
		if got := svgDocumentPrefix([]byte(tc.input)); got != tc.want {
			t.Errorf("svgDocumentPrefix(%q) = %t, want %t", tc.input, got, tc.want)
		}
	}
	if allocs := testing.AllocsPerRun(100, func() {
		_ = svgDocumentPrefix([]byte("not an image"))
	}); allocs != 0 {
		t.Fatalf("svgDocumentPrefix allocates %g times per call", allocs)
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
