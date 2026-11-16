package editor

import "muse/markdown/images"

// LayoutMemorySnapshot reports editor-owned source and layout state without
// exposing MyGo types or the renderer's decoded assets.
type LayoutMemorySnapshot struct {
	SourceBytes          int
	Leaves               int
	VirtualLayout        bool
	ImageCandidateLeaves int
	ImageAssetViews      int
}

// ImageMemoryHandle is the narrow diagnostics contract implemented by the
// UI adapter's decoded image handle. It deliberately exposes no bitmap.
type ImageMemoryHandle interface {
	Loaded() bool
	ApproxPixelBytes() int64
}

func (e *Editor) LayoutMemorySnapshot() LayoutMemorySnapshot {
	if e == nil {
		return LayoutMemorySnapshot{}
	}
	snapshot := LayoutMemorySnapshot{
		SourceBytes:   len(e.Source()),
		Leaves:        len(e.leaves),
		VirtualLayout: e.virtualLayout(),
	}
	for _, leaf := range e.leaves {
		if leafMayContainImage(e.Source(), leaf) {
			snapshot.ImageCandidateLeaves++
		}
		if leaf.view != nil && leaf.view.hasImageAsset() {
			snapshot.ImageAssetViews++
		}
	}
	for _, view := range e.views {
		if view != nil && view.hasImageAsset() {
			snapshot.ImageAssetViews++
		}
	}
	return snapshot
}

// ImageMemoryHandles returns unique retained image resources through their
// backend-neutral diagnostics contract. UI objects remain owned by the adapter.
func (e *Editor) ImageMemoryHandles() []ImageMemoryHandle {
	if e == nil {
		return nil
	}
	seen := make(map[ImageMemoryHandle]struct{})
	add := func(asset ImageMemoryHandle) {
		if asset != nil {
			seen[asset] = struct{}{}
		}
	}
	addView := func(view *leafView) {
		if view == nil {
			return
		}
		if view.block != nil {
			add(view.block.asset)
		}
		for _, piece := range view.disp.pieces {
			if piece.obj != nil {
				add(piece.obj.asset)
			}
		}
	}
	for _, leaf := range e.leaves {
		addView(leaf.view)
	}
	for _, view := range e.views {
		addView(view)
	}
	e.dataImageAssets.Range(func(asset *images.Asset) { add(asset) })
	assets := make([]ImageMemoryHandle, 0, len(seen))
	for asset := range seen {
		assets = append(assets, asset)
	}
	return assets
}
