package editor

import editormodel "muse/editor"

// MemoryDiagnostics is a point-in-time estimate of image memory retained by
// the editor. Pixel bytes exclude allocator overhead and GPU copies.
type MemoryDiagnostics struct {
	SourceBytes          int
	Leaves               int
	VirtualLayout        bool
	ImageCandidateLeaves int
	ImageAssetViews      int
	ImageAssets          int
	LoadedImageAssets    int
	ImagePixelBytes      int64
}

// MemorySnapshot combines editor layout counts with UI-owned decoded image
// handle accounting. The component never exposes the mutable bitmaps themselves.
func MemorySnapshot(e *editormodel.Editor) MemoryDiagnostics {
	if e == nil {
		return MemoryDiagnostics{}
	}
	layout := e.LayoutMemorySnapshot()
	out := MemoryDiagnostics{
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
