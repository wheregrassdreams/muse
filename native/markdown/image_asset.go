package markdown

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/egoist/mygo/ui"
	"muse/markdown/images"
)

// ImageAsset is a decoded image that can be advanced by a UI frame loop.
// Loading and file/network IO happen asynchronously; Bitmap and Advance are
// safe to call from the UI thread after LoadImageAsset returns.
type ImageAsset struct {
	mu sync.Mutex

	cacheKey imageAssetCacheKey

	frames            []image.Image
	delays            []time.Duration
	animated          *gif.GIF
	animCanvas        *image.NRGBA
	animRestore       *image.NRGBA
	animDisposal      byte
	animIndex         int
	animReady         *image.NRGBA
	animReadyIndex    int
	animReadyCanvas   *image.NRGBA
	animReadyRestore  *image.NRGBA
	animReadyDisposal byte
	animBusy          bool
	bitmap            *ui.Bitmap
	direct            *ui.Bitmap
	frame             int
	next              time.Time
	loaded            bool
	failed            bool
	intrinsicSize     image.Point
	intrinsicSizeSet  bool
	sizeWaiters       []func()
	waiters           []func()
}

type dataImageSourceKey struct {
	data *byte
	len  int
}

// LoadImageAsset returns a stable image handle. Local files and remote URLs
// are read and decoded in a goroutine; onLoad runs after success or failure.
// Shared sources share one load and one animation clock.
func LoadImageAsset(src, baseDir string, onLoad func()) *ImageAsset {
	return LoadImageAssetWithCacheStore(src, baseDir, nil, onLoad)
}

// LoadImageAssetWithCacheStore is LoadImageAsset with an application supplied
// store for remote source bytes. A nil store disables the disk cache.
func LoadImageAssetWithCacheStore(src, baseDir string, store images.CacheStore, onLoad func()) *ImageAsset {
	return LoadImageAssetWithSizeAndCacheStore(src, baseDir, store, nil, onLoad)
}

// LoadImageAssetWithSize returns a stable image handle and calls onSize after
// its intrinsic dimensions are known but before pixel decoding begins. onSize
// runs on the loader worker and must only publish thread-safe state or request
// invalidation; onLoad retains the usual completion notification semantics.
func LoadImageAssetWithSize(src, baseDir string, onSize, onLoad func()) *ImageAsset {
	return LoadImageAssetWithSizeAndCacheStore(src, baseDir, nil, onSize, onLoad)
}

// LoadImageAssetWithSizeAndCacheStore is LoadImageAssetWithSize with an
// application supplied store for remote source bytes. A nil store disables
// the disk cache.
func LoadImageAssetWithSizeAndCacheStore(src, baseDir string, store images.CacheStore, onSize, onLoad func()) *ImageAsset {
	src = strings.TrimSpace(src)
	if src == "" {
		return &ImageAsset{loaded: true, failed: true}
	}
	if isRemoteImage(src) {
		return loadRemoteImageAsset(src, store, onSize, onLoad)
	}
	if strings.HasPrefix(src, "data:") {
		return loadDataImageAssetWithSize(src, onSize, onLoad)
	}
	path := resolveImagePath(src, baseDir)
	if path == "" {
		return &ImageAsset{loaded: true, failed: true}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return &ImageAsset{loaded: true, failed: true}
	}
	st, err := os.Stat(abs)
	if err != nil || !st.Mode().IsRegular() {
		return &ImageAsset{loaded: true, failed: true}
	}
	key := imageAssetCacheKey{kind: imageAssetCacheFile, value: abs}
	if entry, ok := decodedImageAssets.load(key); ok {
		if entry.mod.Equal(st.ModTime()) && entry.size == st.Size() {
			entry.asset.addWaiters(onSize, onLoad)
			return entry.asset
		}
	}
	asset := &ImageAsset{sizeWaiters: nonNilWaiters(onSize), waiters: nonNilWaiters(onLoad), cacheKey: key}
	entry := &cachedImageAsset{key: key, asset: asset, mod: st.ModTime(), size: st.Size(), weight: int64(len(abs)), pinned: true, cacheable: true}
	actual, loaded := decodedImageAssets.storeIfAbsent(entry)
	if loaded {
		if actual.mod.Equal(st.ModTime()) && actual.size == st.Size() {
			actual.asset.addWaiters(onSize, onLoad)
			return actual.asset
		}
		decodedImageAssets.replace(entry)
	}
	imageDecodes.submit(func() {
		data, readErr := os.ReadFile(abs)
		if readErr != nil {
			asset.finish(nil, nil, nil, readErr)
			return
		}
		frames, delays, direct, animation, decodeErr := decodeImageAssetWithSize(data, asset.publishIntrinsicSize)
		// The decoded asset owns its pixel data; do not keep the compressed
		// source buffer live through the worker's next wait on its queue.
		data = nil
		asset.finishDecoded(frames, delays, direct, animation, decodeErr)
	})
	return asset
}

