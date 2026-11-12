package mdoc

import (
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestGIFImageAssetComposesAndAdvancesFrames(t *testing.T) {
	path := filepath.Join("..", "testdata", "vault-fixture", "Attachments", "animation.gif")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, delays, direct, err := decodeImageFrames(data)
	if err != nil {
		t.Fatal(err)
	}
	if direct != nil || len(frames) < 2 || len(frames) != len(delays) {
		t.Fatalf("GIF decode produced direct=%v frames=%d delays=%d; want at least two timed frames", direct != nil, len(frames), len(delays))
	}
	for i, delay := range delays {
		if delay <= 0 {
			t.Fatalf("frame %d delay = %v, want positive", i, delay)
		}
	}
	if equalImages(frames[0], frames[1]) {
		t.Fatal("first two decoded GIF frames are identical")
	}

	asset := &ImageAsset{}
	asset.finish(frames, delays, nil, nil)
	start := time.Unix(1_800_000_000, 0)
	wait, changed := asset.Advance(start)
	if !changed || asset.frameIndex() != 0 || asset.Bitmap() == nil {
		t.Fatalf("initial frame: changed=%v index=%d bitmap=%v", changed, asset.frameIndex(), asset.Bitmap())
	}
	if wait != delays[0] {
		t.Fatalf("initial frame delay = %v, want %v", wait, delays[0])
	}
	wait, changed = asset.Advance(start.Add(delays[0]))
	if !changed || asset.frameIndex() != 1 || wait <= 0 {
		t.Fatalf("next frame: changed=%v index=%d next=%v", changed, asset.frameIndex(), wait)
	}
	wait, changed = asset.Advance(start.Add(delays[0] + delays[1]))
	if !changed || asset.frameIndex() != 2 || wait <= 0 {
		t.Fatalf("third frame: changed=%v index=%d next=%v", changed, asset.frameIndex(), wait)
	}
}

func TestGIFDocumentPreviewSchedulesFrameRefresh(t *testing.T) {
	base := filepath.Join("..", "testdata", "vault-fixture", "Attachments")
	path, err := filepath.Abs(filepath.Join(base, "animation.gif"))
	if err != nil {
		t.Fatal(err)
	}
	loaded := make(chan struct{}, 1)
	asset := LoadImageAsset(path, "", func() { loaded <- struct{}{} })
	waitImageLoad(t, loaded)
	if asset.frameCount() < 2 {
		t.Fatalf("fixture GIF has %d frame(s), want multiple", asset.frameCount())
	}
	doc := Parse("![animated fixture](animation.gif)\n")
	if len(doc.Blocks) != 1 || doc.Blocks[0].Kind != Image {
		t.Fatalf("fixture Markdown parsed blocks = %#v, want standalone image", doc.Blocks)
	}
	tester := ui.NewTester(func(c *ui.Context) {
		View(c, doc, Options{BaseDir: pathDir(path)})
	}, 360, 260)
	first := cloneRGBA(tester.Image())
	if asset.frameIndex() != 0 {
		t.Fatalf("preview initial frame index = %d, want 0", asset.frameIndex())
	}
	firstDelay := asset.delays[0]
	tester.Advance(firstDelay + time.Millisecond)
	if asset.frameIndex() != 1 {
		t.Fatalf("preview frame index after scheduled timer = %d, want 1", asset.frameIndex())
	}
	if equalImages(first, tester.Image()) {
		t.Fatal("document preview pixels did not change after timed GIF frame refresh")
	}
}

func pathDir(path string) string { return filepath.Dir(path) }

func cloneRGBA(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst
}

func equalImages(a, b image.Image) bool {
	if a == nil || b == nil || a.Bounds() != b.Bounds() {
		return false
	}
	bounds := a.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if a.At(x, y) != b.At(x, y) {
				return false
			}
		}
	}
	return true
}
