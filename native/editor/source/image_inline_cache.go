package source

import "muse/markdown/syntax"

// ImageInlineCache retains one synthetic inline projection for a source
// revision and parser block. The tokens borrow strings from the source text;
// callers must Reset it when that revision is released.
type ImageInlineCache struct {
	revision uint64
	blockID  int
	inlines  []syntax.IncrementalInline
	valid    bool
}

// Lookup returns the projection only when it belongs to the requested source
// revision and block identity. The returned slice is borrowed and read-only.
func (c *ImageInlineCache) Lookup(revision uint64, blockID int) ([]syntax.IncrementalInline, bool) {
	if c == nil || !c.valid || c.revision != revision || c.blockID != blockID {
		return nil, false
	}
	return c.inlines, true
}

// Store records tokens whose source-backed strings remain valid for revision.
func (c *ImageInlineCache) Store(revision uint64, blockID int, inlines []syntax.IncrementalInline) {
	if c == nil || len(inlines) == 0 {
		return
	}
	c.revision, c.blockID, c.inlines, c.valid = revision, blockID, inlines, true
}

// ResetForRevision releases the previous source's borrowed strings when the
// editor advances its current revision.
func (c *ImageInlineCache) ResetForRevision(revision uint64) {
	if c == nil || c.revision == revision {
		return
	}
	*c = ImageInlineCache{revision: revision}
}

// Reset drops source-backed token references when their revision is no longer
// active.
func (c *ImageInlineCache) Reset() {
	if c == nil {
		return
	}
	*c = ImageInlineCache{}
}