func nonNilWaiters(fn func()) []func() {
	if fn == nil {
		return nil
	}
	return []func(){fn}
}

func loadRemoteImageAsset(src string, store images.CacheStore, onSize, onLoad func()) *ImageAsset {
	key := imageAssetCacheKey{kind: imageAssetCacheRemote, value: src}
	if old, ok := decodedImageAssets.load(key); ok {
		old.asset.addWaiters(onSize, onLoad)
		return old.asset
	}
	asset := &ImageAsset{sizeWaiters: nonNilWaiters(onSize), waiters: nonNilWaiters(onLoad), cacheKey: key}
	entry := &cachedImageAsset{key: key, asset: asset, weight: int64(len(src)), pinned: true, cacheable: true}
	actual, loaded := decodedImageAssets.storeIfAbsent(entry)
	if loaded {
		actual.asset.addWaiters(onSize, onLoad)
		return actual.asset
	}
	go func() {
		remoteImageLoadSlots <- struct{}{}
		defer func() { <-remoteImageLoadSlots }()
		data, err := fetchRemoteImageData(src, store)
		if err != nil {
			asset.finish(nil, nil, nil, err)
			return
		}
		imageDecodes.submit(func() {
			frames, delays, direct, animation, err := decodeImageAssetWithSize(data, asset.publishIntrinsicSize)
			data = nil
			asset.finishDecoded(frames, delays, direct, animation, err)
		})
	}()
	return asset
}

func (a *ImageAsset) addWaiters(onSize, onLoad func()) {
	if onSize == nil && onLoad == nil {
		return
	}
	a.mu.Lock()
	if !a.loaded {
		if onLoad != nil {
			a.waiters = append(a.waiters, onLoad)
		}
		if !a.intrinsicSizeSet && onSize != nil {
			a.sizeWaiters = append(a.sizeWaiters, onSize)
		}
	}
	a.mu.Unlock()
}

func (a *ImageAsset) publishIntrinsicSize(size image.Point) {
	if size.X <= 0 || size.Y <= 0 {
		return
	}
	a.mu.Lock()
	if a.intrinsicSizeSet || a.loaded {
		a.mu.Unlock()
		return
	}
	a.intrinsicSize, a.intrinsicSizeSet = size, true
	waiters := a.sizeWaiters
	a.sizeWaiters = nil
	a.mu.Unlock()
	for _, fn := range waiters {
		fn()
	}
}

// IntrinsicSize returns the source dimensions known to the loader before
// pixel decoding. SVG dimensions match the bounded raster output size.
func (a *ImageAsset) IntrinsicSize() (image.Point, bool) {
	if a == nil {
		return image.Point{}, false
	}
	a.mu.Lock()
	size, ok := a.intrinsicSize, a.intrinsicSizeSet
	a.mu.Unlock()
	return size, ok
}

func (a *ImageAsset) finish(frames []image.Image, delays []time.Duration, direct *ui.Bitmap, err error) {
	a.finishDecoded(frames, delays, direct, nil, err)
}

