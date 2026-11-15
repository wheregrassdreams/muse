package markdown

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func benchmarkPNG(b *testing.B, width, height int) []byte {
	b.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		b.Fatal(err)
	}
	return out.Bytes()
}

func benchmarkAnimatedGIF(b *testing.B, width, height, frames int) []byte {
	b.Helper()
	palette := color.Palette{color.Transparent, color.RGBA{R: 220, A: 255}, color.RGBA{G: 200, A: 255}}
	g := &gif.GIF{Config: image.Config{ColorModel: palette, Width: width, Height: height}}
	for i := 0; i < frames; i++ {
		img := image.NewPaletted(image.Rect(0, 0, width, height), palette)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				img.SetColorIndex(x, y, uint8((x+y+i)%len(palette)))
			}
		}
		g.Image = append(g.Image, img)
		g.Delay = append(g.Delay, 5)
	}
	var out bytes.Buffer
	if err := gif.EncodeAll(&out, g); err != nil {
		b.Fatal(err)
	}
	return out.Bytes()
}

func BenchmarkDecodeLargePNG(b *testing.B) {
	data := benchmarkPNG(b, 2048, 2048)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		frames, _, _, err := decodeImageFrames(data)
		if err != nil || len(frames) != 1 {
			b.Fatalf("decode frames=%d err=%v", len(frames), err)
		}
	}
}

func BenchmarkDecodeManyPNGAssets(b *testing.B) {
	const assets = 64
	data := benchmarkPNG(b, 512, 512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for n := 0; n < assets; n++ {
			if _, _, _, err := decodeImageFrames(data); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.ReportMetric(float64(assets), "assets/op")
}

func BenchmarkLoadManyPNGAssets(b *testing.B) {
	const assets = 64
	data := benchmarkPNG(b, 512, 512)
	dir := b.TempDir()
	paths := make([]string, assets)
	for i := range paths {
		paths[i] = filepath.Join(dir, fmt.Sprintf("image-%03d.png", i))
		if err := os.WriteFile(paths[i], data, 0o600); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	runtime.GC()
	var heapBefore runtime.MemStats
	runtime.ReadMemStats(&heapBefore)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		for _, path := range paths {
			decodedImageAssets.delete(imageAssetCacheKey{kind: imageAssetCacheFile, value: path})
			wg.Add(1)
			LoadImageAsset(path, "", wg.Done)
		}
		wg.Wait()
	}
	b.StopTimer()
	runtime.GC()
	var heapAfter runtime.MemStats
	runtime.ReadMemStats(&heapAfter)
	b.ReportMetric(float64(assets), "assets/op")
	if heapAfter.HeapAlloc >= heapBefore.HeapAlloc {
		b.ReportMetric(float64(heapAfter.HeapAlloc-heapBefore.HeapAlloc), "retained-heap-delta-B")
	}
	if _, weight := decodedImageAssets.stats(); weight > 0 {
		b.ReportMetric(float64(weight), "decoded-cache-B")
	}
}

func BenchmarkLoadDataImageAssetCacheHit(b *testing.B) {
	payloadBytes := 32 << 20
	if raw := os.Getenv("MUSE_STRESS_BASE64_BYTES"); raw != "" {
		var err error
		payloadBytes, err = strconv.Atoi(raw)
		if err != nil || payloadBytes <= 0 {
			b.Fatalf("MUSE_STRESS_BASE64_BYTES must be a positive byte count, got %q", raw)
		}
	}
	path := filepath.Join(b.TempDir(), "large-data-url.md")
	file, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	if _, err := file.WriteString("data:image/png;base64,"); err != nil {
		file.Close()
		b.Fatal(err)
	}
	chunk := strings.Repeat("A", 1<<20)
	for written := 0; written < payloadBytes; written += len(chunk) {
		n := min(len(chunk), payloadBytes-written)
		if _, err := file.WriteString(chunk[:n]); err != nil {
			file.Close()
			b.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		b.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	src := string(data)
	ready := make(chan struct{}, 1)
	asset := LoadImageAsset(src, "", func() { ready <- struct{}{} })
	select {
	case <-ready:
	case <-time.After(30 * time.Second):
		b.Fatal("initial data image asset did not finish")
	}
	if !asset.Loaded() {
		b.Fatal("initial data image asset is not loaded")
	}
	b.Run("asset-cache-hit", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(src)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if got := LoadImageAsset(src, "", nil); got != asset {
				b.Fatal("data image cache did not return the original asset")
			}
		}
	})
}

func BenchmarkDecodeAnimatedGIFCanvasCopies(b *testing.B) {
	data := benchmarkAnimatedGIF(b, 640, 640, 40)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		frames, _, _, err := decodeImageFrames(data)
		if err != nil || len(frames) != 40 {
			b.Fatalf("decode frames=%d err=%v", len(frames), err)
		}
	}
	b.ReportMetric(float64(40), "frames/op")
	b.ReportMetric(float64(640*640*4*40), "canvas-bytes/op")
}

func BenchmarkDecodeAnimatedGIFFirstFrameLazy(b *testing.B) {
	const frames = 16 // Stay below the decoder's retained animated-frame budget.
	data := benchmarkAnimatedGIF(b, 640, 640, frames)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		frames, _, _, animation, err := decodeImageAsset(data)
		if err != nil || animation == nil || len(frames) != 1 {
			b.Fatalf("lazy decode frames=%d animation=%v err=%v", len(frames), animation != nil, err)
		}
	}
	b.ReportMetric(float64(frames), "frames/op")
	b.ReportMetric(float64(640*640*4), "first-frame-bytes/op")
}

