package media

import (
	"unsafe"

	"muse/markdown/images"
)

// DataImageAssets retains non-cacheable data URL assets for one document
// revision. It prevents a view rebuild after asynchronous decode from starting
// another decode of the same large source. The owning editor controls the
// revision boundary and invokes it on the UI thread.
type DataImageAssets struct {
	revision uint64
	items    map[dataImageAssetKey]*images.Asset
}

type dataImageAssetKey struct {
	data *byte
	len  int
}

// ResetForRevision releases assets retained for the previous source revision.
func (c *DataImageAssets) ResetForRevision(revision uint64) {
	if c == nil || c.revision == revision {
		return
	}
	clear(c.items)
	c.items = nil
	c.revision = revision
}

// Load reuses an asset by source identity, retaining only the string header's
// backing pointer and length rather than another copy of a potentially large
// base64 URL. The source owner must keep src alive for the duration of the
// document revision.
func (c *DataImageAssets) Load(src string, options images.LoadOptions, loaded, intrinsicChanged func()) *images.Asset {
	if c == nil {
		return images.Load(src, options, loaded, intrinsicChanged)
	}
	key := dataImageAssetKey{data: unsafe.StringData(src), len: len(src)}
	if asset := c.items[key]; asset != nil {
		return asset
	}
	asset := images.Load(src, options, loaded, intrinsicChanged)
	if c.items == nil {
		c.items = make(map[dataImageAssetKey]*images.Asset)
	}
	c.items[key] = asset
	return asset
}

// Range visits currently retained assets without copying the registry.
func (c *DataImageAssets) Range(visit func(*images.Asset)) {
	if c == nil || visit == nil {
		return
	}
	for _, asset := range c.items {
		visit(asset)
	}
}

// Len reports the number of retained document-local data image assets.
func (c *DataImageAssets) Len() int {
	if c == nil {
		return 0
	}
	return len(c.items)
}