func (a *ImageAsset) finishDecoded(frames []image.Image, delays []time.Duration, direct *ui.Bitmap, animation *gif.GIF, err error) {
	a.mu.Lock()
	a.frames, a.delays = frames, delays
	a.direct = direct
	a.animated = animation
	if animation != nil && len(frames) > 0 {
		a.animCanvas = cloneNRGBA(frames[0].(*image.NRGBA))
		a.animIndex = 0
		if len(animation.Disposal) > 0 {
			a.animDisposal = animation.Disposal[0]
		}
	}
	a.loaded, a.failed = true, err != nil || len(frames) == 0 && direct == nil
	waiters := a.waiters
	a.waiters = nil
	// If header inspection failed, the completion callback is the only
	// remaining notification needed; discard the size-only registrations.
	a.sizeWaiters = nil
	a.mu.Unlock()
	if a.cacheKey.kind != 0 {
		key := a.cacheKey
		// The cache entry, not the live asset, owns its key after loading. In
		// particular this lets a later LRU eviction release a small data URI.
		a.cacheKey = imageAssetCacheKey{}
		decodedImageAssets.finish(key, a, imageAssetCacheWeight(key, frames, direct, animation))
	}
	for _, fn := range waiters {
		fn()
	}
}

// Bitmap returns the current frame bitmap, or nil until the first Advance
// creates it on the UI thread.
func (a *ImageAsset) Bitmap() *ui.Bitmap {
	a.mu.Lock()
	bmp := a.bitmap
	if bmp == nil {
		bmp = a.direct
	}
	a.mu.Unlock()
	return bmp
}

// Loaded reports whether the asynchronous read/decode has completed. It lets
// renderers distinguish a brief loading placeholder from a missing image.
func (a *ImageAsset) Loaded() bool {
	a.mu.Lock()
	loaded := a.loaded
	a.mu.Unlock()
	return loaded
}

// ApproxPixelBytes estimates pixel storage currently owned by this asset.
// It excludes allocator overhead and GPU copies, and is used only by the
// opt-in memory diagnostics.
func (a *ImageAsset) ApproxPixelBytes() int64 {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var bytes int64
	// GIF assets keep the indexed source frames as well as their current
	// compositing canvases. Counting only a.frames misses most of the retained
	// pixel storage while an animation is loaded or being prepared.
	if a.animated != nil {
		for _, frame := range a.animated.Image {
			if frame != nil {
				b := frame.Bounds()
				bytes += int64(b.Dx()) * int64(b.Dy())
			}
		}
	}
	for _, frame := range a.frames {
		if frame != nil {
			b := frame.Bounds()
			bytes += int64(b.Dx()) * int64(b.Dy()) * 4
		}
	}
	for _, canvas := range []*image.NRGBA{a.animCanvas, a.animRestore, a.animReady, a.animReadyCanvas, a.animReadyRestore} {
		if canvas != nil {
			b := canvas.Bounds()
			bytes += int64(b.Dx()) * int64(b.Dy()) * 4
		}
	}
	for _, bitmap := range []*ui.Bitmap{a.bitmap, a.direct} {
		if bitmap != nil {
			w, h := bitmap.Size()
			bytes += int64(w) * int64(h) * 4
		}
	}
	return bytes
}

// Advance moves the asset to the frame appropriate for now. It returns the
// time until the next frame and whether the bitmap changed. Call it from the
// UI thread and schedule a repaint with Context.After(next) while next > 0.
func (a *ImageAsset) Advance(now time.Time) (next time.Duration, changed bool) {
	a.mu.Lock()
	startPrepare := false
	defer func() {
		if startPrepare {
			imageDecodes.submit(a.prepareNextGIFFrame)
		}
		a.mu.Unlock()
	}()
	if !a.loaded || a.failed || len(a.frames) == 0 && a.direct == nil {
		return 0, false
	}
	if a.bitmap == nil {
		a.frame = 0
		if a.direct != nil {
			a.bitmap = a.direct
			a.direct = nil
			return 0, true
		}
		a.bitmap = ui.NewBitmap(a.frames[0])
		if len(a.frames) < 2 && a.animated == nil {
			// Static images no longer need their decoded source once the owned
			// bitmap has copied/converted its pixels.
			a.frames = nil
			return 0, true
		}
		a.next = now.Add(a.delays[0])
		startPrepare = a.animated != nil
		return a.delays[0], true
	}
	if len(a.frames) < 2 && a.animated == nil {
		return 0, false
	}
	if !now.Before(a.next) {
		if a.animated != nil {
			if a.animReady == nil {
				startPrepare = true
				return 16 * time.Millisecond, false
			}
			a.frames[0] = a.animReady
			a.frame = a.animReadyIndex
			a.animCanvas, a.animRestore = a.animReadyCanvas, a.animReadyRestore
			a.animDisposal = a.animReadyDisposal
			a.animIndex = a.animReadyIndex
			a.animReady, a.animReadyCanvas, a.animReadyRestore = nil, nil, nil
			a.bitmap.Update(a.frames[0])
			a.next = now.Add(a.delays[a.frame])
			startPrepare = true
			return max(a.next.Sub(now), time.Millisecond), true
		}
		for !now.Before(a.next) {
			a.frame = (a.frame + 1) % len(a.frames)
			a.next = a.next.Add(a.delays[a.frame])
		}
		a.bitmap.Update(a.frames[a.frame])
		changed = true
	}
	return max(a.next.Sub(now), time.Millisecond), changed
}

