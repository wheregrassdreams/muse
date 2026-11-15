package editor

import "muse/markdown"

// MemoryDiagnostics is a point-in-time estimate of image memory still
// reachable from an editor. Pixel bytes exclude allocator overhead and GPU
// copies; the snapshot is intended for opt-in diagnostics only.
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

// MemoryDiagnostics reports image assets retained by the document's current
// leaves and document-local data-image map, including assets already evicted
// from the process-wide LRU.
func (e *Editor) MemoryDiagnostics() MemoryDiagnostics {
	if e == nil {
		return MemoryDiagnostics{}
	}
	stats := MemoryDiagnostics{
		SourceBytes:   len(e.src),
		Leaves:        len(e.leaves),
		VirtualLayout: e.virtualLayout(),
	}
	assets := make(map[*markdown.ImageAsset]struct{})
	addAsset := func(asset *markdown.ImageAsset) {
		if asset != nil {
			assets[asset] = struct{}{}
		}
	}
	addView := func(view *leafView) {
		if view == nil {
			return
		}
		if view.hasImageAsset() {
			stats.ImageAssetViews++
		}
		if view.block != nil {
			addAsset(view.block.asset)
		}
		for _, piece := range view.disp.pieces {
			if piece.obj != nil {
				addAsset(piece.obj.asset)
			}
		}
	}
	for _, leaf := range e.leaves {
		if leafMayContainImage(e.src, leaf) {
			stats.ImageCandidateLeaves++
		}
		addView(leaf.view)
	}
	for _, view := range e.views {
		addView(view)
	}
	for _, asset := range e.dataImageAssets {
		addAsset(asset)
	}
	stats.ImageAssets = len(assets)
	for asset := range assets {
		if asset.Loaded() {
			stats.LoadedImageAssets++
		}
		stats.ImagePixelBytes += asset.ApproxPixelBytes()
	}
	return stats
}
