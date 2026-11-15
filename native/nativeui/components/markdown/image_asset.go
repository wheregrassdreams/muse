package markdown

import (
	"container/list"
	"image"
	"sync"
	"time"
	"weak"

	"github.com/egoist/mygo/ui"
	"muse/markdown/images"
)

// ImageAsset adapts a feature-owned neutral image Asset to MyGo's Bitmap.
// The image pipeline, source cache, decode budget, and GIF clock live in
// muse/markdown/images; this type only owns the renderer bitmap handle.
type ImageAsset struct {
	core    *images.Asset
	mu      sync.Mutex
	bitmap  *ui.Bitmap
	version uint64
}

const maxImageAssetAdapters = 1024

type imageAssetAdapterRef struct {
	asset   weak.Pointer[images.Asset]
	wrapper weak.Pointer[ImageAsset]
}

var imageAssetAdapters = struct {
	mu      sync.Mutex
	entries map[weak.Pointer[images.Asset]]*list.Element
	lru     list.List
}{entries: make(map[weak.Pointer[images.Asset]]*list.Element)}

func wrapImageAsset(asset *images.Asset) *ImageAsset {
	if asset == nil {
		return nil
	}
	key := weak.Make(asset)
	imageAssetAdapters.mu.Lock()
	if e := imageAssetAdapters.entries[key]; e != nil {
		imageAssetAdapters.lru.MoveToFront(e)
		ref := e.Value.(*imageAssetAdapterRef)
		if wrapper := ref.wrapper.Value(); wrapper != nil && wrapper.core == asset {
			imageAssetAdapters.mu.Unlock()
			return wrapper
		}
		imageAssetAdapters.lru.Remove(e)
		delete(imageAssetAdapters.entries, key)
	}
	wrapper := &ImageAsset{core: asset}
	e := imageAssetAdapters.lru.PushFront(&imageAssetAdapterRef{asset: key, wrapper: weak.Make(wrapper)})
	imageAssetAdapters.entries[key] = e
	if imageAssetAdapters.lru.Len() > maxImageAssetAdapters {
		old := imageAssetAdapters.lru.Back()
		imageAssetAdapters.lru.Remove(old)
		delete(imageAssetAdapters.entries, old.Value.(*imageAssetAdapterRef).asset)
	}
	imageAssetAdapters.mu.Unlock()
	return wrapper
}

// LoadImageAsset returns a stable UI adapter. Source reads and decode are
// asynchronous, with renderer-neutral pixels managed by markdown/images.
func LoadImageAsset(src, baseDir string, onLoad func()) *ImageAsset {
	return LoadImageAssetWithCacheStore(src, baseDir, nil, onLoad)
}

// LoadImageAssetWithCacheStore loads a source using the application's remote
// byte cache store when it is remote.
func LoadImageAssetWithCacheStore(src, baseDir string, store images.CacheStore, onLoad func()) *ImageAsset {
	return LoadImageAssetWithSizeAndCacheStore(src, baseDir, store, nil, onLoad)
}

// LoadImageAssetWithSize calls onSize after intrinsic dimensions are known
// and before pixel decoding finishes.
func LoadImageAssetWithSize(src, baseDir string, onSize, onLoad func()) *ImageAsset {
	return LoadImageAssetWithSizeAndCacheStore(src, baseDir, nil, onSize, onLoad)
}

// LoadImageAssetWithSizeAndCacheStore combines intrinsic-size and completion
// callbacks with an optional application supplied remote cache store.
func LoadImageAssetWithSizeAndCacheStore(src, baseDir string, store images.CacheStore, onSize, onLoad func()) *ImageAsset {
	asset := images.Load(src, images.LoadOptions{Source: images.SourceOptions{BaseDir: baseDir}, Store: store}, onSize, onLoad)
	return wrapImageAsset(asset)
}

// Bitmap returns the adapter bitmap for the current neutral frame, if loaded.
func (a *ImageAsset) Bitmap() *ui.Bitmap {
	a.syncBitmap()
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.bitmap
}

// Loaded reports whether loading and decode completed.
func (a *ImageAsset) Loaded() bool { return a != nil && a.core != nil && a.core.Loaded() }

// IntrinsicSize reports source dimensions once header inspection completes.
func (a *ImageAsset) IntrinsicSize() (image.Point, bool) {
	if a == nil || a.core == nil {
		return image.Point{}, false
	}
	return a.core.IntrinsicSize()
}

// ApproxPixelBytes estimates feature-owned pixels; MyGo's Bitmap shares the
// feature RGBA backing store, so the adapter adds no second pixel plane.
func (a *ImageAsset) ApproxPixelBytes() int64 {
	if a == nil || a.core == nil {
		return 0
	}
	return a.core.ApproxPixelBytes()
}

// Advance moves the neutral asset to the frame appropriate for now and syncs
// the MyGo bitmap on the caller's UI thread.
func (a *ImageAsset) Advance(now time.Time) (time.Duration, bool) {
	if a == nil || a.core == nil {
		return 0, false
	}
	next, changed := a.core.Advance(now)
	a.syncBitmap()
	return next, changed
}

func (a *ImageAsset) syncBitmap() {
	if a == nil || a.core == nil {
		return
	}
	version := a.core.FrameVersion()
	if version == 0 {
		return
	}
	frame := a.core.CurrentFrame()
	if frame == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.version == version {
		return
	}
	if a.bitmap == nil {
		a.bitmap = ui.NewBitmap(frame)
	} else {
		a.bitmap.Update(frame)
	}
	a.version = version
}

func (a *ImageAsset) frameIndex() int {
	if a == nil || a.core == nil {
		return 0
	}
	return a.core.FrameIndex()
}
func (a *ImageAsset) frameCount() int {
	if a == nil || a.core == nil {
		return 0
	}
	return a.core.FrameCount()
}
func (a *ImageAsset) currentFrame() image.Image {
	if a == nil || a.core == nil {
		return nil
	}
	return a.core.CurrentFrame()
}
func (a *ImageAsset) frameDelay(index int) time.Duration {
	if a == nil || a.core == nil {
		return 0
	}
	return a.core.FrameDelay(index)
}
func (a *ImageAsset) gifFrameReady(index int) bool {
	if a == nil || a.core == nil {
		return false
	}
	return a.core.FrameReady(index)
}

// ImageAssetCacheStats reports the single feature-owned decoded LRU.
func ImageAssetCacheStats() (int, int64) { return images.CacheStats() }

// ImageDecodeQueueStats reports the feature-owned bounded decode and frame jobs.
func ImageDecodeQueueStats() (int, int) { return images.QueueStats() }