func (a *ImageAsset) prepareNextGIFFrame() {
	a.mu.Lock()
	if a.animated == nil || a.animBusy || a.animReady != nil || len(a.animated.Image) < 2 {
		a.mu.Unlock()
		return
	}
	a.animBusy = true
	g, idx := a.animated, (a.animIndex+1)%len(a.animated.Image)
	canvas := cloneNRGBA(a.animCanvas)
	var restore *image.NRGBA
	if a.animRestore != nil {
		restore = cloneNRGBA(a.animRestore)
	}
	prevDisposal := a.animDisposal
	a.mu.Unlock()
	if prevDisposal == gif.DisposalBackground {
		prevRect := g.Image[a.animIndex].Bounds()
		draw.Draw(canvas, prevRect, image.NewUniform(color.Transparent), image.Point{}, draw.Src)
	} else if prevDisposal == gif.DisposalPrevious && restore != nil {
		canvas = restore
	}
	disposal := byte(gif.DisposalNone)
	if idx < len(g.Disposal) {
		disposal = g.Disposal[idx]
	}
	var nextRestore *image.NRGBA
	if disposal == gif.DisposalPrevious {
		nextRestore = cloneNRGBA(canvas)
	}
	frame := g.Image[idx]
	draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
	composed := cloneNRGBA(canvas)
	a.mu.Lock()
	a.animBusy = false
	if a.animated == g && (a.animIndex+1 == idx || a.animIndex == len(g.Image)-1 && idx == 0) {
		a.animReady, a.animReadyIndex = composed, idx
		a.animReadyCanvas, a.animReadyRestore, a.animReadyDisposal = canvas, nextRestore, disposal
	}
	a.mu.Unlock()
}

func (a *ImageAsset) frameIndex() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.frame
}

func (a *ImageAsset) frameCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.animated != nil {
		return len(a.animated.Image)
	}
	return len(a.frames)
}

func loadDataImageAsset(src string, onLoad func()) *ImageAsset {
	return loadDataImageAssetWithSize(src, nil, onLoad)
}

func loadDataImageAssetWithSize(src string, onSize, onLoad func()) *ImageAsset {
	sourceKey := dataImageSourceKey{data: unsafe.StringData(src), len: len(src)}
	if old, ok := decodedImageAssets.loadSource(sourceKey); ok {
		old.asset.addWaiters(onSize, onLoad)
		return old.asset
	}
	key := imageAssetCacheKey{kind: imageAssetCacheData, value: src}
	cacheable := len(src) <= maxCachedDataImageSourceBytes
	if cacheable {
		// A substring can keep a much larger Markdown source buffer alive.
		// Cache only a bounded standalone key for small data URLs.
		key.value = strings.Clone(src)
	}
	if old, ok := decodedImageAssets.load(key); ok {
		old.asset.addWaiters(onSize, onLoad)
		return old.asset
	}
	asset := &ImageAsset{sizeWaiters: nonNilWaiters(onSize), waiters: nonNilWaiters(onLoad), cacheKey: key}
	entry := &cachedImageAsset{key: key, asset: asset, weight: int64(len(key.value)), pinned: true, cacheable: cacheable, sourceKey: sourceKey}
	actual, loaded := decodedImageAssets.storeIfAbsent(entry)
	if loaded {
		actual.asset.addWaiters(onSize, onLoad)
		return actual.asset
	}
	imageDecodes.submit(func() {
		data, err := dataImageBytes(src)
		if err != nil {
			asset.finish(nil, nil, nil, err)
			return
		}
		frames, delays, direct, animation, err := decodeImageAssetWithSize(data, asset.publishIntrinsicSize)
		// None of the decoded outputs aliases this encoded input.
		data = nil
		asset.finishDecoded(frames, delays, direct, animation, err)
	})
	return asset
}

