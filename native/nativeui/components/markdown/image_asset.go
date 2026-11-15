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

	frames           []image.Image
	delays           []time.Duration
	animator         *images.GIFAnimator
	bitmap           *ui.Bitmap
	direct           *ui.Bitmap
	frame            int
	next             time.Time
	loaded           bool
	failed           bool
	intrinsicSize    image.Point
	intrinsicSizeSet bool
	sizeWaiters      []func()
	waiters          []func()
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
	loadImageSource(abs, nil, asset)
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
	loadImageSource(src, store, asset)
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
	if animation != nil && len(frames) > 0 {
		first, ok := frames[0].(*image.NRGBA)
		if !ok {
			first = image.NewNRGBA(frames[0].Bounds())
			draw.Draw(first, first.Bounds(), frames[0], frames[0].Bounds().Min, draw.Src)
		}
		a.animator = images.NewGIFAnimator(animation, first, delays)
		a.frames = nil
	}
	a.loaded, a.failed = true, err != nil || len(frames) == 0 && direct == nil && a.animator == nil
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
	if a.animator != nil {
		bytes += a.animator.ApproxPixelBytes()
	}
	for _, frame := range a.frames {
		if frame != nil {
			b := frame.Bounds()
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
	defer a.mu.Unlock()
	if !a.loaded || a.failed || len(a.frames) == 0 && a.direct == nil && a.animator == nil {
		return 0, false
	}
	if a.animator != nil {
		frame, wait, changed := a.animator.Advance(now)
		if changed && frame != nil {
			if a.bitmap == nil {
				a.bitmap = ui.NewBitmap(frame)
			} else {
				a.bitmap.Update(frame)
			}
			a.frame = a.animator.FrameIndex()
		}
		return wait, changed
	}
	if a.bitmap == nil {
		a.frame = 0
		if a.direct != nil {
			a.bitmap = a.direct
			a.direct = nil
			return 0, true
		}
		a.bitmap = ui.NewBitmap(a.frames[0])
		if len(a.frames) < 2 {
			// Static images no longer need their decoded source once the owned
			// bitmap has copied/converted its pixels.
			a.frames = nil
			return 0, true
		}
		a.next = now.Add(a.delays[0])
		return a.delays[0], true
	}
	if len(a.frames) < 2 {
		return 0, false
	}
	if !now.Before(a.next) {
		for !now.Before(a.next) {
			a.frame = (a.frame + 1) % len(a.frames)
			a.next = a.next.Add(a.delays[a.frame])
		}
		a.bitmap.Update(a.frames[a.frame])
		changed = true
	}
	return max(a.next.Sub(now), time.Millisecond), changed
}

func (a *ImageAsset) frameIndex() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.animator != nil {
		return a.animator.FrameIndex()
	}
	return a.frame
}

func (a *ImageAsset) frameCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.animator != nil {
		return a.animator.FrameCount()
	}
	return len(a.frames)
}

func (a *ImageAsset) currentFrame() image.Image {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.animator != nil {
		return a.animator.Current()
	}
	if len(a.frames) > 0 && a.frame >= 0 && a.frame < len(a.frames) {
		return a.frames[a.frame]
	}
	return nil
}

func (a *ImageAsset) frameDelay(index int) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	if index >= 0 && index < len(a.delays) {
		return a.delays[index]
	}
	return 0
}

func (a *ImageAsset) gifFrameReady(index int) bool {
	a.mu.Lock()
	animator := a.animator
	a.mu.Unlock()
	return animator != nil && animator.Ready(index)
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
	loadImageSource(src, nil, asset)
	return asset
}

func loadImageSource(src string, store images.CacheStore, asset *ImageAsset) {
	images.LoadSourceBytes(src, images.SourceOptions{}, store, func(data []byte, err error) {
		if err != nil {
			asset.finish(nil, nil, nil, err)
			return
		}
		imageDecodes.submit(func() {
			frames, delays, direct, animation, decodeErr := decodeImageAssetWithSize(data, asset.publishIntrinsicSize)
			data = nil
			asset.finishDecoded(frames, delays, direct, animation, decodeErr)
		})
	})
}

// decodeImageAsset prepares only the first composited GIF frame. Later frames
// are composed one at a time by ImageAsset, keeping memory proportional to the
// canvas instead of canvas pixels multiplied by the frame count.
func decodeImageAsset(data []byte) ([]image.Image, []time.Duration, *ui.Bitmap, *gif.GIF, error) {
	return decodeImageAssetWithSize(data, nil)
}

func decodeImageAssetWithSize(data []byte, onSize func(image.Point)) ([]image.Image, []time.Duration, *ui.Bitmap, *gif.GIF, error) {
	decoded, err := images.Decode(data, onSize)
	if err == nil {
		return decoded.Frames, decoded.Delays, nil, decoded.Animation, nil
	}
	// Keep MyGo-specific bitmap formats as a rendering adapter fallback. The
	// feature decoder has already applied its header and pixel budget checks.
	if bmp, fallbackErr := decodeDocumentImage(data); fallbackErr == nil {
		return nil, nil, bmp, nil, nil
	}
	return nil, nil, nil, nil, err
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
	return images.ValidateDataURL(src)
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
