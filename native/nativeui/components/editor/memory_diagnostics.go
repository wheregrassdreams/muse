package editor

// MemorySnapshot is a point-in-time estimate of image memory retained by
// the editor. Pixel bytes exclude allocator overhead and GPU copies.
type MemorySnapshot struct {
	SourceBytes          int
	Leaves               int
	VirtualLayout        bool
	ImageCandidateLeaves int
	ImageAssetViews      int
	ImageAssets          int
	LoadedImageAssets    int
	ImagePixelBytes      int64
}

// CollectMemorySnapshot combines editor layout counts with UI-owned decoded
// image handle accounting. The component never exposes mutable bitmaps.
func CollectMemorySnapshot(e *Editor) MemorySnapshot {
	if e == nil {
		return MemorySnapshot{}
	}
	layout := e.LayoutMemorySnapshot()
	out := MemorySnapshot{
		SourceBytes:          layout.SourceBytes,
		Leaves:               layout.Leaves,
		VirtualLayout:        layout.VirtualLayout,
		ImageCandidateLeaves: layout.ImageCandidateLeaves,
		ImageAssetViews:      layout.ImageAssetViews,
	}
	for _, asset := range e.ImageMemoryHandles() {
		out.ImageAssets++
		if asset.Loaded() {
			out.LoadedImageAssets++
		}
		out.ImagePixelBytes += asset.ApproxPixelBytes()
	}
	return out
}

// MemorySnapshot reports retained editor and image memory through the UI
// component contract, without exposing mutable bitmap handles.
func (e *Editor) MemorySnapshot() MemorySnapshot {
	if e == nil {
		return MemorySnapshot{}
	}
	return CollectMemorySnapshot(e)
}