// decodeImageAsset prepares only the first composited GIF frame. Later frames
// are composed one at a time by ImageAsset, keeping memory proportional to the
// canvas instead of canvas pixels multiplied by the frame count.
func decodeImageAsset(data []byte) ([]image.Image, []time.Duration, *ui.Bitmap, *gif.GIF, error) {
	return decodeImageAssetWithSize(data, nil)
}

func decodeImageAssetWithSize(data []byte, onSize func(image.Point)) ([]image.Image, []time.Duration, *ui.Bitmap, *gif.GIF, error) {
	if len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a") {
		budget, err := inspectGIFFrameBudget(data)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("inspect GIF: %w", err)
		}
		if budget.firstPixels > maxSingleImageDecodePixels {
			return nil, nil, nil, nil, fmt.Errorf("GIF first frame exceeds the %d-pixel decode limit", maxSingleImageDecodePixels)
		}
		if onSize != nil {
			onSize(image.Pt(budget.config.Width, budget.config.Height))
		}
		canvasPixels := int64(budget.config.Width) * int64(budget.config.Height)
		keepAnimation := budget.frames > 1 && budget.totalPixels <= maxAnimatedGIFFramePixels && canvasPixels <= maxInlineImagePixels
		if !keepAnimation {
			frame, err := gif.Decode(bytes.NewReader(data))
			if err != nil {
				return nil, nil, nil, nil, fmt.Errorf("decode first GIF frame: %w", err)
			}
			canvas := image.Rect(0, 0, budget.config.Width, budget.config.Height)
			imageOnCanvas := gifCanvasView(frame, canvas)
			return []image.Image{limitInlineImagePixels(imageOnCanvas)}, nil, nil, nil, nil
		}
		decoded, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("decode GIF: %w", err)
		}
		if len(decoded.Image) == 0 {
			return nil, nil, nil, nil, fmt.Errorf("GIF has no image frames")
		}
		bounds := image.Rect(0, 0, decoded.Config.Width, decoded.Config.Height)
		if bounds.Empty() {
			return nil, nil, nil, nil, fmt.Errorf("GIF has empty dimensions")
		}
		canvas := image.NewNRGBA(bounds)
		draw.Draw(canvas, decoded.Image[0].Bounds(), decoded.Image[0], decoded.Image[0].Bounds().Min, draw.Over)
		delays := make([]time.Duration, len(decoded.Image))
		for i := range delays {
			delay := 0
			if i < len(decoded.Delay) {
				delay = decoded.Delay[i]
			}
			delays[i] = max(time.Duration(delay)*10*time.Millisecond, 20*time.Millisecond)
		}
		return []image.Image{cloneNRGBA(canvas)}, delays, nil, decoded, nil
	}
	if onSize != nil {
		if size, ok := imageIntrinsicSize(data); ok {
			onSize(size)
		}
	}
	if err := checkRasterDecodePixelBudget(data); err != nil {
		return nil, nil, nil, nil, err
	}
	if img, ok := decodeLargePNGAtDisplaySize(data); ok {
		return []image.Image{img}, nil, nil, nil, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err == nil {
		return []image.Image{limitInlineImagePixels(img)}, nil, nil, nil, nil
	}
	if bmp, svgErr := decodeDocumentImage(data); svgErr == nil {
		return nil, nil, bmp, nil, nil
	}
	return nil, nil, nil, nil, fmt.Errorf("decode image: %w", err)
}

