package media

import (
	"github.com/egoist/mygo/ui"
	"muse/markdown/images"
)

// RefreshImageBitmap synchronizes a toolkit bitmap with the current decoded
// frame of an existing image asset. The asset/cache remains owned by
// markdown/images; callers keep one UI bitmap per visible object.
func RefreshImageBitmap(asset *images.Asset, bitmap *ui.Bitmap, frameVersion uint64) (*ui.Bitmap, uint64) {
	if asset == nil {
		return bitmap, frameVersion
	}
	version := asset.FrameVersion()
	if version == frameVersion {
		return bitmap, frameVersion
	}
	frame := asset.CurrentFrame()
	if frame == nil {
		return bitmap, frameVersion
	}
	if bitmap == nil {
		bitmap = ui.NewBitmap(frame)
	} else {
		bitmap.Update(frame)
	}
	return bitmap, version
}
