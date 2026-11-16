package media

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"muse/markdown/images"
)

// ImageHost is the narrow capability used by the media subsystem to resolve
// and acquire a vault image. Source paths, cache ownership, and invalidation
// remain with the editor host.
type ImageHost interface {
	ResolveImage(destination string) string
	LoadImage(resolved string, dataURL bool) *images.Asset
}

// ImageObject is a detached layout/resource projection for one displayed
// image. It owns image sizing and placeholder metrics, while the host owns the
// asset lifetime and document source ranges.
type ImageObject struct {
	Asset        *images.Asset
	Bitmap       *ui.Bitmap
	FrameVersion uint64
	Pending      bool
	Path         string
	Destination  string
	Alt          string
	Width        float32
	Height       float32
	Descent      float32
	Block        bool
}

// NewImageObject creates the layout projection using intrinsic geometry when
// available and stable placeholder geometry while an asset is loading.
func NewImageObject(host ImageHost, destination, alt string, width, fontSize float32, alone bool) *ImageObject {
	if host == nil {
		return nil
	}
	path := host.ResolveImage(destination)
	asset := host.LoadImage(path, strings.HasPrefix(path, "data:"))
	if asset == nil {
		return nil
	}
	obj := &ImageObject{Asset: asset, Pending: !asset.Loaded(), Path: path, Destination: destination, Alt: alt, Block: alone}
	obj.Refresh()
	intrinsic, hasIntrinsic := asset.IntrinsicSize()
	if obj.Bitmap == nil {
		obj.Destination = destination
		obj.Width = max(width, 80)
		if width > 0 {
			obj.Width = width
		}
		obj.Height = fontSize * 2.4
		obj.Descent = obj.Height * .25
		if hasIntrinsic {
			obj.Width, obj.Height = ImageSizeAtWidth(float32(intrinsic.X), float32(intrinsic.Y), width)
			obj.Descent = 0
		}
		return obj
	}
	if hasIntrinsic {
		obj.Width, obj.Height = ImageSizeAtWidth(float32(intrinsic.X), float32(intrinsic.Y), width)
		return obj
	}
	w, h := obj.Bitmap.Size()
	obj.Width, obj.Height = ImageSizeAtWidth(float32(w), float32(h), width)
	return obj
}

// ApplySize applies source-declared HTML/wiki dimensions and clamps them to
// the available width.
func (obj *ImageObject) ApplySize(size ImageSize, width float32) {
	if obj == nil || size == (ImageSize{}) {
		return
	}
	baseW, baseH := obj.Width, obj.Height
	if intrinsic, ok := obj.Asset.IntrinsicSize(); ok {
		baseW, baseH = float32(intrinsic.X), float32(intrinsic.Y)
	} else if obj.Bitmap != nil {
		w, h := obj.Bitmap.Size()
		baseW, baseH = float32(w), float32(h)
	}
	obj.Width, obj.Height = SizedImageDimensions(baseW, baseH, size, width)
	if obj.Bitmap == nil {
		obj.Descent = 0
	}
}

// Refresh updates a toolkit bitmap only when the decoded asset advances its
// frame version.
func (obj *ImageObject) Refresh() {
	if obj == nil {
		return
	}
	obj.Bitmap, obj.FrameVersion = RefreshImageBitmap(obj.Asset, obj.Bitmap, obj.FrameVersion)
}
