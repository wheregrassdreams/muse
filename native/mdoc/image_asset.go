package mdoc

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

	"github.com/egoist/mygo/ui"
)

// ImageAsset is a decoded image that can be advanced by a UI frame loop.
// Loading and file/network IO happen asynchronously; Bitmap and Advance are
// safe to call from the UI thread after LoadImageAsset returns.
type ImageAsset struct {
	mu sync.Mutex

	frames  []image.Image
	delays  []time.Duration
	bitmap  *ui.Bitmap
	direct  *ui.Bitmap
	frame   int
	next    time.Time
	loaded  bool
	failed  bool
	waiters []func()
}

type imageAssetEntry struct {
	asset *ImageAsset
	mod   time.Time
	size  int64
}

var imageAssets sync.Map
var remoteImageAssets sync.Map
var dataImageAssets sync.Map

// LoadImageAsset returns a stable image handle. Local files and remote URLs
// are read and decoded in a goroutine; onLoad runs after success or failure.
// Shared sources share one load and one animation clock.
func LoadImageAsset(src, baseDir string, onLoad func()) *ImageAsset {
	src = strings.TrimSpace(src)
	if src == "" {
		return &ImageAsset{loaded: true, failed: true}
	}
	if isRemoteImage(src) {
		return loadRemoteImageAsset(src, onLoad)
	}
	if strings.HasPrefix(src, "data:") {
		return loadDataImageAsset(src, onLoad)
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
	if old, ok := imageAssets.Load(abs); ok {
		entry := old.(imageAssetEntry)
		if entry.mod.Equal(st.ModTime()) && entry.size == st.Size() {
			entry.asset.addWaiter(onLoad)
			return entry.asset
		}
	}
	asset := &ImageAsset{waiters: nonNilWaiters(onLoad)}
	entry := imageAssetEntry{asset: asset, mod: st.ModTime(), size: st.Size()}
	actual, loaded := imageAssets.LoadOrStore(abs, entry)
	if loaded {
		old := actual.(imageAssetEntry)
		if old.mod.Equal(st.ModTime()) && old.size == st.Size() {
			old.asset.addWaiter(onLoad)
			return old.asset
		}
		imageAssets.Store(abs, entry)
	}
	go func() {
		data, readErr := os.ReadFile(abs)
		if readErr != nil {
			asset.finish(nil, nil, nil, readErr)
			return
		}
		frames, delays, direct, decodeErr := decodeImageFrames(data)
		asset.finish(frames, delays, direct, decodeErr)
	}()
	return asset
}

func nonNilWaiters(fn func()) []func() {
	if fn == nil {
		return nil
	}
	return []func(){fn}
}

func loadRemoteImageAsset(src string, onLoad func()) *ImageAsset {
	if old, ok := remoteImageAssets.Load(src); ok {
		asset := old.(*ImageAsset)
		asset.addWaiter(onLoad)
		return asset
	}
	asset := &ImageAsset{waiters: nonNilWaiters(onLoad)}
	actual, loaded := remoteImageAssets.LoadOrStore(src, asset)
	if loaded {
		current := actual.(*ImageAsset)
		current.addWaiter(onLoad)
		return current
	}
	go func() {
		data, err := fetchRemoteImageData(src)
		if err != nil {
			asset.finish(nil, nil, nil, err)
			return
		}
		frames, delays, direct, err := decodeImageFrames(data)
		asset.finish(frames, delays, direct, err)
	}()
	return asset
}

func (a *ImageAsset) addWaiter(fn func()) {
	if fn == nil {
		return
	}
	a.mu.Lock()
	if a.loaded {
		a.mu.Unlock()
		return
	}
	a.waiters = append(a.waiters, fn)
	a.mu.Unlock()
}

func (a *ImageAsset) finish(frames []image.Image, delays []time.Duration, direct *ui.Bitmap, err error) {
	a.mu.Lock()
	a.frames, a.delays = frames, delays
	a.direct = direct
	a.loaded, a.failed = true, err != nil || len(frames) == 0 && direct == nil
	waiters := a.waiters
	a.waiters = nil
	a.mu.Unlock()
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

// Advance moves the asset to the frame appropriate for now. It returns the
// time until the next frame and whether the bitmap changed. Call it from the
// UI thread and schedule a repaint with Context.After(next) while next > 0.
func (a *ImageAsset) Advance(now time.Time) (next time.Duration, changed bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.loaded || a.failed || len(a.frames) == 0 {
		return 0, false
	}
	if a.bitmap == nil {
		a.frame = 0
		a.bitmap = ui.NewBitmap(a.frames[0])
		if len(a.frames) < 2 {
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
	return a.frame
}

func (a *ImageAsset) frameCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.frames)
}

func loadDataImageAsset(src string, onLoad func()) *ImageAsset {
	if old, ok := dataImageAssets.Load(src); ok {
		asset := old.(*ImageAsset)
		asset.addWaiter(onLoad)
		return asset
	}
	asset := &ImageAsset{waiters: nonNilWaiters(onLoad)}
	actual, loaded := dataImageAssets.LoadOrStore(src, asset)
	if loaded {
		current := actual.(*ImageAsset)
		current.addWaiter(onLoad)
		return current
	}
	go func() {
		data, err := dataImageBytes(src)
		if err != nil {
			asset.finish(nil, nil, nil, err)
			return
		}
		frames, delays, direct, err := decodeImageFrames(data)
		asset.finish(frames, delays, direct, err)
	}()
	return asset
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
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(src[comma+1:]))
	if err != nil {
		return nil, err
	}
	return data, nil
}
