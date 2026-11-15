package markdown

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"os"
	"path/filepath"
	"runtime"
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

func TestGIFImageAssetComposesFramesOnDemandAndMatchesReference(t *testing.T) {
	path := filepath.Join("..", "testdata", "vault-fixture", "Attachments", "animation.gif")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	want, delays, err := compositeGIF(decoded)
	if err != nil {
		t.Fatal(err)
	}
	frames, gotDelays, _, animation, err := decodeImageAsset(data)
	if err != nil {
		t.Fatal(err)
	}
	if animation == nil || len(frames) != 1 || len(gotDelays) != len(delays) {
		t.Fatalf("lazy decode retained frames=%d delays=%d animated=%v", len(frames), len(gotDelays), animation != nil)
	}
	asset := &ImageAsset{}
	asset.finishDecoded(frames, gotDelays, nil, animation, nil)
	now := time.Unix(1_800_000_000, 0)
	if next, changed := asset.Advance(now); !changed || next != delays[0] {
		t.Fatalf("initial lazy GIF advance = (%v, %v), want (%v, true)", next, changed, delays[0])
	}
	if !equalImages(asset.frames[0], want[0]) {
		t.Fatal("lazy first GIF frame differs from reference compositor")
	}
	when := now
	for step := 1; step <= len(want); step++ {
		frame := step % len(want)
		waitForGIFFrame(t, asset, frame)
		when = when.Add(delays[(step-1)%len(delays)])
		next, changed := asset.Advance(when)
		if !changed || next != delays[frame] {
			t.Fatalf("lazy GIF frame %d advance = (%v, %v), want (%v, true)", frame, next, changed, delays[frame])
		}
		if !equalImages(asset.frames[0], want[frame]) {
			t.Fatalf("lazy frame %d differs from reference compositor", frame)
		}
		if len(asset.frames) != 1 {
			t.Fatalf("retained %d full frames after advancing; want one", len(asset.frames))
		}
	}
}

func TestGIFImageAssetPixelEstimateIncludesRetainedAnimationData(t *testing.T) {
	palette := colorPalette()
	animation := &gif.GIF{
		Config: image.Config{ColorModel: palette, Width: 4, Height: 3},
		Image: []*image.Paletted{
			palettedPatch(image.Rect(0, 0, 4, 3), palette, 1),
			palettedPatch(image.Rect(1, 0, 3, 2), palette, 2),
		},
	}
	first := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	asset := &ImageAsset{}
	asset.finishDecoded([]image.Image{first}, []time.Duration{20 * time.Millisecond, 30 * time.Millisecond}, nil, animation, nil)

	// Two paletted source frames use 12+4 bytes, while the decoded first frame
	// and initial animation canvas each use 4*3*4 bytes.
	const wantBeforeBitmap = int64(16 + 48 + 48)
	if got := asset.ApproxPixelBytes(); got != wantBeforeBitmap {
		t.Fatalf("GIF pixel estimate before bitmap = %d, want %d", got, wantBeforeBitmap)
	}

	if _, changed := asset.Advance(time.Unix(1_800_000_000, 0)); !changed {
		t.Fatal("initial GIF advance did not create its bitmap")
	}
	const wantWithBitmap = wantBeforeBitmap + 4*3*4
	if got := asset.ApproxPixelBytes(); got != wantWithBitmap {
		t.Fatalf("GIF pixel estimate with bitmap = %d, want %d", got, wantWithBitmap)
	}
}

func TestGIFImageAssetLazyCompositorPreservesDisposal(t *testing.T) {
	palette := colorPalette()
	gifData := &gif.GIF{
		Config: image.Config{ColorModel: palette, Width: 4, Height: 3},
		Image: []*image.Paletted{
			palettedPatch(image.Rect(0, 0, 4, 3), palette, 1),
			palettedPatch(image.Rect(1, 0, 3, 2), palette, 2),
			palettedPatch(image.Rect(2, 1, 4, 3), palette, 3),
			palettedPatch(image.Rect(0, 1, 2, 3), palette, 2),
		},
		Delay:    []int{2, 3, 4, 5},
		Disposal: []byte{gif.DisposalNone, gif.DisposalBackground, gif.DisposalPrevious, gif.DisposalNone},
	}
	want, delays, err := compositeGIF(gifData)
	if err != nil {
		t.Fatal(err)
	}
	first := image.NewNRGBA(image.Rect(0, 0, gifData.Config.Width, gifData.Config.Height))
	draw.Draw(first, gifData.Image[0].Bounds(), gifData.Image[0], gifData.Image[0].Bounds().Min, draw.Over)
	asset := &ImageAsset{}
	asset.finishDecoded([]image.Image{cloneNRGBA(first)}, []time.Duration{20 * time.Millisecond, 30 * time.Millisecond, 40 * time.Millisecond, 50 * time.Millisecond}, nil, gifData, nil)
	if len(delays) != len(gifData.Image) {
		t.Fatalf("reference delays=%d, want %d", len(delays), len(gifData.Image))
	}
	now := time.Unix(1_800_000_000, 0)
	asset.Advance(now)
	when := now
	for step := 1; step <= len(want); step++ {
		frame := step % len(want)
		waitForGIFFrame(t, asset, frame)
		when = when.Add(delays[(step-1)%len(delays)])
		asset.Advance(when)
		if !equalImages(asset.frames[0], want[frame]) {
			t.Fatalf("frame %d with disposal sequence %v differs from reference", frame, gifData.Disposal)
		}
	}
}

func colorPalette() color.Palette {
	return color.Palette{color.Transparent, color.RGBA{R: 255, A: 255}, color.RGBA{G: 255, A: 255}, color.RGBA{B: 255, A: 255}}
}

func palettedPatch(bounds image.Rectangle, palette color.Palette, index uint8) *image.Paletted {
	frame := image.NewPaletted(bounds, palette)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			frame.SetColorIndex(x, y, index)
		}
	}
	return frame
}

func waitForGIFFrame(t *testing.T, asset *ImageAsset, frame int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		asset.mu.Lock()
		ready := asset.animReady != nil && asset.animReadyIndex == frame
		asset.mu.Unlock()
		if ready {
			return
		}
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("GIF frame %d was not prepared", frame)
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