func decodeImageFrames(data []byte) ([]image.Image, []time.Duration, *ui.Bitmap, error) {
	if decoded, err := gif.DecodeAll(bytes.NewReader(data)); err == nil && len(decoded.Image) > 0 {
		frames, delays, err := compositeGIF(decoded)
		return frames, delays, nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err == nil {
		return []image.Image{img}, nil, nil, nil
	}
	if bmp, svgErr := decodeDocumentImage(data); svgErr == nil {
		return nil, nil, bmp, nil
	}
	return nil, nil, nil, fmt.Errorf("decode image: %w", err)
}

func compositeGIF(g *gif.GIF) ([]image.Image, []time.Duration, error) {
	bounds := image.Rect(0, 0, g.Config.Width, g.Config.Height)
	if bounds.Empty() {
		return nil, nil, fmt.Errorf("GIF has empty dimensions")
	}
	canvas := image.NewNRGBA(bounds)
	frames := make([]image.Image, 0, len(g.Image))
	delays := make([]time.Duration, 0, len(g.Image))
	var previous image.Rectangle
	var previousDisposal byte
	var restore *image.NRGBA
	for i, frame := range g.Image {
		if i > 0 {
			switch previousDisposal {
			case gif.DisposalBackground:
				draw.Draw(canvas, previous, image.NewUniform(color.Transparent), image.Point{}, draw.Src)
			case gif.DisposalPrevious:
				if restore != nil {
					canvas = cloneNRGBA(restore)
				}
			}
		}
		disposal := byte(gif.DisposalNone)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}
		if disposal == gif.DisposalPrevious {
			restore = cloneNRGBA(canvas)
		} else {
			restore = nil
		}
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		frames = append(frames, cloneNRGBA(canvas))
		delay := 0
		if i < len(g.Delay) {
			delay = g.Delay[i]
		}
		delays = append(delays, max(time.Duration(delay)*10*time.Millisecond, 20*time.Millisecond))
		previous, previousDisposal = frame.Bounds(), disposal
	}
	return frames, delays, nil
}

func cloneNRGBA(src *image.NRGBA) *image.NRGBA {
	dst := image.NewNRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst
}

func dataImageBytes(src string) ([]byte, error) {
	comma := strings.IndexByte(src, ',')
	if comma < 0 || !strings.Contains(src[:comma], ";base64") {
		return nil, fmt.Errorf("image data URL is not base64")
	}
	encoded := strings.TrimSpace(src[comma+1:])
	if dataImageHeaderMismatch(src[:comma], encoded) {
		return nil, fmt.Errorf("image data URL does not match its declared format")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// dataImageHeaderMismatch cheaply rejects a known image MIME type when the
// first decoded bytes cannot possibly match that format. Large pasted data
// URLs can otherwise allocate a full decoded copy before the image decoder
// rejects an obviously invalid payload.
func dataImageHeaderMismatch(header, encoded string) bool {
	if colon := strings.IndexByte(header, ':'); colon >= 0 {
		header = header[colon+1:]
	}
	semi := strings.IndexByte(header, ';')
	const encodedPrefixBytes = 16 // decodes at most twelve bytes
	if semi < 0 || len(encoded) < encodedPrefixBytes {
		return false
	}
	mimeType := header[:semi]
	var want []byte
	switch {
	case strings.EqualFold(mimeType, "image/png"):
		want = []byte("\x89PNG\r\n\x1a\n")
	case strings.EqualFold(mimeType, "image/jpeg"), strings.EqualFold(mimeType, "image/jpg"):
		want = []byte{0xff, 0xd8, 0xff}
	case strings.EqualFold(mimeType, "image/gif"):
		if !strings.HasPrefix(encoded, "R0lGOD") {
			return true
		}
		return false
	case strings.EqualFold(mimeType, "image/bmp"):
		want = []byte{'B', 'M'}
	case strings.EqualFold(mimeType, "image/webp"):
		var prefix [12]byte
		n, _ := base64.StdEncoding.Decode(prefix[:], []byte(encoded[:encodedPrefixBytes]))
		return n < 12 || string(prefix[:4]) != "RIFF" || string(prefix[8:12]) != "WEBP"
	default:
		return false
	}
	var prefix [12]byte
	n, _ := base64.StdEncoding.Decode(prefix[:], []byte(encoded[:encodedPrefixBytes]))
	return n < len(want) || !bytes.Equal(prefix[:len(want)], want)
}
