package images

import (
	"image"
	"image/draw"
	"sync"
	"time"
)

// Asset is a renderer-neutral decoded image handle. It owns decoded pixels,
// intrinsic size and GIF animation state; renderers may read CurrentFrame and
// adapt it to their own bitmap type.
type Asset struct {
	mu                       sync.Mutex
	frames                   []image.Image
	delays                   []time.Duration
	animator                 *GIFAnimator
	current                  image.Image
	frame                    int
	next                     time.Time
	loaded, failed           bool
	err                      error
	version                  uint64
	intrinsic                image.Point
	intrinsicSet             bool
	sizeWaiters, loadWaiters []func()
	cache                    *AssetCache
	cacheKey                 AssetKey
}

// Loaded reports whether source loading and decode finished.
func (a *Asset) Loaded() bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.loaded
}

// Failed reports a completed source or decode failure.
func (a *Asset) Failed() bool {
	if a == nil {
		return true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.failed
}

// Failure returns the source or decode error after loading has failed.
func (a *Asset) Failure() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.err
}

// IntrinsicSize returns dimensions published before pixel decode when known.
func (a *Asset) IntrinsicSize() (image.Point, bool) {
	if a == nil {
		return image.Point{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.intrinsic, a.intrinsicSet
}

// CurrentFrame returns the selected renderer-neutral pixel image.
func (a *Asset) CurrentFrame() image.Image {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.current
}

// FrameVersion changes whenever the selected frame changes.
func (a *Asset) FrameVersion() uint64 {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.version
}

// FrameIndex reports the selected animation frame.
func (a *Asset) FrameIndex() int {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.animator != nil {
		return a.animator.FrameIndex()
	}
	return a.frame
}

// FrameCount reports the available animation frames (or one for a static image).
func (a *Asset) FrameCount() int {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.animator != nil {
		return a.animator.FrameCount()
	}
	if len(a.frames) > 0 || a.current != nil {
		return 1
	}
	return 0
}

// FrameDelay returns the configured duration for a frame.
func (a *Asset) FrameDelay(index int) time.Duration {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.animator != nil {
		return a.animator.FrameDelay(index)
	}
	if index < 0 || index >= len(a.delays) {
		return 0
	}
	return a.delays[index]
}

// FrameReady reports whether a GIF frame has finished composition.
func (a *Asset) FrameReady(index int) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	animator := a.animator
	a.mu.Unlock()
	return animator != nil && animator.Ready(index)
}

// Advance selects the image frame appropriate for now. Frame composition is
// scheduled through the bounded feature decode queue.
func (a *Asset) Advance(now time.Time) (time.Duration, bool) {
	if a == nil {
		return 0, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.loaded || a.failed {
		return 0, false
	}
	if a.animator != nil {
		frame, wait, changed := a.animator.Advance(now)
		if changed {
			a.current = canonicalRGBA(frame)
			a.frame = a.animator.FrameIndex()
			a.version++
		}
		return wait, changed
	}
	if a.current == nil && len(a.frames) > 0 {
		a.current = a.frames[0]
		a.frames = nil
		a.frame = 0
		a.version++
		return 0, true
	}
	return 0, false
}

// ApproxPixelBytes estimates Go pixel buffers retained by this asset. It
// excludes allocator overhead and renderer/GPU resources.
func (a *Asset) ApproxPixelBytes() int64 {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var n int64
	if a.animator != nil {
		n += a.animator.ApproxPixelBytes()
	} else if a.current != nil {
		b := a.current.Bounds()
		n += int64(b.Dx()) * int64(b.Dy()) * 4
	}
	for _, f := range a.frames {
		if f != nil {
			b := f.Bounds()
			n += int64(b.Dx()) * int64(b.Dy()) * 4
		}
	}
	return n
}

func (a *Asset) addWaiters(onSize, onLoad func()) {
	if a == nil || onSize == nil && onLoad == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.loaded {
		return
	}
	if onLoad != nil {
		a.loadWaiters = append(a.loadWaiters, onLoad)
	}
	if !a.intrinsicSet && onSize != nil {
		a.sizeWaiters = append(a.sizeWaiters, onSize)
	}
}

func (a *Asset) publishIntrinsicSize(size image.Point) {
	if size.X <= 0 || size.Y <= 0 {
		return
	}
	a.mu.Lock()
	if a.intrinsicSet || a.loaded {
		a.mu.Unlock()
		return
	}
	a.intrinsic, a.intrinsicSet = size, true
	waiters := a.sizeWaiters
	a.sizeWaiters = nil
	a.mu.Unlock()
	for _, fn := range waiters {
		fn()
	}
}

func (a *Asset) finish(decoded Decoded, err error) {
	a.mu.Lock()
	a.frames, a.delays = decoded.Frames, decoded.Delays
	if decoded.Animation != nil && len(decoded.Frames) > 0 {
		a.animator = NewGIFAnimator(decoded.Animation, decoded.Frames[0], decoded.Delays)
		a.frames = nil
	}
	a.loaded = true
	a.failed = err != nil || len(a.frames) == 0 && a.animator == nil
	a.err = err
	waiters := a.loadWaiters
	a.loadWaiters = nil
	a.sizeWaiters = nil
	cache, key := a.cache, a.cacheKey
	a.cache, a.cacheKey = nil, AssetKey{}
	frames, animation := a.frames, decoded.Animation
	if a.animator != nil {
		frames = []image.Image{a.animator.Current()}
	}
	a.mu.Unlock()
	if cache != nil {
		cache.Finish(key, a, EstimateRetainedBytes(len(key.Value), frames, image.Point{}, animation))
	}
	for _, fn := range waiters {
		fn()
	}
}

func (a *Asset) fail(err error) { a.finish(Decoded{}, err) }

func canonicalRGBA(img image.Image) *image.RGBA {
	if img == nil {
		return nil
	}
	b := img.Bounds()
	if rgba, ok := img.(*image.RGBA); ok && rgba.Stride == 4*b.Dx() && b.Min == (image.Point{}) {
		return rgba
	}
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}
