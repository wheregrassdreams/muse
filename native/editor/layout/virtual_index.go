// Package layout owns reusable, toolkit-independent document geometry indexes.
package layout

import "sort"

// VirtualIndex stores base document y positions plus Fenwick-tree height
// corrections for blocks measured after the initial virtual layout.
type VirtualIndex struct {
	baseY       []float32
	heightDelta []float32
	baseHeight  float32
	ready       bool
}

// Reset discards the current geometry snapshot while retaining bounded slice
// capacity for the next document layout.
func (v *VirtualIndex) Reset() {
	v.baseY = v.baseY[:0]
	v.heightDelta = v.heightDelta[:0]
	v.baseHeight = 0
	v.ready = false
}

// Release drops all backing arrays when the owning document lifetime ends.
func (v *VirtualIndex) Release() { *v = VirtualIndex{} }

// Capture saves the initial y position of each visible block and the document
// height before subsequent lazy measurement corrections.
func (v *VirtualIndex) Capture(y []float32, documentHeight float32) {
	v.CaptureCount(len(y), documentHeight)
	copy(v.baseY, y)
}

// CaptureCount starts a geometry snapshot for n blocks. SetBaseY fills each
// block's original position without requiring the editor to allocate a copy.
func (v *VirtualIndex) CaptureCount(n int, documentHeight float32) {
	if n < 0 {
		n = 0
	}
	if cap(v.baseY) < n {
		v.baseY = make([]float32, n)
	} else {
		v.baseY = v.baseY[:n]
	}
	if cap(v.heightDelta) < n+1 {
		v.heightDelta = make([]float32, n+1)
	} else {
		v.heightDelta = v.heightDelta[:n+1]
		clear(v.heightDelta)
	}
	v.baseHeight = documentHeight
	v.ready = true
}

// SetBaseY records a block's original y position during CaptureCount.
func (v *VirtualIndex) SetBaseY(i int, y float32) { v.baseY[i] = y }

// Ready reports whether Capture has installed an index for n visible blocks.
func (v *VirtualIndex) Ready(n int) bool {
	return v.ready && len(v.baseY) == n && len(v.heightDelta) == n+1
}

// Len is the number of indexed visible blocks.
func (v *VirtualIndex) Len() int { return len(v.baseY) }

// BaseY is the block's original y position before lazy height corrections.
func (v *VirtualIndex) BaseY(i int) float32 {
	if i < 0 || i >= len(v.baseY) {
		return 0
	}
	return v.baseY[i]
}

// Y returns a block's current y position after corrections to prior blocks.
func (v *VirtualIndex) Y(i int) float32 {
	if i < 0 || i >= len(v.baseY) {
		return 0
	}
	return v.baseY[i] + v.Prefix(i)
}

// Prefix returns the accumulated height correction for the first n blocks.
func (v *VirtualIndex) Prefix(n int) float32 {
	n = max(0, min(n, len(v.heightDelta)-1))
	var sum float32
	for at := n; at > 0; at -= at & -at {
		sum += v.heightDelta[at]
	}
	return sum
}

// AddHeight applies a measured block height difference to all following
// positions in logarithmic time.
func (v *VirtualIndex) AddHeight(i int, delta float32) {
	if i < 0 || i >= len(v.baseY) {
		return
	}
	for at := i + 1; at < len(v.heightDelta); at += at & -at {
		v.heightDelta[at] += delta
	}
}

// DocumentHeight applies all measured corrections to the captured height.
func (v *VirtualIndex) DocumentHeight() float32 { return v.baseHeight + v.Prefix(len(v.baseY)) }

// BaseDocumentHeight returns the captured height before lazy corrections.
func (v *VirtualIndex) BaseDocumentHeight() float32 { return v.baseHeight }

// VisibleRange finds the half-open range of blocks intersecting [top,bottom],
// with overscan applied by the caller through adjusted bounds. Heights are
// supplied by the owning editor because block content remains editor-owned.
func (v *VirtualIndex) VisibleRange(top, bottom float32, heightAt func(int) float32) (start, end int) {
	if heightAt == nil {
		return 0, 0
	}
	start = sort.Search(len(v.baseY), func(i int) bool {
		return v.Y(i)+max(heightAt(i), 0) >= top
	})
	end = sort.Search(len(v.baseY), func(i int) bool { return v.Y(i) > bottom })
	return start, end
}