func BenchmarkDecodeAnimatedGIFAllFramesLazy(b *testing.B) {
	const framesPerGIF = 16 // Stay below the decoder's retained animated-frame budget.
	data := benchmarkAnimatedGIF(b, 640, 640, framesPerGIF)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		frames, delays, _, animation, err := decodeImageAsset(data)
		if err != nil || animation == nil || len(frames) != 1 || len(delays) != framesPerGIF {
			b.Fatalf("lazy decode frames=%d animation=%v delays=%d err=%v", len(frames), animation != nil, len(delays), err)
		}
		asset := &ImageAsset{}
		asset.finishDecoded(frames, delays, nil, animation, nil)
		now := time.Now()
		asset.animator.Advance(now)
		for frame := 1; frame < len(animation.Image); frame++ {
			waitForGIFFrameBenchmark(b, asset, frame)
			asset.animator.Advance(now.Add(time.Duration(frame) * 100 * time.Millisecond))
		}
	}
	b.ReportMetric(float64(framesPerGIF), "frames/op")
	b.ReportMetric(float64(640*640*4), "max-composited-canvas-bytes/op")
}

func BenchmarkGIFMemoryProfileLazy(b *testing.B) {
	const width, height, frames = 640, 640, 16
	data := benchmarkAnimatedGIF(b, width, height, frames)
	runtime.GC()
	var eagerBase runtime.MemStats
	runtime.ReadMemStats(&eagerBase)
	decoded, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		b.Fatal(err)
	}
	want, _, err := compositeGIF(decoded)
	if err != nil || len(want) != frames {
		b.Fatalf("reference compositor frames=%d err=%v", len(want), err)
	}
	var eager runtime.MemStats
	runtime.ReadMemStats(&eager)
	decoded = nil
	want = nil
	runtime.GC()
	var lazyBase runtime.MemStats
	runtime.ReadMemStats(&lazyBase)
	assetFrames, delays, _, animation, err := decodeImageAsset(data)
	if err != nil {
		b.Fatal(err)
	}
	asset := &ImageAsset{}
	asset.finishDecoded(assetFrames, delays, nil, animation, nil)
	now := time.Now()
	asset.animator.Advance(now)
	var peak uint64
	for step := 1; step < frames; step++ {
		waitForGIFFrameBenchmark(b, asset, step)
		asset.animator.Advance(now.Add(time.Duration(step) * 100 * time.Millisecond))
		var sample runtime.MemStats
		runtime.ReadMemStats(&sample)
		if sample.HeapAlloc > peak {
			peak = sample.HeapAlloc
		}
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(asset)
	b.ReportMetric(float64(eager.HeapAlloc-eagerBase.HeapAlloc), "eager-reference-heap-bytes")
	b.ReportMetric(float64(peak-lazyBase.HeapAlloc), "lazy-peak-heap-delta-bytes")
	b.ReportMetric(float64(after.HeapAlloc-lazyBase.HeapAlloc), "lazy-retained-heap-delta-bytes")
	b.ReportMetric(float64(width*height*4*frames), "eager-frame-pixels-bytes")
	b.ReportMetric(float64(width*height*4*4), "lazy-four-canvas-budget-bytes")
}

func waitForGIFFrameBenchmark(b *testing.B, asset *ImageAsset, frame int) {
	b.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if asset.gifFrameReady(frame) {
			return
		}
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	b.Fatalf("GIF frame %d was not prepared", frame)
}
