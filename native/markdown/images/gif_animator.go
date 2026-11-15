package images

import (
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"sync"
	"time"
)

// GIFAnimator owns bounded compositing state for one GIF. It retains indexed
// source frames and at most the current/next composited canvas set.
type GIFAnimator struct {
	mu                        sync.Mutex
	g                         *gif.GIF
	delays                    []time.Duration
	current                   *image.NRGBA
	canvas, restore           *image.NRGBA
	disposal                  byte
	index                     int
	next                      time.Time
	ready                     *image.NRGBA
	readyCanvas, readyRestore *image.NRGBA
	readyDisposal             byte
	readyIndex                int
	busy                      bool
}

// NewGIFAnimator takes ownership of the decoded animation and initial frame.
func NewGIFAnimator(g *gif.GIF, initial *image.NRGBA, delays []time.Duration) *GIFAnimator {
	a := &GIFAnimator{g: g, current: initial, canvas: cloneNRGBA(initial), delays: append([]time.Duration(nil), delays...), readyIndex: -1}
	if g != nil && len(g.Disposal) > 0 {
		a.disposal = g.Disposal[0]
	}
	return a
}

// Advance selects the frame appropriate for now. Composition is queued on the
// feature's bounded decode workers and advances lazily one frame at a time.
func (a *GIFAnimator) Advance(now time.Time) (image.Image, time.Duration, bool) {
	a.mu.Lock()
	start := false
	defer func() {
		if start {
			SubmitDecode(a.prepareNext)
		}
		a.mu.Unlock()
	}()
	if a.g == nil || len(a.g.Image) < 2 || len(a.delays) == 0 {
		return a.current, 0, false
	}
	if a.next.IsZero() {
		a.next = now.Add(a.delays[0])
		start = true
		return a.current, a.delays[0], true
	}
	if now.Before(a.next) {
		return a.current, max(a.next.Sub(now), time.Millisecond), false
	}
	if a.ready == nil {
		start = true
		return a.current, 16 * time.Millisecond, false
	}
	a.current = a.ready
	a.index = a.readyIndex
	a.canvas, a.restore = a.readyCanvas, a.readyRestore
	a.disposal = a.readyDisposal
	a.ready, a.readyCanvas, a.readyRestore = nil, nil, nil
	a.readyIndex = -1
	a.next = now.Add(a.delays[a.index])
	start = true
	return a.current, max(a.next.Sub(now), time.Millisecond), true
}

// FrameIndex reports the most recently selected source frame.
func (a *GIFAnimator) FrameIndex() int { a.mu.Lock(); defer a.mu.Unlock(); return a.index }

// FrameCount reports the number of animation frames.
func (a *GIFAnimator) FrameCount() int {
	if a == nil || a.g == nil {
		return 0
	}
	return len(a.g.Image)
}

// Current returns the current composited frame.
func (a *GIFAnimator) Current() image.Image { a.mu.Lock(); defer a.mu.Unlock(); return a.current }

// Ready reports whether the requested frame is already composed.
func (a *GIFAnimator) Ready(index int) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ready != nil && a.readyIndex == index
}

// ApproxPixelBytes estimates retained source and composition pixel data.
func (a *GIFAnimator) ApproxPixelBytes() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	var n int64
	if a.g != nil {
		for _, f := range a.g.Image {
			if f != nil {
				b := f.Bounds()
				n += int64(b.Dx()) * int64(b.Dy())
			}
		}
	}
	for _, img := range []*image.NRGBA{a.current, a.canvas, a.restore, a.ready, a.readyCanvas, a.readyRestore} {
		if img != nil {
			b := img.Bounds()
			n += int64(b.Dx()) * int64(b.Dy()) * 4
		}
	}
	return n
}
func (a *GIFAnimator) prepareNext() {
	a.mu.Lock()
	if a.g == nil || a.busy || a.ready != nil || len(a.g.Image) < 2 {
		a.mu.Unlock()
		return
	}
	a.busy = true
	g := a.g
	idx := (a.index + 1) % len(g.Image)
	canvas := cloneNRGBA(a.canvas)
	var restore *image.NRGBA
	if a.restore != nil {
		restore = cloneNRGBA(a.restore)
	}
	previousDisposal := a.disposal
	currentIndex := a.index
	a.mu.Unlock()
	if previousDisposal == gif.DisposalBackground {
		rect := g.Image[currentIndex].Bounds()
		draw.Draw(canvas, rect, image.NewUniform(color.Transparent), image.Point{}, draw.Src)
	} else if previousDisposal == gif.DisposalPrevious && restore != nil {
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
	a.busy = false
	if a.g == g && (a.index+1 == idx || a.index == len(g.Image)-1 && idx == 0) {
		a.ready, a.readyIndex = composed, idx
		a.readyCanvas, a.readyRestore, a.readyDisposal = canvas, nextRestore, disposal
	}
	a.mu.Unlock()
}
